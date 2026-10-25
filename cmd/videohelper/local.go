package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/workspace"
)

func localPrincipal(ctx context.Context, pool *pgxpool.Pool) (workspace.Principal, error) {
	return workspace.New(pool).Resolve(ctx, authn.Claims{Sub: "videohelper-local", Name: "Local", Email: "local@videohelper"}, 0)
}

func localUpload(ctx context.Context, app *runtime, principal workspace.Principal, path string) (library.Video, error) {
	// Reject pipes/devices before opening so an invalid input cannot block waiting for a writer.
	info, err := os.Stat(path)
	if err != nil {
		return library.Video{}, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return library.Video{}, errors.New("local-upload requires a nonempty regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return library.Video{}, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return library.Video{}, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return library.Video{}, errors.New("local-upload requires a nonempty regular file")
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if !strings.HasPrefix(contentType, "video/") {
		contentType = "video/mp4"
	}
	video, plan, err := app.library.StartUpload(ctx, principal, filepath.Base(path), info.Size(), contentType, nil)
	if err != nil {
		return library.Video{}, err
	}
	id := video.ID
	parts, err := uploadParts(ctx, app.http, file, info.Size(), plan)
	if err == nil {
		video, err = app.library.CompleteUpload(ctx, principal, video.ID, plan.UploadID, parts)
	}
	if err != nil {
		return library.Video{}, fmt.Errorf("video %d upload incomplete (session retained): %w", id, err)
	}
	return video, nil
}

func uploadParts(ctx context.Context, client *http.Client, file io.ReaderAt, size int64, plan library.UploadPlan) ([]blob.Part, error) {
	parts := make([]blob.Part, 0, len(plan.Parts))
	for _, part := range plan.Parts {
		offset := int64(part.N-1) * plan.PartSize
		length := min(plan.PartSize, size-offset)
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, part.URL, io.NewSectionReader(file, offset, length))
		if err != nil {
			return nil, err
		}
		request.ContentLength = length
		response, err := client.Do(request)
		if err != nil {
			var transport *url.Error
			if errors.As(err, &transport) {
				err = transport.Err
			}
			return nil, fmt.Errorf("part %d: %w", part.N, err)
		}
		_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("part %d response: %w", part.N, readErr)
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("part %d returned HTTP %d", part.N, response.StatusCode)
		}
		if response.Header.Get("ETag") == "" {
			return nil, fmt.Errorf("part %d response has no ETag", part.N)
		}
		parts = append(parts, blob.Part{N: part.N, ETag: response.Header.Get("ETag")})
	}
	return parts, nil
}
