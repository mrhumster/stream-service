package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	authmock "github.com/mrhumster/identity-service/pkg/auth/mock"
	"github.com/mrhumster/stream-service/internal/domain/models"
	queuemock "github.com/mrhumster/stream-service/internal/queue/mock"
	"github.com/mrhumster/stream-service/internal/repository"
	repomock "github.com/mrhumster/stream-service/internal/repository/mock"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/storage/mock"
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)

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
			Return(expectedStreams, int64(2), nil)

		streams, _, err := serviceImpl.ListUserStreams(ctx, userID)

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
	mockQueue := queuemock.NewMockTaskDistributor(ctrl)

	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)
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
		mockRepo.EXPECT().List(gomock.Any(), filter).Return(expectedStreams, int64(2), nil).Times(1)

		streams, _, err := serviceImpl.ListStreams(ctx, filter)

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

		mockRepo.EXPECT().List(gomock.Any(), filter).Return(expectedStreams, int64(1), nil).Times(1)
		streams, _, err := serviceImpl.ListStreams(ctx, filter)
		require.NoError(t, err)
		require.Len(t, streams, 1)
		assert.Equal(t, "Gaming Stream", streams[0].Title)
	})

	t.Run("empty result", func(t *testing.T) {
		filter := repository.StreamFilter{Limit: 10}
		mockRepo.EXPECT().List(gomock.Any(), filter).Return([]*models.Stream{}, int64(0), nil).Times(1)
		streams, _, err := serviceImpl.ListStreams(ctx, filter)
		require.NoError(t, err)
		assert.Empty(t, streams)
	})

	t.Run("repository error propagate", func(t *testing.T) {
		filter := repository.StreamFilter{Limit: 10}
		mockRepo.EXPECT().List(gomock.Any(), filter).Return(nil, int64(2), assert.AnError).Times(1)
		streams, _, err := serviceImpl.ListStreams(ctx, filter)
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
	mockQueue := queuemock.NewMockTaskDistributor(ctrl)
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)

	t.Run("successful delete", func(t *testing.T) {
		ownerID := uuid.New()
		generatedStreamID := uuid.New()

		streamForDelete := &models.Stream{
			Description: "Drasft",
			Title:       "Stream test",
			OwnerID:     ownerID,
			Status:      models.StatusProcessing,
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		srv := service.NewStreamServiceImpl(r, p, s, mockQueue, nil)
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
	t.Run("successful delete with terminate task", func(t *testing.T) {
		ownerID := uuid.New()
		generatedStreamID := uuid.New()

		taskID := "task-id"
		streamProcessing := models.StreamProcessing{
			Progress: 50,
			Steps:    []string{"convertation"},
			Error:    nil,
			TaskID:   &taskID,
		}
		streamProcessingJSON, _ := json.Marshal(streamProcessing)

		streamForDelete := &models.Stream{
			Description: "Drasft",
			Title:       "Stream test",
			OwnerID:     ownerID,
			Status:      models.StatusProcessing,
		}

		streamForDelete.ID = generatedStreamID

		streamForDelete.Processing = datatypes.JSON(streamProcessingJSON)
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
		mockQueue.EXPECT().TerminateTask(gomock.Any(), gomock.Any()).Return(nil)
		err := serviceImpl.DeleteStream(ctx, generatedStreamID)
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
	mockQueue := queuemock.NewMockTaskDistributor(ctrl)

	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)
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
	mockQueue := queuemock.NewMockTaskDistributor(ctrl)
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil)

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

func TestStreamServiceImpl_UploadVideo(t *testing.T) {
	ctx := context.Background()

	t.Run("successful video upload", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
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
		taskID := "task-id"
		mockQueue.EXPECT().
			DistributeVideoTranscoding(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&taskID, nil).
			Times(1)
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepoo, mockPerm, mockStorage, mockQueue, nil)

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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepoo, mockPerm, mockStorage, mockQueue, nil)

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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
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
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
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

		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)

		streamID := uuid.New()
		userID := uuid.New()

		storageInfo := models.StreamStorage{
			Provider: "minio",
			Key:      "streams/user-id/videos/file-key.mp4",
			Filename: "video.mp4",
			Bucket:   "streams",
		}

		streamMeta := models.StreamMetadata{
			Size: int64(100),
		}

		metaJSON, _ := json.Marshal(streamMeta)

		storageJSON, _ := json.Marshal(storageInfo)

		stream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Status:    models.StatusReady,
			Storage:   datatypes.JSON(storageJSON),
			Metadata:  datatypes.JSON(metaJSON),
		}

		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(stream, nil)

		expectedURL := "https://storage.example.com/streams/user-id/videos/file-key.mp4?signature=..."
		mockStorage.EXPECT().
			GeneratePresignedURL(ctx, storageInfo.Key, storageInfo.Filename, gomock.Any()).
			Return(&url.URL{
				Scheme:   "https",
				Host:     "storage.example.com",
				Path:     "/streams/user-id/videos/file-key.mp4",
				RawQuery: "signature=...",
			}, nil)

		resp, err := serviceImpl.GenerateDownloadURL(ctx, streamID)

		require.NoError(t, err)
		assert.Equal(t, expectedURL, resp.DownloadURL.String())
		assert.True(t, resp.ExpiresAt.After(time.Now()))
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

func TestStreamServiceImpl_StartStreamUpload(t *testing.T) {
	ctx := context.Background()

	t.Run("success start upload", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "minio-session-123"
		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			OwnerID:   userID,
			Status:    models.StatusDraft,
		}
		mockRepo.EXPECT().Read(ctx, streamID).Return(existingStream, nil)
		mockStorage.EXPECT().GetBucketName().Return("streams")
		mockStorage.EXPECT().InitMultipart(
			ctx, gomock.Any()).Return(uploadID, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Do(func(_ context.Context, s *models.Stream) {
			var storageInfo models.StreamStorage
			json.Unmarshal(s.Storage, &storageInfo)
			assert.Equal(t, uploadID, storageInfo.UploadID)
			assert.Equal(t, models.StatusUploading, s.Status)
			assert.NotEmpty(t, storageInfo.Key)
		}).Return(nil)
		req := service.StartUploadRequest{
			StreamID:  streamID,
			UserID:    userID,
			Filename:  "video.mp4",
			TotalSize: int64(100),
		}
		info, err := svc.StartStreamUpload(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, info.UploadID, uploadID)
	})

	t.Run("stream not found error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
		mockRepo.EXPECT().Read(ctx, gomock.Any()).Return(nil, gorm.ErrRecordNotFound)
		req := service.StartUploadRequest{
			StreamID:  uuid.New(),
			UserID:    uuid.New(),
			Filename:  "video.mp4",
			TotalSize: int64(64),
		}
		_, err := svc.StartStreamUpload(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("repo error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
		mockRepo.EXPECT().Read(ctx, gomock.Any()).Return(nil, errors.New("internal error"))

		req := service.StartUploadRequest{
			StreamID:  uuid.New(),
			UserID:    uuid.New(),
			Filename:  "video.mp4",
			TotalSize: int64(64),
		}
		_, err := svc.StartStreamUpload(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "internal error")
	})

	t.Run("not the owner", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
		expectedStream := &models.Stream{
			OwnerID: uuid.New(),
			Title:   "someone else's stream",
		}
		mockRepo.EXPECT().Read(ctx, gomock.Any()).Return(expectedStream, nil)
		req := service.StartUploadRequest{
			StreamID:  uuid.New(),
			UserID:    uuid.New(),
			Filename:  "video.mp4",
			TotalSize: int64(100),
		}
		_, err := svc.StartStreamUpload(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "forbidden: not a owner")
	})

	t.Run("stream with not right status", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
		userID := uuid.New()
		expectedStream := &models.Stream{
			Title:   "Stream already published",
			Status:  models.StatusPublished,
			OwnerID: userID,
		}
		mockRepo.EXPECT().Read(ctx, gomock.Any()).Return(expectedStream, nil)
		req := service.StartUploadRequest{
			StreamID:  uuid.New(),
			UserID:    userID,
			Filename:  "video.mp4",
			TotalSize: int64(64),
		}
		_, err := svc.StartStreamUpload(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot start upload for stream in status: published")
	})

	t.Run("failed init storage", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)
		userID := uuid.New()
		expectedStream := &models.Stream{
			Title:   "Stream already published",
			Status:  models.StatusDraft,
			OwnerID: userID,
		}
		mockRepo.EXPECT().Read(ctx, gomock.Any()).Return(expectedStream, nil)
		mockStorage.EXPECT().InitMultipart(ctx, gomock.Any()).Return("", errors.New("storage not found"))
		req := service.StartUploadRequest{
			StreamID:  uuid.New(),
			UserID:    userID,
			Filename:  "video.mp4",
			TotalSize: int64(100),
		}
		_, err := svc.StartStreamUpload(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to init storage: storage not found")
	})
	t.Run("failed to update repo after storage init", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockPerm := authmock.NewMockPermissionClient(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil)

		userID := uuid.New()
		streamID := uuid.New()
		uploadID := "UploadID-123"

		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			Status:    models.StatusDraft,
			OwnerID:   userID,
		}
		mockRepo.EXPECT().Read(ctx, streamID).Return(expectedStream, nil)
		mockStorage.EXPECT().InitMultipart(ctx, gomock.Any()).Return(uploadID, nil)
		mockStorage.EXPECT().GetBucketName().Return("bucketName")
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(errors.New("db connection lost"))

		mockStorage.EXPECT().AbortMultipart(ctx, gomock.Any(), uploadID).Return(nil)

		req := service.StartUploadRequest{
			StreamID:  streamID,
			UserID:    userID,
			Filename:  "vide.mp4",
			TotalSize: int64(100),
		}
		_, err := svc.StartStreamUpload(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update stream: db connection lost")
	})
}

