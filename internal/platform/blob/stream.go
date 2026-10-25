package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var ErrTooLarge = errors.New("object exceeds size limit")

// PutStream uses bounded memory and aborts incomplete uploads on every failure.
func (s *Store) PutStream(ctx context.Context, key string, r io.Reader, maxBytes int64) (n int64, err error) {
	return s.putStream(ctx, key, r, maxBytes, "application/octet-stream")
}

func (s *Store) putStream(ctx context.Context, key string, r io.Reader, maxBytes int64, contentType string) (n int64, err error) {
	if maxBytes < 0 || maxBytes > 5*1024*1024*1024*1024 {
		return 0, errors.New("stream size limit must be between 0 and 5 TiB")
	}
	uploadID, err := s.CreateMultipart(ctx, key, contentType)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err == nil {
			return
		}
		// Cleanup must still reach S3 after the upload context is canceled.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, abortErr := s.client.AbortMultipartUpload(cleanupCtx, &s3.AbortMultipartUploadInput{
			Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: aws.String(uploadID),
		})
		if abortErr != nil {
			err = errors.Join(err, fmt.Errorf("abort multipart upload: %w", abortErr))
		}
	}()
	partSize := max(int64(16*1024*1024), (maxBytes+maxParts-1)/maxParts)
	buf := make([]byte, partSize)
	var parts []Part
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		size := min(partSize, maxBytes-n+1)
		read, readErr := readPart(ctx, r, buf[:size])
		n += int64(read)
		if n > maxBytes {
			return n, ErrTooLarge
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return n, readErr
		}
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if read > 0 || len(parts) == 0 {
			partN := len(parts) + 1
			out, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
				Bucket: aws.String(s.bucket), Key: aws.String(key), UploadId: aws.String(uploadID),
				PartNumber: aws.Int32(int32(partN)), Body: bytes.NewReader(buf[:read]),
			})
			if err != nil {
				return n, err
			}
			parts = append(parts, Part{N: partN, ETag: aws.ToString(out.ETag)})
		}
		if readErr != nil {
			break
		}
	}
	return n, s.CompleteMultipart(ctx, key, uploadID, parts)
}

// Preserve source errors such as an HTTP body's unexpected EOF instead of completing truncated data.
func readPart(ctx context.Context, r io.Reader, buf []byte) (n int, err error) {
	for n < len(buf) {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		read, err := r.Read(buf[n:])
		n += read
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
