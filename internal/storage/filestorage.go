//go:generate mockgen -source=filestorage.go -destination=mock/filestorage_mock.go -package=mock
package storage

import (
	"context"
	"io"
)

type FileStorage interface {
	Upload(ctx context.Context, path string, data io.Reader, size int64) error
	Download(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
}