func TestStreamServicImpl_UploadPart(t *testing.T) {
	ctx := context.Background()
	t.Run("successful call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "UploadID-123"
		data := strings.NewReader("part 1")
		storageInfo := models.StreamStorage{
			UploadID: uploadID,
			Bucket:   "streams",
			Key:      "key",
			Filename: "video.mp4",
			Provider: "minio",
		}
		storageJSON, _ := json.Marshal(storageInfo)
		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			Storage:   datatypes.JSON(storageJSON),
			OwnerID:   userID,
			Status:    models.StatusUploading,
		}
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)
		mockStor.EXPECT().
			UploadPart(
				ctx,
				gomock.Any(),
				uploadID,
				1,
				gomock.Any(),
				gomock.Any(),
			).
			Return("etag", nil)
		req := service.UploadPartRequest{
			StreamID:   streamID,
			UserID:     userID,
			UploadID:   uploadID,
			PartNumber: 1,
			Data:       data,
		}
		partInfo, err := svc.UploadPart(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, partInfo.PartNumber, req.PartNumber)
		assert.Equal(t, partInfo.ETag, "etag")
	})

	t.Run("it is possible only in state of uploading", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "UploadID-123"
		data := strings.NewReader("part 1")
		storageInfo := models.StreamStorage{
			UploadID: uploadID,
			Bucket:   "streams",
			Key:      "key",
			Filename: "video.mp4",
			Provider: "minio",
		}
		storageJSON, _ := json.Marshal(storageInfo)
		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			Storage:   datatypes.JSON(storageJSON),
			OwnerID:   userID,
			Status:    models.StatusDraft,
		}
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)
		req := service.UploadPartRequest{
			StreamID:   streamID,
			UserID:     userID,
			UploadID:   uploadID,
			PartNumber: 1,
			Data:       data,
		}
		_, err := svc.UploadPart(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot start upload for stream in status: draft")
	})

	t.Run("repos error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "UploadID-123"
		data := strings.NewReader("part 1")
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(nil, fmt.Errorf("not found"))
		req := service.UploadPartRequest{
			StreamID:   streamID,
			UserID:     userID,
			UploadID:   uploadID,
			PartNumber: 1,
			Data:       data,
		}
		_, err := svc.UploadPart(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("error Unmarshal", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "UploadID-123"
		data := strings.NewReader("part 1")
		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			Storage:   datatypes.JSON(``),
			OwnerID:   userID,
			Status:    models.StatusUploading,
		}
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)
		req := service.UploadPartRequest{
			StreamID:   streamID,
			UserID:     userID,
			UploadID:   uploadID,
			PartNumber: 1,
			Data:       data,
		}
		_, err := svc.UploadPart(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error unmarshaling storage info")
	})

	t.Run("Mismatch between request Upload ID and storage Upload ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "UploadID-123"
		data := strings.NewReader("part 1")
		req := service.UploadPartRequest{
			StreamID:   streamID,
			UserID:     userID,
			UploadID:   uploadID,
			PartNumber: 1,
			Data:       data,
		}
		storageInfo := models.StreamStorage{
			UploadID: "UploadID-321",
			Bucket:   "streams",
			Key:      "key",
			Filename: "video.mp4",
			Provider: "minio",
		}
		storageJSON, _ := json.Marshal(storageInfo)
		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			Storage:   datatypes.JSON(storageJSON),
			OwnerID:   userID,
			Status:    models.StatusUploading,
		}
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)
		_, err := svc.UploadPart(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "upload id from request not equal upload id from storage info")
	})
	t.Run("not a owner", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "UploadID-123"
		data := strings.NewReader("part 1")
		req := service.UploadPartRequest{
			StreamID:   streamID,
			UserID:     userID,
			UploadID:   uploadID,
			PartNumber: 1,
			Data:       data,
		}
		storageInfo := models.StreamStorage{
			UploadID: uploadID,
			Bucket:   "streams",
			Key:      "key",
			Filename: "video.mp4",
			Provider: "minio",
		}
		storageJSON, _ := json.Marshal(storageInfo)
		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			Storage:   datatypes.JSON(storageJSON),
			OwnerID:   uuid.New(),
			Status:    models.StatusUploading,
		}
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)
		_, err := svc.UploadPart(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a owner")
	})
	t.Run("storag error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil)
		streamID := uuid.New()
		userID := uuid.New()
		uploadID := "UploadID-123"
		data := strings.NewReader("part 1")
		req := service.UploadPartRequest{
			StreamID:   streamID,
			UserID:     userID,
			UploadID:   uploadID,
			PartNumber: 1,
			Data:       data,
		}
		storageInfo := models.StreamStorage{
			UploadID: uploadID,
			Bucket:   "streams",
			Key:      "key",
			Filename: "video.mp4",
			Provider: "minio",
		}
		storageJSON, _ := json.Marshal(storageInfo)
		existingStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamID},
			Storage:   datatypes.JSON(storageJSON),
			OwnerID:   userID,
			Status:    models.StatusUploading,
		}
		mockRepo.EXPECT().
			Read(ctx, streamID).
			Return(existingStream, nil)
		mockStor.EXPECT().
			UploadPart(
				ctx,
				storageInfo.Key,
				uploadID,
				req.PartNumber,
				req.Data,
				gomock.Any(),
			).
			Return("", fmt.Errorf("storage not ready"))
		_, err := svc.UploadPart(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error upload part to strage: storage not ready")
	})
}

