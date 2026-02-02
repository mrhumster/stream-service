package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	repomock "github.com/mrhumster/stream-service/internal/repository/mock"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/storage"
	"github.com/mrhumster/stream-service/internal/storage/mock"
	authmock "github.com/mrhumster/web-server-gin/pkg/auth/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestStreamServiceImpl_ListUserStreams(t *testing.T) {
	ctx := context.Background()

	t.Run("list user streams", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)

		userID := uuid.New()

		expectedStreams := []*models.Stream{
			{Title: "User Stream 1", OwnerID: userID},
			{Title: "User Stream 2", OwnerID: userID},
		}

		mockRepo.EXPECT().
			List(
				gomock.Any(),
				gomock.All(
					gomock.AssignableToTypeOf(repository.StreamFilter{}),
					gomock.Cond(func(x interface{}) bool {
						f, ok := x.(repository.StreamFilter)
						return ok && f.OwnerID != nil && *f.OwnerID == userID
					}),
				),
			).
			Return(expectedStreams, nil)

		streams, err := serviceImpl.ListUserStreams(ctx, userID)

		require.NoError(t, err)
		require.Len(t, streams, 2)
	})
}
func TestStreamServicImpl_ListStreams(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRepo := repomock.NewMockStreamRepository(ctrl)
	mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
	mockStorage := mock.NewMockFileStorage(ctrl)
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)

	t.Run("list all streams with filter", func(t *testing.T) {

		ownerID := uuid.New()
		filter := repository.StreamFilter{
			OwnerID: &ownerID,
			Limit:   10,
			Offset:  0,
		}

		expectedStreams := []*models.Stream{
			{Title: "Stream 1", OwnerID: ownerID},
			{Title: "Stream 2", OwnerID: ownerID},
		}
		mockRepo.EXPECT().List(gomock.Any(), filter).Return(expectedStreams, nil).Times(1)

		streams, err := serviceImpl.ListStreams(ctx, filter)

		require.NoError(t, err)
		require.Len(t, streams, 2)
		assert.Equal(t, "Stream 1", streams[0].Title)
		assert.Equal(t, "Stream 2", streams[1].Title)
	})

	t.Run("list with search filter", func(t *testing.T) {

		filter := repository.StreamFilter{
			Search: "gaming",
			Limit:  10,
		}

		expectedStreams := []*models.Stream{
			{Title: "Gaming Stream", OwnerID: uuid.New()},
		}

		mockRepo.EXPECT().List(gomock.Any(), filter).Return(expectedStreams, nil).Times(1)
		streams, err := serviceImpl.ListStreams(ctx, filter)
		require.NoError(t, err)
		require.Len(t, streams, 1)
		assert.Equal(t, "Gaming Stream", streams[0].Title)
	})

	t.Run("empty result", func(t *testing.T) {
		filter := repository.StreamFilter{Limit: 10}
		mockRepo.EXPECT().List(gomock.Any(), filter).Return([]*models.Stream{}, nil).Times(1)
		streams, err := serviceImpl.ListStreams(ctx, filter)
		require.NoError(t, err)
		assert.Empty(t, streams)
	})

	t.Run("repository error propagate", func(t *testing.T) {
		filter := repository.StreamFilter{Limit: 10}
		mockRepo.EXPECT().List(gomock.Any(), filter).Return(nil, assert.AnError).Times(1)
		streams, err := serviceImpl.ListStreams(ctx, filter)
		require.Error(t, err)
		assert.Nil(t, streams)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
func TestStreamServiceImpl_DeleteStream(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repomock.NewMockStreamRepository(ctrl)
	mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
	mockStorage := mock.NewMockFileStorage(ctrl)
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)

	t.Run("successful delete", func(t *testing.T) {
		ownerID := uuid.New()
		generatedStreamID := uuid.New()

		streamForDelete := &models.Stream{
			Description: "Drasft",
			Title:       "Stream test",
			OwnerID:     ownerID,
			Status:      models.StatusDraft,
		}

		streamForDelete.ID = generatedStreamID

		mockRepo.EXPECT().
			Read(gomock.Any(), generatedStreamID).
			Return(streamForDelete, nil)

		mockRepo.EXPECT().
			Delete(gomock.Any(), generatedStreamID).
			Return(nil)

		mockPermissionClient.EXPECT().
			RemovePolicy(
				gomock.Any(),
				ownerID.String(),
				fmt.Sprintf("stream/%s", generatedStreamID.String()),
				"write",
			).
			Return(true, nil).
			Times(1)

		mockPermissionClient.EXPECT().RemovePolicy(
			gomock.Any(),
			ownerID.String(),
			fmt.Sprintf("stream/%s", generatedStreamID.String()),
			"read",
		).
			Return(true, nil).
			Times(1)
		mockPermissionClient.EXPECT().RemovePolicy(
			gomock.Any(),
			ownerID.String(),
			fmt.Sprintf("stream/%s", generatedStreamID.String()),
			"delete",
		).
			Return(true, nil).
			Times(1)

		err := serviceImpl.DeleteStream(ctx, generatedStreamID)
		require.NoError(t, err)
	})

	t.Run("stream not found", func(t *testing.T) {
		streamID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(nil, gorm.ErrRecordNotFound)
		err := serviceImpl.DeleteStream(ctx, streamID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("repository error propagate", func(t *testing.T) {
		streamID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(nil, assert.AnError)
		err := serviceImpl.DeleteStream(ctx, streamID)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("cannot delete published stream", func(t *testing.T) {
		streamID := uuid.New()
		publishedStream := &models.Stream{
			Title:  "Published Stream",
			Status: models.StatusPublished,
		}
		publishedStream.ID = streamID
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(publishedStream, nil)
		err := serviceImpl.DeleteStream(ctx, streamID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "published")
	})
	t.Run("delete with file", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		r := repomock.NewMockStreamRepository(c)
		p := authmock.NewMockPermissionClient(c)
		s := mock.NewMockFileStorage(ctrl)
		srv := service.NewStreamServiceImpl(r, p, s)
		streamID := uuid.New()
		userID := uuid.New()
		storageKey := fmt.Sprintf("streams/%s/videos/%s_%s",
			userID.String(),
			streamID.String(),
			uuid.New().String())

		stor := models.StreamStorage{
			Key:      storageKey,
			Bucket:   "bucket",
			Filename: "filename",
			Provider: "minio",
		}
		storJSON, err := json.Marshal(stor)

		if err != nil {
			t.Errorf("Marshalization error: %v", err)
		}

		streamWithFile := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Title:     "Stream with file",
			Status:    models.StatusDraft,
			Storage:   datatypes.JSON(storJSON),
		}
		r.EXPECT().Read(gomock.Any(), streamID).Return(streamWithFile, nil)
		r.EXPECT().Delete(gomock.Any(), streamID).Return(nil)
		p.EXPECT().RemovePolicy(gomock.Any(), userID.String(), "stream/"+streamID.String(), "read").Return(true, nil)
		p.EXPECT().RemovePolicy(gomock.Any(), userID.String(), "stream/"+streamID.String(), "write").Return(true, nil)
		p.EXPECT().RemovePolicy(gomock.Any(), userID.String(), "stream/"+streamID.String(), "delete").Return(true, nil)
		s.EXPECT().Delete(gomock.Any(), stor.Key).Return(nil)
		err = srv.DeleteStream(ctx, streamID)
		require.NoError(t, err)
	})
}
func TestStreamServicImpl_GetStream(t *testing.T) {
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repomock.NewMockStreamRepository(ctrl)
	mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
	mockStorage := mock.NewMockFileStorage(ctrl)
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)

	t.Run("successful get stream", func(t *testing.T) {
		streamID := uuid.New()
		existingStream := &models.Stream{
			Title:   "Original Title",
			OwnerID: uuid.New(),
			Status:  models.StatusDraft,
		}
		existingStream.ID = streamID
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existingStream, nil)
		stream, err := serviceImpl.GetStream(ctx, streamID)

		require.NoError(t, err)
		require.NotNil(t, stream)
		assert.Equal(t, existingStream.ID, stream.ID)
		assert.Equal(t, existingStream.Title, stream.Title)
	})

	t.Run("stream not found", func(t *testing.T) {

		nonExistenID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), nonExistenID).Return(nil, gorm.ErrRecordNotFound)
		stream, err := serviceImpl.GetStream(ctx, nonExistenID)
		require.Error(t, err)
		assert.Nil(t, stream)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("repository error propagate", func(t *testing.T) {
		streamID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(nil, assert.AnError)
		stream, err := serviceImpl.GetStream(ctx, streamID)
		require.Error(t, err)
		assert.Nil(t, stream)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
func TestStreamServicImpl_UpdateStream(t *testing.T) {
	ctx := context.Background()

	t.Run("succesful stream update", func(t *testing.T) {

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)
		streamID := uuid.New()
		ownerID := uuid.New()

		existingStream := &models.Stream{
			Title:       "Original Title",
			Description: "Original Description",
			OwnerID:     ownerID,
			Status:      models.StatusDraft,
			Visibility:  models.VisibilityPrivate,
		}
		existingStream.ID = streamID
		title := "Updated title"
		description := "Updated Description"
		visibility := models.VisibilityPublic
		req := service.UpdateStreamRequest{
			Title:       &title,
			Description: &description,
			Visibility:  (*models.StreamVisibility)(&visibility),
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existingStream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Cond(func(stream *models.Stream) bool {
			return stream.Title == title &&
				stream.Description == description &&
				stream.Visibility == models.VisibilityPublic &&
				stream.ID == streamID
		})).Return(nil)
		updatedStream, err := serviceImpl.UpdateStream(ctx, streamID, req)
		require.NoError(t, err)
		require.NotNil(t, updatedStream)
		assert.Equal(t, title, updatedStream.Title)
		assert.Equal(t, description, updatedStream.Description)
		assert.Equal(t, models.VisibilityPublic, updatedStream.Visibility)
	})

	t.Run("update stream tags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)

		mockStorage := mock.NewMockFileStorage(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)
		streamID := uuid.New()
		existingStream := &models.Stream{
			Title:   "Test Stream",
			OwnerID: uuid.New(),
			Tags:    datatypes.JSON(`["old-tag"]`),
		}

		newTags := []string{"gaming", "live", "fun"}
		req := service.UpdateStreamRequest{
			Tags: &newTags,
		}

		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existingStream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Cond(func(s *models.Stream) bool {
			var tags []string
			json.Unmarshal(s.Tags, &tags)
			return assert.ElementsMatch(t, newTags, tags)
		}))

		updated, err := serviceImpl.UpdateStream(ctx, streamID, req)
		require.NoError(t, err)
		require.NotNil(t, updated)
	})

	t.Run("should validate title before update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)
		streamID := uuid.New()
		existiongStream := &models.Stream{
			Title:   "Original Title",
			OwnerID: uuid.New(),
			Status:  models.StatusDraft,
		}
		existiongStream.ID = streamID
		emptyTitle := ""
		req := service.UpdateStreamRequest{
			Title: &emptyTitle,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existiongStream, nil)
		updated, err := serviceImpl.UpdateStream(ctx, streamID, req)
		require.Error(t, err)
		assert.Nil(t, updated)
		assert.Contains(t, err.Error(), "title")
	})

	t.Run("should validate title length", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)
		streamID := uuid.New()
		existingStream := &models.Stream{
			Title:   "Original",
			OwnerID: uuid.New(),
			Status:  models.StatusDraft,
		}
		existingStream.ID = streamID
		longTitle := ""
		for i := 0; i < 256; i++ {
			longTitle += "a"
		}
		req := service.UpdateStreamRequest{
			Title: &longTitle,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existingStream, nil)
		updated, err := serviceImpl.UpdateStream(ctx, streamID, req)

		require.Error(t, err)
		assert.Nil(t, updated)
	})

	t.Run("cannot update published stream", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)

		mockStorage := mock.NewMockFileStorage(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)
		streamID := uuid.New()
		publishedStream := &models.Stream{
			Title:  "Published Stream",
			Status: models.StatusPublished,
		}
		publishedStream.ID = streamID
		newTitle := "New Title"
		req := service.UpdateStreamRequest{Title: &newTitle}

		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(publishedStream, nil)
		updated, err := serviceImpl.UpdateStream(ctx, streamID, req)

		require.Error(t, err)
		assert.Nil(t, updated)
		assert.Contains(t, err.Error(), "published")
	})
}
func TestStreamServicImpl_CreateStream(t *testing.T) {
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repomock.NewMockStreamRepository(ctrl)
	mockPermissionClient := authmock.NewMockPermissionClient(ctrl)

	mockStorage := mock.NewMockFileStorage(ctrl)
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage)

	t.Run("succesful stream creation", func(t *testing.T) {

		ownerID := uuid.New()

		req := service.CreateStreamRequest{
			Title:       "My Awesome Stream",
			Description: "This is a test stream description",
			Visibility:  models.VisibilityPrivate,
			Tags:        []string{"gaming", "live"},
			OwnerID:     ownerID,
		}

		generatedStreamID := uuid.New()
		mockRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, stream *models.Stream) {
				if stream.ID == uuid.Nil {
					stream.ID = generatedStreamID
				}
				stream.CreatedAt = time.Now()
				stream.UpdatedAt = time.Now()
			}).
			Return(nil)

		mockPermissionClient.EXPECT().
			AddPolicy(gomock.Any(), ownerID.String(), fmt.Sprintf("stream/%s", generatedStreamID.String()), "write").
			Return(true, nil)
		mockPermissionClient.EXPECT().
			AddPolicy(gomock.Any(), ownerID.String(), fmt.Sprintf("stream/%s", generatedStreamID.String()), "read").
			Return(true, nil)
		mockPermissionClient.EXPECT().
			AddPolicy(gomock.Any(), ownerID.String(), fmt.Sprintf("stream/%s", generatedStreamID.String()), "delete").
			Return(true, nil)
		stream, err := serviceImpl.CreateStream(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, stream)
		assert.Equal(t, req.Title, stream.Title)
		assert.Equal(t, req.Description, stream.Description)
		assert.Equal(t, ownerID, stream.OwnerID)
		assert.Equal(t, models.StatusDraft, stream.Status)
		assert.Equal(t, models.VisibilityPrivate, stream.Visibility)
	})

	t.Run("empty title should fail", func(t *testing.T) {
		req := service.CreateStreamRequest{
			Title:   "",
			OwnerID: uuid.New(),
		}

		stream, err := serviceImpl.CreateStream(ctx, req)

		require.Error(t, err)
		assert.Nil(t, stream)
	})

	t.Run("repository error should propagate", func(t *testing.T) {
		req := service.CreateStreamRequest{
			Title:   "Test stream",
			OwnerID: uuid.New(),
		}

		mockRepo.EXPECT().Create(gomock.Any(), gomock.AssignableToTypeOf(&models.Stream{})).Return(assert.AnError)
		stream, err := serviceImpl.CreateStream(ctx, req)

		require.Error(t, err)
		assert.Nil(t, stream)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestStreamServiceImpl_StartStreamUpload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	mockRepo := repomock.NewMockStreamRepository(ctrl)
	mockPerms := authmock.NewMockPermissionClient(ctrl)
	mockStorage := mock.NewMockFileStorage(ctrl)

	service := service.NewStreamServiceImpl(mockRepo, mockPerms, mockStorage)

	streamID := uuid.New()
	userID := uuid.New()

	existingStream := &models.Stream{
		BaseModel: models.BaseModel{ID: streamID},
		OwnerID:   userID,
		Title:     "TestStrema",
		Status:    models.StatusDraft,
		Storage:   datatypes.JSON("{}"),
	}

	t.Run("succesfully generates upload URL for draft stream", func(t *testing.T) {
		mockRepo.EXPECT().Read(ctx, streamID).Return(existingStream, nil)
		expectedURL, _ := url.Parse("https://minio.localhost:9000/test-bucket/videos/...?X-Amz-...")
		expectedKey := "videos/" + userID.String() + "/" + streamID.String() + "/original"
		mockStorage.EXPECT().
			GeneratePresignedURL(ctx, expectedKey, gomock.Any()).
			Return(expectedURL, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, updatedStream *models.Stream) error {
			if updatedStream.Status != models.StatusProcessing {
				t.Errorf("Expected status 'processing', got %s", updatedStream.Status)
			}
			var storageInfo models.StreamStorage
			if err := json.Unmarshal(updatedStream.Storage, &storageInfo); err != nil {
				t.Errorf("Failed to unmarshal storage: %v", err)
			}
			if storageInfo.Key != expectedKey {
				t.Errorf("Expected key %s, got %s", expectedKey, storageInfo.Key)
			}
			if storageInfo.Provider != "minio" {
				t.Errorf("Expected provider 'minio', got %s", storageInfo.Provider)
			}
			return nil
		})
		result, err := service.StartStreamUpload(ctx, streamID)
		if err != nil {
			t.Fatalf("StartStreamUpload failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected UploadInfo, got nil")
		}
		if result.UploadURL != expectedURL.String() {
			t.Errorf("Expected URL %s, got %s", expectedURL, result.UploadURL)
		}
		if result.StreamID != streamID {
			t.Errorf("Expected streamID %s, got %s", streamID, result.StreamID)
		}
	})

	t.Run("return errors for published stream", func(t *testing.T) {
		publishedStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Status:    models.StatusPublished,
			Storage:   datatypes.JSON("{}"),
		}

		mockRepo.EXPECT().Read(ctx, streamID).Return(publishedStream, nil)
		result, err := service.StartStreamUpload(ctx, streamID)
		if err == nil {
			t.Fatal("Expected error for published stream")
		}
		if result != nil {
			t.Fatalf("Expected nil result when error occurs")
		}
		expectedErr := "cannot upload to published stream"
		if err.Error() != expectedErr {
			t.Errorf("Expected err %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("returns error when stream not found", func(t *testing.T) {
		mockRepo.EXPECT().Read(ctx, streamID).Return(nil, repository.ErrNotFound)
		result, err := service.StartStreamUpload(ctx, streamID)
		if err == nil {
			t.Fatal("Expected error when stream not found")
		}
		if result != nil {
			t.Error("Expected nil result when stream not found")
		}
		if err.Error() != "failed to get stream: stream not found" {
			t.Errorf("Expected 'stream not found' error, got '%v'", err)
		}
	})

	t.Run("returns error when storage fails", func(t *testing.T) {
		mockRepo.EXPECT().Read(ctx, streamID).Return(existingStream, nil)

		storageErr := storage.ErrStorage
		mockStorage.EXPECT().GeneratePresignedURL(ctx, gomock.Any(), gomock.Any()).Return(nil, storageErr)
		result, err := service.StartStreamUpload(ctx, streamID)
		if err == nil {
			t.Fatal("Expected error when storage fails")
		}

		if result != nil {
			t.Error("Expected nil result when storage fails")
		}

		if err.Error() != "failed to generate upload URL: storage error" {
			t.Errorf("Expected storage error, got '%v'", err)
		}
	})

	t.Run("returns error when cannot update stream", func(t *testing.T) {
		mockRepo.EXPECT().Read(ctx, streamID).Return(existingStream, nil)
		mockStorage.EXPECT().
			GeneratePresignedURL(ctx, gomock.Any(), gomock.Any()).
			Return(func() *url.URL {
				u, _ := url.Parse("https://example.com/upload")
				return u
			}(), nil)
		updateErr := fmt.Errorf("database error")
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(updateErr)
		result, err := service.StartStreamUpload(ctx, streamID)
		if err == nil {
			t.Fatal("Expected error when update fails")
		}
		if result != nil {
			t.Error("Expected nil result when update fails")
		}
		if err.Error() != "failed to update stream: database error" {
			t.Errorf("Expected update error, got '%v'", err)
		}
	})
}

func TestStreamServiceImpl_UploadVideo(t *testing.T) {

	ctx := context.Background()

	t.Run("successful video upload", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage)
		streamID := uuid.New()
		userID := uuid.New()
		fileName := "test.mp4"
		fileSize := int64(1024 * 1024)
		fileData := []byte("fake video data")

		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Title:     "Test stream",
			Status:    models.StatusDraft,
			Storage:   datatypes.JSON("{}"),
		}

		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)

		mockStorage.EXPECT().
			Upload(ctx, gomock.Any(), gomock.Any(), fileSize).
			DoAndReturn(func(ctx context.Context, path string, reader io.Reader, size int64) error {
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, fileData, data)
				assert.Contains(t, path, "streams/"+userID.String())
				assert.Contains(t, path, streamID.String())
				return nil
			})
		mockStorage.EXPECT().GetBucketName().Return("bucketname")
		mockRepo.EXPECT().
			Update(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, s *models.Stream) error {
				assert.Equal(t, models.StatusProcessing, s.Status)
				var storageInfo models.StreamStorage
				err := json.Unmarshal(s.Storage, &storageInfo)
				require.NoError(t, err)
				assert.Equal(t, fileName, storageInfo.Filename)
				assert.NotEmpty(t, storageInfo.Bucket)

				var streamMeta models.StreamMetadata
				err = json.Unmarshal(s.Metadata, &streamMeta)
				require.NoError(t, err)
				assert.Equal(t, fileSize, streamMeta.Size)

				assert.Contains(t, storageInfo.Key, userID.String())
				assert.Contains(t, storageInfo.Key, streamID.String())
				return nil
			})

		req := service.UploadVideoRequest{
			StreamID: streamID,
			UserID:   userID,
			File:     bytes.NewReader(fileData),
			FileName: fileName,
			Size:     fileSize,
		}
		err := serviceImpl.UploadVideo(ctx, req)
		require.NoError(t, err)
	})

	t.Run("stream not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepoo := repomock.NewMockStreamRepository(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)

		serviceImpl := service.NewStreamServiceImpl(mockRepoo, mockPerm, mockStorage)

		streamID := uuid.New()
		userID := uuid.New()

		mockRepoo.EXPECT().Read(ctx, streamID).Return(nil, gorm.ErrRecordNotFound)
		req := service.UploadVideoRequest{
			StreamID: streamID,
			UserID:   userID,
			File:     bytes.NewReader(nil),
			Size:     100,
		}
		err := serviceImpl.UploadVideo(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stream not found")
	})

	t.Run("cannot upload to published stream", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepoo := repomock.NewMockStreamRepository(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)

		serviceImpl := service.NewStreamServiceImpl(mockRepoo, mockPerm, mockStorage)

		streamID := uuid.New()
		userID := uuid.New()

		stream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Status:    models.StatusPublished,
		}
		mockRepoo.EXPECT().Read(ctx, streamID).Return(stream, nil)
		req := service.UploadVideoRequest{
			StreamID: streamID,
			UserID:   userID,
			File:     bytes.NewReader(nil),
			FileName: "test.mp4",
			Size:     100,
		}

		err := serviceImpl.UploadVideo(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot upload")
		assert.Contains(t, err.Error(), "published")
	})
	t.Run("storage upload fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage)
		streamID := uuid.New()
		userID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Status:    models.StatusDraft,
		}
		mockRepo.EXPECT().Read(ctx, streamID).Return(stream, nil)
		storageErr := fmt.Errorf("storage error: disk full")
		mockStorage.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(storageErr)
		req := service.UploadVideoRequest{
			StreamID: streamID,
			UserID:   userID,
			File:     bytes.NewReader(nil),
			FileName: "test.mp4",
			Size:     100,
		}
		err := serviceImpl.UploadVideo(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage")
	})

	t.Run("stream update fails after successful upload", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage)
		mockStorage.EXPECT().GetBucketName().Return("bucketname")
		streamID := uuid.New()
		userID := uuid.New()

		stream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Status:    models.StatusDraft,
			Storage:   datatypes.JSON("{}"),
		}
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(stream, nil)

		mockStorage.EXPECT().
			Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)
		updateErr := fmt.Errorf("database error: connection lost")
		mockRepo.EXPECT().
			Update(ctx, gomock.Any()).
			Return(updateErr)
		mockStorage.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			Return(nil)
		req := service.UploadVideoRequest{
			StreamID: streamID,
			UserID:   userID,
			File:     bytes.NewReader(nil),
			FileName: "test.mp4",
			Size:     100,
		}

		err := serviceImpl.UploadVideo(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update stream")
	})

}

