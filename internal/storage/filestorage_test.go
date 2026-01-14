package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mrhumster/stream-service/internal/storage/mock"
	"go.uber.org/mock/gomock"
)

func TestFileStorageInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mock.NewMockFileStorage(ctrl)
	ctx := context.Background()

	t.Run("Upload and Download", func(t *testing.T) {
		key := "test-file.txt"
		content := []byte("Hello, World!")

		mockStorage.EXPECT().
			Upload(ctx, key, gomock.Any(), int64(len(content))).
			DoAndReturn(func(ctx context.Context, key string, reader io.Reader, size int64) error {
				data, err := io.ReadAll(reader)
				if err != nil {
					return err
				}
				if !bytes.Equal(data, content) {
					return errors.New("content mismatch")
				}
				return nil
			})

		mockStorage.EXPECT().
			Exists(ctx, key).
			Return(true, nil)

		mockStorage.EXPECT().
			Download(ctx, key).
			Return(io.NopCloser(bytes.NewReader(content)), nil)

		err := mockStorage.Upload(ctx, key, bytes.NewReader(content), int64(len(content)))
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		exists, err := mockStorage.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Fatal("File should exist")
		}

		reader, err := mockStorage.Download(ctx, key)
		if err != nil {
			t.Fatalf("Download failed: %v", err)
		}
		defer reader.Close()

		downloaded, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if !bytes.Equal(downloaded, content) {
			t.Fatalf("Content mismatch")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		key := "delete-test.txt"

		mockStorage.EXPECT().
			Delete(ctx, key).
			Return(nil)

		mockStorage.EXPECT().
			Exists(ctx, key).
			Return(false, nil)

		err := mockStorage.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		exists, err := mockStorage.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if exists {
			t.Fatal("File should not exist after delete")
		}
	})

	t.Run("Download non-existent file", func(t *testing.T) {
		key := "no-key"

		mockStorage.EXPECT().
			Download(ctx, key).
			Return(nil, ErrNotFound)

		_, err := mockStorage.Download(ctx, key)
		if err != ErrNotFound {
			t.Fatalf("Expected ErrNotFound, got %v", err)
		}
	})
}