func TestStreamServiceImpl_PublishStream(t *testing.T) {
	t.Run("success published stream", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "unpublished stream",
			Status: models.StatusDraft,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(stream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) {
				assert.Equal(t, stream.Status, models.StatusPublished)
			}).
			Return(nil)
		ctx := context.Background()
		err := svc.PublishStream(ctx, streamUUID)
		require.NoError(t, err)
	})

	t.Run("read repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(nil, fmt.Errorf("read error"))
		ctx := context.Background()
		err := svc.PublishStream(ctx, streamUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
	})
	t.Run("update stream error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "unpublished stream",
			Status: models.StatusDraft,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(stream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) {
				assert.Equal(t, stream.Status, models.StatusPublished)
			}).
			Return(fmt.Errorf("update error"))
		ctx := context.Background()
		err := svc.PublishStream(ctx, streamUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update error")
	})
}

func TestStreamServiceImpl_UnpublishStream(t *testing.T) {
	t.Run("success unpublished stream", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "published stream",
			Status: models.StatusPublished,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(stream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) {
				assert.Equal(t, stream.Status, models.StatusDraft)
			}).
			Return(nil)
		ctx := context.Background()
		err := svc.UnpublishStream(ctx, streamUUID)
		require.NoError(t, err)
	})

	t.Run("read repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(nil, fmt.Errorf("read error"))
		ctx := context.Background()
		err := svc.UnpublishStream(ctx, streamUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
	})
	t.Run("update stream error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "unpublished stream",
			Status: models.StatusPublished,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(stream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) {
				assert.Equal(t, stream.Status, models.StatusDraft)
			}).
			Return(fmt.Errorf("update error"))
		ctx := context.Background()
		err := svc.UnpublishStream(ctx, streamUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update error")
	})
}

