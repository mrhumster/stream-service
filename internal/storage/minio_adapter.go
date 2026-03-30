package storage

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
)

type minioAdapter struct {
	client *minio.Client
	core   *minio.Core
}

func NewMinIOAdapter(client *minio.Client) MinIOClient {
	return &minioAdapter{
		client: client,
		core:   &minio.Core{Client: client},
	}
}

func (a *minioAdapter) PutObject(ctx context.Context, bucket, object string, reader io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return a.client.PutObject(ctx, bucket, object, reader, size, opts)
}

func (a *minioAdapter) GetObject(ctx context.Context, bucket, object string, opts minio.GetObjectOptions) (*minio.Object, error) {
	return a.client.GetObject(ctx, bucket, object, opts)
}

func (a *minioAdapter) RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error {
	return a.client.RemoveObject(ctx, bucket, object, opts)
}

func (a *minioAdapter) StatObject(ctx context.Context, bucket, object string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return a.client.StatObject(ctx, bucket, object, opts)
}

func (a *minioAdapter) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return a.client.BucketExists(ctx, bucket)
}

func (a *minioAdapter) MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error {
	return a.client.MakeBucket(ctx, bucket, opts)
}

func (a *minioAdapter) PresignedGetObject(ctx context.Context, bucket, object string, expires time.Duration, reqParams url.Values) (*url.URL, error) {
	return a.client.PresignedGetObject(ctx, bucket, object, expires, reqParams)
}

func (a *minioAdapter) NewMultipartUpload(ctx context.Context, bucket, object string, opts minio.PutObjectOptions) (string, error) {
	return a.core.NewMultipartUpload(ctx, bucket, object, opts)
}

func (a *minioAdapter) PutObjectPart(ctx context.Context, bucket, object, uploadID string, partNumber int, reader io.Reader, objectSize int64, opts minio.PutObjectPartOptions) (minio.ObjectPart, error) {
	return a.core.PutObjectPart(ctx, bucket, object, uploadID, partNumber, reader, objectSize, opts)
}

func (a *minioAdapter) CompleteMultipartUpload(ctx context.Context, bucket, object, uploadID string, uploadedParts []minio.CompletePart, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return a.core.CompleteMultipartUpload(ctx, bucket, object, uploadID, uploadedParts, opts)
}

func (a *minioAdapter) AbortMultipartUpload(ctx context.Context, bucket, object, uploadID string) error {
	return a.core.AbortMultipartUpload(ctx, bucket, object, uploadID)
}

func (a *minioAdapter) ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return a.client.ListObjects(ctx, bucketName, opts)
}

func (a *minioAdapter) RemoveObjects(ctx context.Context, bucketName string, objectsCh <-chan minio.ObjectInfo, opts minio.RemoveObjectsOptions) <-chan minio.RemoveObjectError {
	return a.client.RemoveObjects(ctx, bucketName, objectsCh, opts)
}
