package blob

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const maxParts = 10000

type Part struct {
	N    int    `json:"n"`
	ETag string `json:"etag"`
}

type PartURL struct {
	N   int    `json:"n"`
	URL string `json:"url"`
}

type Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func New(client *s3.Client, bucket string) *Store {
	return &Store{client: client, presign: s3.NewPresignClient(client), bucket: bucket}
}

// EnsureBucket is for local bootstrap; production buckets may be provisioned separately.
func (s *Store) EnsureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) || (apiErr.ErrorCode() != "NotFound" && apiErr.ErrorCode() != "NoSuchBucket") {
		return err
	}
	in := &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}
	if region := s.client.Options().Region; region != "" && region != "us-east-1" {
		in.CreateBucketConfiguration = &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(region)}
	}
	_, err = s.client.CreateBucket(ctx, in)
	return err
}

func (s *Store) CreateMultipart(ctx context.Context, key, contentType string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.UploadId), nil
}

func (s *Store) PresignParts(ctx context.Context, key, uploadID string, count int, ttl time.Duration) ([]PartURL, error) {
	if count < 1 || count > maxParts {
		return nil, fmt.Errorf("part count must be between 1 and %d", maxParts)
	}
	if err := validTTL(ttl); err != nil {
		return nil, err
	}
	parts := make([]PartURL, 0, count)
	for n := 1; n <= count; n++ {
		out, err := s.presign.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: aws.String(uploadID), PartNumber: aws.Int32(int32(n)),
		}, s3.WithPresignExpires(ttl))
		if err != nil {
			return nil, err
		}
		parts = append(parts, PartURL{N: n, URL: out.URL})
	}
	return parts, nil
}

func (s *Store) CompleteMultipart(ctx context.Context, key, uploadID string, parts []Part) error {
	if len(parts) == 0 || len(parts) > maxParts {
		return fmt.Errorf("part count must be between 1 and %d", maxParts)
	}
	parts = slices.Clone(parts)
	slices.SortFunc(parts, func(a, b Part) int { return cmp.Compare(a.N, b.N) })
	completed := make([]types.CompletedPart, 0, len(parts))
	for i, p := range parts {
		if p.N < 1 || p.N > maxParts || p.ETag == "" || (i > 0 && p.N == parts[i-1].N) {
			return errors.New("multipart parts require unique numbers between 1 and 10000 and nonempty ETags")
		}
		completed = append(completed, types.CompletedPart{PartNumber: aws.Int32(int32(p.N)), ETag: aws.String(p.ETag)})
	}
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed},
	})
	return err
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader, contentType string) error {
	if _, seekable := r.(io.Seeker); !seekable {
		// shortcut: unknown-length Put supports 10000 16-MiB parts; use PutStream with an explicit larger limit beyond that.
		_, err := s.putStream(ctx, key, r, 16*1024*1024*maxParts, contentType)
		return err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: r, ContentType: aws.String(contentType),
	})
	return err
}

func (s *Store) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := validTTL(ttl); err != nil {
		return "", err
	}
	out, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *Store) DeletePrefix(ctx context.Context, prefix string) error {
	if prefix == "" {
		return errors.New("object prefix must not be empty")
	}
	pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket), Prefix: aws.String(prefix),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}
		out, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket), Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
		if len(out.Errors) > 0 {
			return fmt.Errorf("delete object %q: %s: %s", aws.ToString(out.Errors[0].Key), aws.ToString(out.Errors[0].Code), aws.ToString(out.Errors[0].Message))
		}
	}
	return nil
}

func (s *Store) URI(key string) string { return "s3://" + s.bucket + "/" + key }

func validTTL(ttl time.Duration) error {
	if ttl < time.Second || ttl > 7*24*time.Hour {
		return errors.New("presign duration must be between 1 second and 7 days")
	}
	return nil
}