func TestStreamServiceImpl_UpdateStreamStatus(t *testing.T) {
	t.Run("success update stream status", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "published stream",
			Status: models.StatusPublished,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(stream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) {
				assert.Equal(t, stream.Status, models.StatusError)
			}).
			Return(nil)
		ctx := context.Background()
		err := svc.UpdateStreamStatus(ctx, streamUUID, models.StatusError)
		require.NoError(t, err)
	})

	t.Run("read repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(nil, fmt.Errorf("read error"))
		ctx := context.Background()
		err := svc.UpdateStreamStatus(ctx, streamUUID, models.StatusError)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
	})
	t.Run("update stream error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)

		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "unpublished stream",
			Status: models.StatusDraft,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(stream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) {
				assert.Equal(t, stream.Status, models.StatusError)
			}).
			Return(fmt.Errorf("update error"))
		ctx := context.Background()
		err := svc.UpdateStreamStatus(ctx, streamUUID, models.StatusError)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update error")
	})
}

func TestStreamServiceImpl_CompleteStreamUpload(t *testing.T) {
	t.Run("success complete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "1234",
			},
		}
		svcReq := service.CompleteStreamUploadRequest{
			StreamID: streamUUID,
			UserID:   userUUID,
			Parts:    parts,
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil).Times(1)

		mockStor.EXPECT().
			CompleteMultipart(
				gomock.Any(),
				storageInfo.Key,
				storageInfo.UploadID,
				parts).
			Return(nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Return(nil)
		taskID := "task-1"
		mockQueue.EXPECT().
			DistributeVideoTranscoding(
				gomock.Any(),
				streamUUID,
				storageInfo.Key).
			Return(&taskID, nil)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)

		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Return(nil)
		err = svc.CompleteStreamUpload(ctx, svcReq)
		require.NoError(t, err)
	})

	t.Run("repository read error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "1234",
			},
		}
		svcReq := service.CompleteStreamUploadRequest{
			StreamID: streamUUID,
			UserID:   userUUID,
			Parts:    parts,
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(nil, fmt.Errorf("repo read error"))

		err = svc.CompleteStreamUpload(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "repo read error")
	})
	t.Run("not owner error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "1234",
			},
		}
		svcReq := service.CompleteStreamUploadRequest{
			StreamID: streamUUID,
			UserID:   userUUID,
			Parts:    parts,
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: uuid.New(),
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)
		err = svc.CompleteStreamUpload(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "forbidden: not a owner")
	})
	t.Run("not ready status error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "1234",
			},
		}
		svcReq := service.CompleteStreamUploadRequest{
			StreamID: streamUUID,
			UserID:   userUUID,
			Parts:    parts,
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusDraft,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)
		err = svc.CompleteStreamUpload(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot start upload for stream in status: draft")
	})
	t.Run("complete multipart error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "1234",
			},
		}
		svcReq := service.CompleteStreamUploadRequest{
			StreamID: streamUUID,
			UserID:   userUUID,
			Parts:    parts,
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)

		mockStor.EXPECT().
			CompleteMultipart(
				gomock.Any(),
				storageInfo.Key,
				storageInfo.UploadID,
				parts).
			Return(fmt.Errorf("complete error"))
		err = svc.CompleteStreamUpload(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "complete error")
	})
	t.Run("update repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "1234",
			},
		}
		svcReq := service.CompleteStreamUploadRequest{
			StreamID: streamUUID,
			UserID:   userUUID,
			Parts:    parts,
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)

		mockStor.EXPECT().
			CompleteMultipart(
				gomock.Any(),
				storageInfo.Key,
				storageInfo.UploadID,
				parts).
			Return(nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Return(fmt.Errorf("update repo error"))
		err = svc.CompleteStreamUpload(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update repo error")
	})
	t.Run("queue error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "1234",
			},
		}
		svcReq := service.CompleteStreamUploadRequest{
			StreamID: streamUUID,
			UserID:   userUUID,
			Parts:    parts,
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)

		mockStor.EXPECT().
			CompleteMultipart(
				gomock.Any(),
				storageInfo.Key,
				storageInfo.UploadID,
				parts).
			Return(nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Return(nil)
		mockQueue.EXPECT().
			DistributeVideoTranscoding(
				gomock.Any(),
				streamUUID,
				storageInfo.Key).
			Return(nil, fmt.Errorf("queue error"))
		mockRepo.EXPECT().Read(gomock.Any(), gomock.Any()).Return(expectedStream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
		err = svc.CompleteStreamUpload(ctx, svcReq)
		require.NoError(t, err)
	})
}

