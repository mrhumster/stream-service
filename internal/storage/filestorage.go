//go:generate mockgen -source=filestorage.go -destination=mock/filestorage_mock.go -package=mock
package storage

import (
	"context"
	"io"
	"net/url"
	"time"
)

type FileStorage interface {
	Upload(ctx context.Context, path string, data io.Reader, size int64) error
	Download(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
	GeneratePresignedURL(ctx context.Context, path, filename string, expires time.Duration) (*url.URL, error)
	InitMultipart(ctx context.Context, path string) (uploadID string, err error)
	UploadPart(ctx context.Context, path, uploadID string, partNumber int, data io.Reader, size int64) (etag string, err error)
	CompleteMultipart(ctx context.Context, path, uploadID string, parts []MultipartPart) error
	AbortMultipart(ctx context.Context, path, uploadID string) error
	GetBucketName() string
}

type MultipartPart struct {
	PartNumber int
	ETag       string
}
