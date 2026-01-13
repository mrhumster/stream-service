package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStorage struct {
	client MinIOClient
}

func NewMinIOStorage(client MinIOClient) *MinIOStorage {
	return &MinIOStorage{
		client: client,
	}
}

func NewMinIOStorageFromConfig(endpoint, accessKey, secretKey string, useSSL bool) (*MinIOStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}
	return &MinIOStorage{
		client: client,
	}, nil
}

func (s *MinIOStorage) Upload(ctx context.Context, bucket, key string, data io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, bucket, key, data, size, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})

	if err != nil {
		return s.mapMinIOError(err)
	}

	return nil
}

func (s *MinIOStorage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, s.mapMinIOError(err)
	}
	return obj, nil
}

func (s *MinIOStorage) Delete(ctx context.Context, bucket, key string) error {
	err := s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return s.mapMinIOError(err)
	}
	return nil
}

func (s *MinIOStorage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
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