func TestStreamServiceImpl_GenerateDownloadURL(t *testing.T) {
	ctx := context.Background()

	t.Run("successful URL generation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage)

		streamID := uuid.New()
		userID := uuid.New()

		storageInfo := models.StreamStorage{
			Provider: "minio",
			Key:      "streams/user-id/videos/file-key.mp4",
			Filename: "video.mp4",
			Bucket:   "streams",
		}
		storageJSON, _ := json.Marshal(storageInfo)

		stream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Status:    models.StatusReady,
			Storage:   datatypes.JSON(storageJSON),
		}

		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(stream, nil)

		expectedURL := "https://storage.example.com/streams/user-id/videos/file-key.mp4?signature=..."
		mockStorage.EXPECT().
			GeneratePresignedURL(ctx, storageInfo.Key, gomock.Any()).
			Return(&url.URL{
				Scheme:   "https",
				Host:     "storage.example.com",
				Path:     "/streams/user-id/videos/file-key.mp4",
				RawQuery: "signature=...",
			}, nil)

		urlStr, expiresAt, err := serviceImpl.GenerateDownloadURL(ctx, streamID)

		require.NoError(t, err)
		assert.Equal(t, expectedURL, urlStr)
		assert.True(t, expiresAt.After(time.Now()))
	})

	t.Run("stream not found", func(t *testing.T) {
		// ...
	})

	t.Run("access denied", func(t *testing.T) {
		// ...
	})

	t.Run("stream not ready", func(t *testing.T) {
		// ...
	})
}