func TestStreamServiceImpl_UpdateStreamProcessing(t *testing.T) {
	t.Run("success update stream processing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		taskID := "task-id"
		svcReq := &service.UpdateStreamProcessingRequest{
			StreamUUID: streamUUID,
			Processing: models.StreamProcessing{
				Progress: int(100),
				Steps:    []string{"convert"},
				Error:    nil,
				TaskID:   &taskID,
			},
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Do(func(ctx context.Context, stream *models.Stream) {
				require.Contains(t, stream.Processing.String(), "100")
				require.Contains(t, stream.Processing.String(), "task-id")
			}).
			Return(nil)
		err = svc.UpdateStreamProcessing(ctx, svcReq)
		require.NoError(t, err)
	})
	t.Run("read repo error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		svcReq := &service.UpdateStreamProcessingRequest{
			StreamUUID: streamUUID,
			Processing: models.StreamProcessing{
				Progress: int(100),
				Steps:    []string{"convert"},
				Error:    nil,
			},
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		taskID := "task-id"
		expectedStream.UpdateProcessing(1, []string{"convert"}, nil, &taskID)
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(nil, fmt.Errorf("read error"))
		err = svc.UpdateStreamProcessing(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
	})
	t.Run("update repo error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		svcReq := &service.UpdateStreamProcessingRequest{
			StreamUUID: streamUUID,
			Processing: models.StreamProcessing{
				Progress: int(100),
				Steps:    []string{"convert"},
				Error:    nil,
			},
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Do(func(ctx context.Context, stream *models.Stream) {
				require.Contains(t, stream.Processing.String(), "100")
			}).
			Return(fmt.Errorf("update repo error"))
		err = svc.UpdateStreamProcessing(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update repo error")
	})
}

