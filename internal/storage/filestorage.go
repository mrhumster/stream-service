package storage

import (
	"context"
	"io"
)

type FileStorage interface {
	Upload(ctx context.Context, bucket, key string, data io.Reader, size int64) error
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, bucket, key string) error
	Exists(ctx context.Context, bucket, key string) (bool, error)
}
