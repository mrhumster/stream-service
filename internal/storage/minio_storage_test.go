package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/mrhumster/stream-service/internal/storage/mock"
	"go.uber.org/mock/gomock"
)

type mockMinIOObject struct {
	io.Reader
}

func (m *mockMinIOObject) Close() error {
	if closer, ok := m.Reader.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

func (m *mockMinIOObject) Stat() (minio.ObjectInfo, error) {
	return minio.ObjectInfo{}, nil
}

func (m *mockMinIOObject) Read(p []byte) (n int, err error) {
	return m.Reader.Read(p)
}

func TestMinIOStorage_Upload(t *testing.T) {
	tests := []struct {
		name          string
		bucketExists  bool
		makeBucketErr error
		putObjectErr  error
		expectErr     bool
	}{
		{
			name:         "success when bucket exists",
			bucketExists: true,
			expectErr:    false,
		},
		{
			name:         "success when bucket created",
			bucketExists: false,
			expectErr:    false,
		},
		{
			name:          "fail when bucket creation fails",
			bucketExists:  false,
			makeBucketErr: errors.New("creation failed"),
			expectErr:     true,
		},
		{
			name:         "fail when upload fails",
			bucketExists: true,
			putObjectErr: errors.New("upload failed"),
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()
			mockMinIO := mock.NewMockMinIOClient(ctrl)
			bucket := "test-bucket"
			storage := &MinIOStorage{
				client: mockMinIO,
				bucket: bucket,
			}

			key := "test-file.txt"
			content := []byte("Hello, MinIO!")
			size := int64(len(content))

			mockMinIO.EXPECT().
				BucketExists(gomock.Any(), bucket).
				Return(tt.bucketExists, nil).AnyTimes()

			if !tt.bucketExists && tt.makeBucketErr == nil {
				mockMinIO.EXPECT().
					MakeBucket(gomock.Any(), bucket, gomock.Any()).
					Return(nil)
			}

			if tt.makeBucketErr != nil {
				mockMinIO.EXPECT().
					MakeBucket(gomock.Any(), bucket, gomock.Any()).
					Return(tt.makeBucketErr)
			}

			if tt.expectErr == false || (tt.bucketExists && tt.putObjectErr != nil) {
				mockMinIO.EXPECT().
					PutObject(gomock.Any(), bucket, key, gomock.Any(), size, gomock.Any()).
					Return(minio.UploadInfo{}, tt.putObjectErr)
			}

			err := storage.Upload(ctx, key, bytes.NewReader(content), size)

			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

func TestMinIOStorage_Download(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockMinIO := mock.NewMockMinIOClient(ctrl)
	bucket := "test-bucket"
	storage := &MinIOStorage{
		client: mockMinIO,
		bucket: bucket,
	}
	key := "test-file.txt"
	tests := []struct {
		name          string
		setupMock     func()
		expectedError error
	}{
		{
			name: "NoSuchKey error",
			setupMock: func() {
				mockMinIO.EXPECT().
					GetObject(gomock.Any(), bucket, key, gomock.Any()).
					Return(nil, minio.ErrorResponse{Code: "NoSuchKey"})
			},
			expectedError: ErrNotFound,
		},
		{
			name: "NoSuchBucket error",
			setupMock: func() {
				mockMinIO.EXPECT().
					GetObject(gomock.Any(), bucket, key, gomock.Any()).
					Return(nil, minio.ErrorResponse{Code: "NoSuchBucket"})
			},
			expectedError: ErrBucketNotExist,
		},
		{
			name: "AccessDenied error",
			setupMock: func() {
				mockMinIO.EXPECT().
					GetObject(gomock.Any(), bucket, key, gomock.Any()).
					Return(nil, minio.ErrorResponse{Code: "AccessDenied"})
			},
			expectedError: ErrPermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			_, err := storage.Download(ctx, key)
			if err != tt.expectedError {
				t.Fatalf("Expected %v, got %v", tt.expectedError, err)
			}
		})
	}
}

func TestMinIOStorage_Exists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockMinIO := mock.NewMockMinIOClient(ctrl)
	bucket := "test-bucket"
	storage := &MinIOStorage{
		client: mockMinIO,
		bucket: bucket,
	}
	key := "existing-file.txt"

	t.Run("File exists", func(t *testing.T) {
		mockMinIO.EXPECT().
			StatObject(gomock.Any(), bucket, key, gomock.Any()).
			Return(minio.ObjectInfo{}, nil)
		exists, err := storage.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Fatal("File should exist")
		}
	})

	t.Run("File does not exist", func(t *testing.T) {
		mockMinIO.EXPECT().
			StatObject(gomock.Any(), bucket, key, gomock.Any()).
			Return(minio.ObjectInfo{}, minio.ErrorResponse{
				Code: "NoSuchKey",
			})
		exists, err := storage.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if exists {
			t.Fatal("File should not exist")
		}
	})

	t.Run("Bucket does not exist", func(t *testing.T) {
		mockMinIO.EXPECT().
			StatObject(gomock.Any(), bucket, key, gomock.Any()).
			Return(minio.ObjectInfo{}, minio.ErrorResponse{
				Code: "NoSuchBucket",
			})
		exists, err := storage.Exists(ctx, key)
		if err != ErrBucketNotExist {
			t.Fatalf("Expected ErrBucketNotExist, got %v", err)
		}
		if exists {
			t.Fatal("Should return false on bucket error")
		}
	})
}

func TestMinIOStorage_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockMinIO := mock.NewMockMinIOClient(ctrl)
	bucket := "test-bucket"
	storage := &MinIOStorage{
		client: mockMinIO,
		bucket: bucket,
	}
	key := "file-to-delete.txt"

	t.Run("Successful delete", func(t *testing.T) {
		mockMinIO.EXPECT().
			RemoveObject(gomock.Any(), bucket, key, gomock.Any()).
			Return(nil)

		err := storage.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	})

	t.Run("Delete non-existent file", func(t *testing.T) {
		mockMinIO.EXPECT().
			RemoveObject(gomock.Any(), bucket, key, gomock.Any()).
			Return(minio.ErrorResponse{
				Code: "NoSuchKey",
			})

		err := storage.Delete(ctx, key)
		if err != ErrNotFound {
			t.Fatalf("Expected ErrNotFound, got %v", err)
		}
	})
}