func TestStreamServiceImpl_UpdateStreamMetadata(t *testing.T) {
	t.Run("success update stream metadata", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		svcReq := &service.UpdateStreamMetadataRequest{
			StreamUUID: streamUUID,
			Metadata: models.StreamMetadata{
				Duration:   100,
				Size:       int64(1024),
				Format:     "mp4",
				Resolution: "1080",
			},
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Do(func(ctx context.Context, stream *models.Stream) {
				require.Contains(t, stream.Metadata.String(), "1080")
			}).
			Return(nil)
		err = svc.UpdateStreamMetadata(ctx, svcReq)
		require.NoError(t, err)
	})
	t.Run("read repo error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		svcReq := &service.UpdateStreamMetadataRequest{
			StreamUUID: streamUUID,
			Metadata: models.StreamMetadata{
				Duration:   100,
				Size:       int64(1024),
				Format:     "mp4",
				Resolution: "1080",
			},
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(nil, fmt.Errorf("read error"))
		err = svc.UpdateStreamMetadata(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
	})
	t.Run("update repo error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		svcReq := &service.UpdateStreamMetadataRequest{
			StreamUUID: streamUUID,
			Metadata: models.StreamMetadata{
				Duration:   100,
				Size:       int64(1024),
				Format:     "mp4",
				Resolution: "1080",
			},
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-739",
		}
		err := expectedStream.SetStorageInfo(storageInfo)
		assert.NoError(t, err)
		mockRepo.EXPECT().
			Read(gomock.Any(), streamUUID).
			Return(expectedStream, nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Do(func(ctx context.Context, stream *models.Stream) {
				require.Contains(t, stream.Metadata.String(), "100")
			}).
			Return(fmt.Errorf("update repo error"))
		err = svc.UpdateStreamMetadata(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update repo error")
	})
}

