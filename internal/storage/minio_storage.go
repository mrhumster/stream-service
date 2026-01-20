package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
)

type MinIOStorage struct {
	client MinIOClient
	bucket string
}

func NewMinIOStorage(client MinIOClient, bucket string) *MinIOStorage {
	return &MinIOStorage{
		client: client,
		bucket: bucket,
	}
}

func (s *MinIOStorage) Upload(ctx context.Context, path string, data io.Reader, size int64) error {
	exists, err := s.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket: %w", err)
	}

	if !exists {
		if err := s.CreateBucket(ctx, s.bucket); err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}
	_, err = s.client.PutObject(ctx, s.bucket, path, data, size, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})

	if err != nil {
		return s.mapMinIOError(err)
	}

	return nil
}

func (s *MinIOStorage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, s.mapMinIOError(err)
	}
	return obj, nil
}

func (s *MinIOStorage) Delete(ctx context.Context, path string) error {
	err := s.client.RemoveObject(ctx, s.bucket, path, minio.RemoveObjectOptions{})
	if err != nil {
		return s.mapMinIOError(err)
	}
	return nil
}

func (s *MinIOStorage) Exists(ctx context.Context, path string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, path, minio.StatObjectOptions{})
	if err != nil {
		if minioErr, ok := err.(minio.ErrorResponse); ok {
			switch minioErr.Code {
			case "NoSuchKey":
				return false, nil
			case "NoSuchBucket":
				return false, ErrBucketNotExist
			}
		}
		return false, s.mapMinIOError(err)
	}
	return true, nil
}

func (s *MinIOStorage) BucketExists(ctx context.Context, bucket string) (bool, error) {
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return false, s.mapMinIOError(err)
	}
	return exists, nil
}

func (s *MinIOStorage) CreateBucket(ctx context.Context, bucket string) error {
	exists, err := s.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if exists {
		return ErrAlreadyExists
	}

	err = s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	if err != nil {
		return s.mapMinIOError(err)
	}
	return nil
}

func (s *MinIOStorage) GeneratePresignedURL(ctx context.Context, path string, expire time.Duration) (*url.URL, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, path, expire, nil)
	if err != nil {
		return nil, ErrGenerateURLFailed
	}
	return u, nil
}

func (s *MinIOStorage) mapMinIOError(err error) error {
	if minioErr, ok := err.(minio.ErrorResponse); ok {
		switch minioErr.Code {
		case "NoSuchKey":
			return ErrNotFound
		case "NoSuchBucket":
			return ErrBucketNotExist
		case "AccessDenied":
			return ErrPermissionDenied
		}
		return fmt.Errorf("%w: %s", ErrStorage, minioErr.Message)
	}
	return fmt.Errorf("%w: %v", ErrStorage, err)
}