func TestMinIOStorage_BucketOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockMinIO := mock.NewMockMinIOClient(ctrl)
	bucket := "test-bucket"
	storage := &MinIOStorage{
		client: mockMinIO,
		bucket: bucket,
	}

	t.Run("Bucket exists", func(t *testing.T) {
		mockMinIO.EXPECT().
			BucketExists(gomock.Any(), bucket).
			Return(true, nil)

		exists, err := storage.BucketExists(ctx, bucket)
		if err != nil {
			t.Fatalf("BucketExists failed: %v", err)
		}
		if !exists {
			t.Fatal("Bucket should exist")
		}
	})

	t.Run("Create bucket", func(t *testing.T) {
		mockMinIO.EXPECT().
			BucketExists(gomock.Any(), bucket).
			Return(false, nil)

		mockMinIO.EXPECT().
			MakeBucket(gomock.Any(), bucket, gomock.Any()).
			Return(nil)

		err := storage.CreateBucket(ctx, bucket)
		if err != nil {
			t.Fatalf("CreateBucket failed: %v", err)
		}
	})

	t.Run("Create existing bucket", func(t *testing.T) {
		mockMinIO.EXPECT().
			BucketExists(gomock.Any(), bucket).
			Return(true, nil)

		err := storage.CreateBucket(ctx, bucket)
		if err != ErrAlreadyExists {
			t.Fatalf("Expected ErrAlreadyExists, got %v", err)
		}
	})
}