func TestStreamServiceImpl_GetFileByKey(t *testing.T) {
	t.Run("success get m3u8 file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusReady,
		}
		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   "index.m3u8",
		}
		path := path.Join("processed", svcReq.StreamUUID.String(), svcReq.FileName)
		dummyReadCloser := io.NopCloser(strings.NewReader("some data"))
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().
			Download(ctx, path).
			Return(dummyReadCloser, int64(100), nil)
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.NoError(t, err)
		assert.Equal(t, svcRes.ContentType, "application/x-mpegURL")
	})

	t.Run("success get ts file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusReady,
		}
		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   "part.ts",
		}
		path := path.Join("processed", svcReq.StreamUUID.String(), svcReq.FileName)
		dummyReadCloser := io.NopCloser(strings.NewReader("some data"))
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().
			Download(ctx, path).
			Return(dummyReadCloser, int64(100), nil)
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.NoError(t, err)
		assert.Equal(t, svcRes.ContentType, "video/MP2T")
	})

	t.Run("success get file", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusReady,
		}
		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   "binaryfile",
		}
		path := path.Join("processed", svcReq.StreamUUID.String(), svcReq.FileName)
		dummyReadCloser := io.NopCloser(strings.NewReader("some data"))
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().
			Download(ctx, path).
			Return(dummyReadCloser, int64(100), nil)
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.NoError(t, err)
		assert.Equal(t, svcRes.ContentType, "application/octet-stream")
	})

	t.Run("read repo error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   "binaryfile",
		}
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(nil, fmt.Errorf("read error"))
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
		assert.Nil(t, svcRes)
	})
	t.Run("stream status dont correct", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusUploading,
		}
		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   "binaryfile",
		}
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "you can't watch a stream with the status uploading")
		assert.Nil(t, svcRes)
	})
	t.Run("storage error propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(
			mockRepo,
			mockAuth,
			mockStor,
			mockQueue,
			nil,
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:   "Stream",
			OwnerID: userUUID,
			Status:  models.StatusReady,
		}
		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   "index.m3u8",
		}
		path := path.Join("processed", svcReq.StreamUUID.String(), svcReq.FileName)
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().
			Download(ctx, path).
			Return(nil, int64(0), fmt.Errorf("download error"))
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download error")
		assert.Nil(t, svcRes)
	})
}
