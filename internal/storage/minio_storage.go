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
	Client MinIOClient
	Bucket string
}

func NewMinIOStorage(client *minio.Client, bucket string) *MinIOStorage {
	return &MinIOStorage{
		Client: NewMinIOAdapter(client),
		Bucket: bucket,
	}
}

func (s *MinIOStorage) GetBucketName() string {
	return s.Bucket
}

func (s *MinIOStorage) Upload(ctx context.Context, path string, data io.Reader, size int64) error {
	exists, err := s.BucketExists(ctx, s.Bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket: %w", err)
	}

	if !exists {
		if err := s.CreateBucket(ctx, s.Bucket); err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}
	_, err = s.Client.PutObject(ctx, s.Bucket, path, data, size, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return s.mapMinIOError(err)
	}

	return nil
}

func (s *MinIOStorage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	obj, err := s.Client.GetObject(ctx, s.Bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, s.mapMinIOError(err)
	}
	return obj, nil
}

func (s *MinIOStorage) Delete(ctx context.Context, path string) error {
	err := s.Client.RemoveObject(ctx, s.Bucket, path, minio.RemoveObjectOptions{})
	if err != nil {
		return s.mapMinIOError(err)
	}
	return nil
}

func (s *MinIOStorage) Exists(ctx context.Context, path string) (bool, error) {
	_, err := s.Client.StatObject(ctx, s.Bucket, path, minio.StatObjectOptions{})
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
	exists, err := s.Client.BucketExists(ctx, bucket)
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

	err = s.Client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	if err != nil {
		return s.mapMinIOError(err)
	}
	return nil
}

func (s *MinIOStorage) GeneratePresignedURL(ctx context.Context, path, filename string, expire time.Duration) (*url.URL, error) {
	u, err := s.Client.PresignedGetObject(ctx, s.Bucket, path, expire, nil)
	if err != nil {
		return nil, ErrGenerateURLFailed
	}
	return u, nil
}

func (s *MinIOStorage) AbortMultipart(ctx context.Context, path, uploadID string) error {
	return s.Client.AbortMultipartUpload(ctx, s.Bucket, path, uploadID)
}

func (s *MinIOStorage) InitMultipart(ctx context.Context, path string) (uploadID string, err error) {
	uploadID, err = s.Client.NewMultipartUpload(ctx, s.Bucket, path, minio.PutObjectOptions{
		ContentType: "video/mp4",
	})
	if err != nil {
		return "", s.mapMinIOError(err)
	}
	return uploadID, err
}

func (s *MinIOStorage) UploadPart(ctx context.Context, path, uploadID string, partNumber int, data io.Reader, size int64) (etag string, err error) {
	part, err := s.Client.PutObjectPart(ctx, s.Bucket, path, uploadID, partNumber, data, size, minio.PutObjectPartOptions{})
	if err != nil {
		return "", err
	}
	return part.ETag, nil
}

func (s *MinIOStorage) CompleteMultipart(ctx context.Context, path, uploadID string, parts []MultipartPart) error {
	minioParts := make([]minio.CompletePart, len(parts))
	for i, p := range parts {
		minioParts[i] = minio.CompletePart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		}
	}
	_, err := s.Client.CompleteMultipartUpload(
		ctx,
		s.Bucket,
		path,
		uploadID,
		minioParts,
		minio.PutObjectOptions{},
	)
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
