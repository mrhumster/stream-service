package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authmock "github.com/mrhumster/identity-service/pkg/auth/mock"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/domain/models"
	queuemock "github.com/mrhumster/stream-service/internal/queue/mock"
	"github.com/mrhumster/stream-service/internal/repository"
	repomock "github.com/mrhumster/stream-service/internal/repository/mock"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/storage"
	"github.com/mrhumster/stream-service/internal/storage/mock"
	wssmock "github.com/mrhumster/stream-service/internal/wss/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func srvCfg() *config.Server {
	return &config.Server{
		KeepOriginalFile: false,
	}
}

func TestStreamServiceImpl_ListUserStreams(t *testing.T) {
	ctx := context.Background()

	t.Run("list user streams", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())

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

		streams, total, err := serviceImpl.ListUserStreams(ctx, userID, repository.StreamFilter{Limit: 50, Offset: 25})

		require.NoError(t, err)
		require.Len(t, streams, 2)
		require.Equal(t, int64(2), total)
	})

	t.Run("passes limit and offset into filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())

		userID := uuid.New()

		mockRepo.EXPECT().
			List(
				gomock.Any(),
				gomock.Cond(func(x interface{}) bool {
					f, ok := x.(repository.StreamFilter)
					return ok && f.OwnerID != nil && *f.OwnerID == userID && f.Limit == 50 && f.Offset == 25
				}),
			).
			Return([]*models.Stream{}, int64(0), nil)

		_, _, err := serviceImpl.ListUserStreams(ctx, userID, repository.StreamFilter{Limit: 50, Offset: 25})
		require.NoError(t, err)
	})

	t.Run("passes faces and sort into filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())

		userID := uuid.New()
		faces := true

		mockRepo.EXPECT().
			List(
				gomock.Any(),
				gomock.Cond(func(x interface{}) bool {
					f, ok := x.(repository.StreamFilter)
					return ok && f.OwnerID != nil && *f.OwnerID == userID &&
						f.FacesDetected != nil && *f.FacesDetected &&
						f.SortBy == "title" && f.SortOrder == "asc"
				}),
			).
			Return([]*models.Stream{}, int64(0), nil)

		_, _, err := serviceImpl.ListUserStreams(ctx, userID, repository.StreamFilter{FacesDetected: &faces, SortBy: "title", SortOrder: "asc"})
		require.NoError(t, err)
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

	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
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
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())

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
		srv := service.NewStreamServiceImpl(r, p, s, mockQueue, nil, srvCfg())
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
		thumbKey := fmt.Sprintf("thumbnails/%s.jpg", streamID.String())
		s.EXPECT().Delete(gomock.Any(), thumbKey).Return(nil)
		err = srv.DeleteStream(ctx, streamID)
		require.NoError(t, err)
	})
	t.Run("successful delete with terminate task", func(t *testing.T) {
		ownerID := uuid.New()
		generatedStreamID := uuid.New()

		taskID := "task-id"
		streamProcessing := models.StreamProcessingTask{
			TaskType: models.TaskTypeTranscode,
			Progress: 50,
			Steps:    []string{"convertation"},
			Error:    nil,
			TaskID:   &taskID,
		}
		streamProcessingJSON, _ := json.Marshal([]models.StreamProcessingTask{streamProcessing})

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

	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
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

	t.Run("update stream rotation preserves other metadata", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
		streamID := uuid.New()
		existingStream := &models.Stream{
			Title:    "Test Stream",
			OwnerID:  uuid.New(),
			Status:   models.StatusReady,
			Metadata: datatypes.JSON(`{"duration":12,"size":3456,"format":"hls","resolution":"1280x720","camera":"PixelCam"}`),
		}
		existingStream.ID = streamID

		rotation := 90
		req := service.UpdateStreamRequest{Rotation: &rotation}

		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existingStream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Cond(func(s *models.Stream) bool {
			var meta map[string]any
			json.Unmarshal(s.Metadata, &meta)
			return int(meta["rotation"].(float64)) == 90 &&
				meta["duration"].(float64) == 12 &&
				meta["size"].(float64) == 3456 &&
				meta["format"] == "hls" &&
				meta["resolution"] == "1280x720" &&
				meta["camera"] == "PixelCam"
		})).Return(nil)

		updated, err := serviceImpl.UpdateStream(ctx, streamID, req)
		require.NoError(t, err)
		require.NotNil(t, updated)
	})

	t.Run("update rotation on stream without metadata", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
		mockStorage := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
		streamID := uuid.New()
		existingStream := &models.Stream{
			Title:   "Test Stream",
			OwnerID: uuid.New(),
			Status:  models.StatusDraft,
		}
		existingStream.ID = streamID

		rotation := 270
		req := service.UpdateStreamRequest{Rotation: &rotation}

		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existingStream, nil)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Cond(func(s *models.Stream) bool {
			var meta map[string]any
			json.Unmarshal(s.Metadata, &meta)
			return int(meta["rotation"].(float64)) == 270
		})).Return(nil)

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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())
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
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient, mockStorage, mockQueue, nil, srvCfg())

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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		thumbID := "thumbs-task-id"
		mockQueue.EXPECT().
			DistributeThumbsnailProcessor(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&thumbID, nil).
			Times(1)
		facesID := "faces-task-id"
		mockQueue.EXPECT().
			DistributeFacesProcessor(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&facesID, nil).
			Times(1)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) error {
				assert.Contains(t, s.Processing.String(), models.TaskTypeTranscode)
				assert.Contains(t, s.Processing.String(), models.TaskTypeThumbnail)
				assert.Contains(t, s.Processing.String(), models.TaskTypeFaces)
				return nil
			}).Return(nil)

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
		serviceImpl := service.NewStreamServiceImpl(mockRepoo, mockPerm, mockStorage, mockQueue, nil, srvCfg())

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
		serviceImpl := service.NewStreamServiceImpl(mockRepoo, mockPerm, mockStorage, mockQueue, nil, srvCfg())

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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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

// exportSvc wires the export collaborators onto a service so each case only has
// to declare the behaviour it cares about.
func exportSvc(t *testing.T) (service.StreamService, *repomock.MockStreamRepository, *repomock.MockStreamExportRepository, *mock.MockFileStorage, *queuemock.MockExportTaskDistributor) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repo := repomock.NewMockStreamRepository(ctrl)
	exportRepo := repomock.NewMockStreamExportRepository(ctrl)
	storage := mock.NewMockFileStorage(ctrl)
	exportQueue := queuemock.NewMockExportTaskDistributor(ctrl)

	svc := service.NewStreamServiceImpl(repo, authmock.NewMockPermissionClient(ctrl), storage, queuemock.NewMockTaskDistributor(ctrl), nil, srvCfg())
	svc.WithExportRepository(exportRepo)
	svc.WithExportQueue(exportQueue)
	return svc, repo, exportRepo, storage, exportQueue
}

func exportStream(owner uuid.UUID, status models.StreamStatus) *models.Stream {
	return &models.Stream{
		BaseModel: models.BaseModel{ID: uuid.New()},
		OwnerID:   owner,
		Status:    status,
	}
}

// exportSvcNoRepo is exportSvc without the stream repository, for the worker
// callback which never touches a stream row.
func exportSvcNoRepo(t *testing.T) (service.StreamService, *repomock.MockStreamExportRepository, *mock.MockFileStorage) {
	t.Helper()
	svc, _, exportRepo, storage, _ := exportSvc(t)
	return svc, exportRepo, storage
}

// exportSvcWithHub attaches a hub so the ready notification can be asserted.
func exportSvcWithHub(t *testing.T) (service.StreamService, *repomock.MockStreamExportRepository, *wssmock.MockHub) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	exportRepo := repomock.NewMockStreamExportRepository(ctrl)
	hub := wssmock.NewMockHub(ctrl)

	svc := service.NewStreamServiceImpl(
		repomock.NewMockStreamRepository(ctrl),
		authmock.NewMockPermissionClient(ctrl),
		mock.NewMockFileStorage(ctrl),
		queuemock.NewMockTaskDistributor(ctrl),
		hub,
		srvCfg(),
	)
	svc.WithExportRepository(exportRepo)
	svc.WithExportQueue(queuemock.NewMockExportTaskDistributor(ctrl))
	return svc, exportRepo, hub
}

func TestStreamServiceImpl_RequestStreamExport(t *testing.T) {
	ctx := context.Background()

	t.Run("creates a pending row and queues the worker", func(t *testing.T) {
		svc, repo, exportRepo, _, exportQueue := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(nil, gorm.ErrRecordNotFound)
		exportRepo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, e *models.StreamExport) error {
			assert.Equal(t, models.ExportStatusPending, e.Status)
			assert.Equal(t, stream.ID, e.StreamID)
			assert.Equal(t, owner.String(), e.UserID)
			return nil
		})
		exportQueue.EXPECT().DistributeVideoExport(ctx, stream.ID, owner, "owner@example.com").Return(nil, nil)

		info, err := svc.RequestStreamExport(ctx, stream.ID, owner, "owner@example.com")

		require.NoError(t, err)
		assert.Equal(t, models.ExportStatusPending, info.Status)
	})

	t.Run("ready export is a no-op", func(t *testing.T) {
		svc, repo, exportRepo, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusPublished)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusReady, Size: 4242,
		}, nil)
		// No DistributeVideoExport expectation: gomock fails on an unexpected call.

		info, err := svc.RequestStreamExport(ctx, stream.ID, owner, "owner@example.com")

		require.NoError(t, err)
		assert.Equal(t, models.ExportStatusReady, info.Status)
		assert.Equal(t, int64(4242), info.Size)
	})

	t.Run("in-flight export is not queued twice", func(t *testing.T) {
		svc, repo, exportRepo, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusPending,
		}, nil)

		info, err := svc.RequestStreamExport(ctx, stream.ID, owner, "owner@example.com")

		require.NoError(t, err)
		assert.Equal(t, models.ExportStatusPending, info.Status)
	})

	t.Run("failed export is retried on the same row", func(t *testing.T) {
		svc, repo, exportRepo, _, exportQueue := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		row := &models.StreamExport{
			BaseModel: models.BaseModel{ID: uuid.New()},
			StreamID:  stream.ID,
			UserID:    owner.String(),
			Status:    models.ExportStatusFailed,
			Error:     "mux failed",
		}
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(row, nil)
		exportRepo.EXPECT().Update(ctx, row).DoAndReturn(func(_ context.Context, e *models.StreamExport) error {
			assert.Equal(t, models.ExportStatusPending, e.Status)
			assert.Empty(t, e.Error)
			return nil
		})
		exportQueue.EXPECT().DistributeVideoExport(ctx, stream.ID, owner, "owner@example.com").Return(nil, nil)

		info, err := svc.RequestStreamExport(ctx, stream.ID, owner, "owner@example.com")

		require.NoError(t, err)
		assert.Equal(t, models.ExportStatusPending, info.Status)
	})

	t.Run("non-owner is refused", func(t *testing.T) {
		svc, repo, _, _, _ := exportSvc(t)
		stream := exportStream(uuid.New(), models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)

		_, err := svc.RequestStreamExport(ctx, stream.ID, uuid.New(), "x@example.com")

		require.ErrorIs(t, err, service.ErrStreamForbidden)
	})

	t.Run("unprocessed stream is refused", func(t *testing.T) {
		svc, repo, _, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusProcessing)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)

		_, err := svc.RequestStreamExport(ctx, stream.ID, owner, "owner@example.com")

		require.ErrorIs(t, err, service.ErrStreamNotReady)
	})

	t.Run("queue failure marks the row failed", func(t *testing.T) {
		svc, repo, exportRepo, _, exportQueue := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(nil, gorm.ErrRecordNotFound)
		// The service builds the row itself, so capture it instead of asserting
		// on a pre-made value: the follow-up Update must hit this same record.
		var created *models.StreamExport
		exportRepo.EXPECT().Create(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, e *models.StreamExport) error {
				created = e
				return nil
			})
		exportQueue.EXPECT().DistributeVideoExport(ctx, stream.ID, owner, "owner@example.com").
			Return(nil, errors.New("redis down"))
		// Matched with gomock.Any because gomock binds matcher arguments when the
		// expectation is registered, before Create has run to fill in `created`.
		exportRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, e *models.StreamExport) error {
			assert.Same(t, created, e, "the failure must be recorded on the row that was created")
			assert.Equal(t, models.ExportStatusFailed, e.Status)
			assert.NotEmpty(t, e.Error)
			return nil
		})

		_, err := svc.RequestStreamExport(ctx, stream.ID, owner, "owner@example.com")

		require.Error(t, err)
	})
}

func TestStreamServiceImpl_GetStreamExport(t *testing.T) {
	ctx := context.Background()

	t.Run("never exported reads as pending", func(t *testing.T) {
		svc, repo, exportRepo, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(nil, gorm.ErrRecordNotFound)

		info, err := svc.GetStreamExport(ctx, stream.ID, owner)

		require.NoError(t, err)
		assert.Equal(t, models.ExportStatusPending, info.Status)
		// Pending alone cannot tell the UI "nothing yet" from "building"; this
		// flag is what lets it offer the button in the first place.
		assert.False(t, info.Requested)
	})

	t.Run("a queued export reports pending and requested", func(t *testing.T) {
		svc, repo, exportRepo, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusPending,
		}, nil)

		info, err := svc.GetStreamExport(ctx, stream.ID, owner)

		require.NoError(t, err)
		assert.Equal(t, models.ExportStatusPending, info.Status)
		assert.True(t, info.Requested)
	})

	t.Run("a finished export carries its size", func(t *testing.T) {
		svc, repo, exportRepo, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusReady, Size: 4096,
		}, nil)

		info, err := svc.GetStreamExport(ctx, stream.ID, owner)

		require.NoError(t, err)
		assert.Equal(t, models.ExportStatusReady, info.Status)
		assert.Equal(t, int64(4096), info.Size)
		assert.True(t, info.Requested)
	})

	t.Run("non-owner is refused", func(t *testing.T) {
		svc, repo, _, _, _ := exportSvc(t)
		stream := exportStream(uuid.New(), models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)

		_, err := svc.GetStreamExport(ctx, stream.ID, uuid.New())

		require.ErrorIs(t, err, service.ErrStreamForbidden)
	})
}

func TestStreamServiceImpl_OpenStreamDownload(t *testing.T) {
	ctx := context.Background()
	payload := []byte("fake mp4 bytes")

	t.Run("streams the cached object to the owner", func(t *testing.T) {
		svc, repo, exportRepo, storage, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusPublished)
		stream.Title = "My vacation"
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusReady, Size: int64(len(payload)),
		}, nil)
		storage.EXPECT().Download(ctx, models.ExportObjectKey(stream.ID)).
			Return(io.NopCloser(bytes.NewReader(payload)), int64(len(payload)), nil)

		info, err := svc.OpenStreamDownload(ctx, stream.ID, owner)

		require.NoError(t, err)
		assert.Equal(t, "video/mp4", info.ContentType)
		assert.Equal(t, "My vacation.mp4", info.FileName)
		got, err := io.ReadAll(info.Content)
		require.NoError(t, err)
		assert.Equal(t, payload, got)
		require.NoError(t, info.Content.Close())
	})

	t.Run("title is sanitized into a safe attachment name", func(t *testing.T) {
		svc, repo, exportRepo, storage, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		stream.Title = `../../etc/pass wd"rm -rf|`
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusReady, Size: int64(len(payload)),
		}, nil)
		storage.EXPECT().Download(ctx, models.ExportObjectKey(stream.ID)).
			Return(io.NopCloser(bytes.NewReader(payload)), int64(len(payload)), nil)

		info, err := svc.OpenStreamDownload(ctx, stream.ID, owner)

		require.NoError(t, err)
		assert.Equal(t, "etcpass wdrm -rf.mp4", info.FileName)
		assert.NotContains(t, info.FileName, "..")
		require.NoError(t, info.Content.Close())
	})

	t.Run("empty object is refused", func(t *testing.T) {
		svc, repo, exportRepo, storage, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusReady, Size: 0,
		}, nil)
		storage.EXPECT().Download(ctx, models.ExportObjectKey(stream.ID)).
			Return(io.NopCloser(bytes.NewReader(nil)), int64(0), nil)

		_, err := svc.OpenStreamDownload(ctx, stream.ID, owner)

		require.Error(t, err)
	})

	t.Run("pending export is a conflict, not a 500", func(t *testing.T) {
		svc, repo, exportRepo, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(&models.StreamExport{
			StreamID: stream.ID, Status: models.ExportStatusPending,
		}, nil)

		_, err := svc.OpenStreamDownload(ctx, stream.ID, owner)

		require.ErrorIs(t, err, service.ErrExportPending)
	})

	t.Run("no export row yet", func(t *testing.T) {
		svc, repo, exportRepo, _, _ := exportSvc(t)
		owner := uuid.New()
		stream := exportStream(owner, models.StatusReady)
		repo.EXPECT().Read(ctx, stream.ID).Return(stream, nil)
		exportRepo.EXPECT().ReadByStream(ctx, stream.ID).Return(nil, gorm.ErrRecordNotFound)

		_, err := svc.OpenStreamDownload(ctx, stream.ID, owner)

		require.ErrorIs(t, err, service.ErrExportNotFound)
	})
}

func TestStreamServiceImpl_CompleteStreamExport(t *testing.T) {
	ctx := context.Background()

	t.Run("success marks the row ready", func(t *testing.T) {
		svc, exportRepo, _ := exportSvcNoRepo(t)
		row := &models.StreamExport{
			BaseModel: models.BaseModel{ID: uuid.New()},
			StreamID:  uuid.New(),
			UserID:    uuid.New().String(),
			Status:    models.ExportStatusPending,
		}
		exportRepo.EXPECT().ReadByStream(ctx, row.StreamID).Return(row, nil)
		exportRepo.EXPECT().Update(ctx, row).DoAndReturn(func(_ context.Context, e *models.StreamExport) error {
			assert.Equal(t, models.ExportStatusReady, e.Status)
			assert.Equal(t, int64(999), e.Size)
			assert.Empty(t, e.Error)
			return nil
		})

		require.NoError(t, svc.CompleteStreamExport(ctx, row.StreamID, 999, ""))
	})

	t.Run("error marks the row failed and clears the size", func(t *testing.T) {
		svc, exportRepo, _ := exportSvcNoRepo(t)
		row := &models.StreamExport{
			BaseModel: models.BaseModel{ID: uuid.New()},
			StreamID:  uuid.New(),
			Status:    models.ExportStatusPending,
			Size:      500,
		}
		exportRepo.EXPECT().ReadByStream(ctx, row.StreamID).Return(row, nil)
		exportRepo.EXPECT().Update(ctx, row).DoAndReturn(func(_ context.Context, e *models.StreamExport) error {
			assert.Equal(t, models.ExportStatusFailed, e.Status)
			assert.Equal(t, int64(0), e.Size)
			assert.Equal(t, "ffmpeg exited 1", e.Error)
			return nil
		})

		require.NoError(t, svc.CompleteStreamExport(ctx, row.StreamID, 500, "ffmpeg exited 1"))
	})

	t.Run("unknown stream export is reported", func(t *testing.T) {
		svc, exportRepo, _ := exportSvcNoRepo(t)
		streamID := uuid.New()
		exportRepo.EXPECT().ReadByStream(ctx, streamID).Return(nil, gorm.ErrRecordNotFound)

		require.ErrorIs(t, svc.CompleteStreamExport(ctx, streamID, 1, ""), service.ErrExportNotFound)
	})

	t.Run("ready notifies the owner over the websocket", func(t *testing.T) {
		svc, exportRepo, hub := exportSvcWithHub(t)
		owner := uuid.New()
		row := &models.StreamExport{
			BaseModel: models.BaseModel{ID: uuid.New()},
			StreamID:  uuid.New(),
			UserID:    owner.String(),
			Status:    models.ExportStatusPending,
		}
		exportRepo.EXPECT().ReadByStream(ctx, row.StreamID).Return(row, nil)
		exportRepo.EXPECT().Update(ctx, row).Return(nil)
		hub.EXPECT().SendMessgeToOwner(owner, gomock.Any()).DoAndReturn(func(_ uuid.UUID, data any) {
			env, ok := data.(gin.H)
			require.True(t, ok, "the hub payload must be a gin.H envelope")
			assert.Equal(t, "STREAM_EXPORT_READY", env["type"])
		})

		require.NoError(t, svc.CompleteStreamExport(ctx, row.StreamID, 777, ""))
	})

	t.Run("failed export notifies the owner with the reason", func(t *testing.T) {
		svc, exportRepo, hub := exportSvcWithHub(t)
		row := &models.StreamExport{
			BaseModel: models.BaseModel{ID: uuid.New()},
			StreamID:  uuid.New(),
			UserID:    uuid.New().String(),
			Status:    models.ExportStatusPending,
		}
		exportRepo.EXPECT().ReadByStream(ctx, row.StreamID).Return(row, nil)
		exportRepo.EXPECT().Update(ctx, row).Return(nil)
		hub.EXPECT().SendMessgeToOwner(gomock.Any(), gomock.Any()).DoAndReturn(func(_ uuid.UUID, data any) {
			env, ok := data.(gin.H)
			require.True(t, ok, "the hub payload must be a gin.H envelope")
			assert.Equal(t, "STREAM_EXPORT_FAILED", env["type"])
			// Without the reason the UI can only say "it failed".
			payload, ok := env["payload"].(gin.H)
			require.True(t, ok)
			assert.Equal(t, "boom", payload["error"])
		})

		require.NoError(t, svc.CompleteStreamExport(ctx, row.StreamID, 0, "boom"))
		assert.Equal(t, models.ExportStatusFailed, row.Status)
	})

	t.Run("unparsable owner id does not panic the rpc handler", func(t *testing.T) {
		svc, exportRepo, _ := exportSvcWithHub(t)
		row := &models.StreamExport{
			BaseModel: models.BaseModel{ID: uuid.New()},
			StreamID:  uuid.New(),
			UserID:    "not-a-uuid",
			Status:    models.ExportStatusPending,
		}
		exportRepo.EXPECT().ReadByStream(ctx, row.StreamID).Return(row, nil)
		exportRepo.EXPECT().Update(ctx, row).Return(nil)

		require.NoError(t, svc.CompleteStreamExport(ctx, row.StreamID, 10, ""))
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
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockPerm, mockStorage, mockQueue, nil, srvCfg())

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
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
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
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
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
			srvCfg(),
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "unpublished stream",
			Status: models.StatusReady,
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
			srvCfg(),
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
			srvCfg(),
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "unpublished stream",
			Status: models.StatusReady,
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

	t.Run("not publish stream if he not ready", func(t *testing.T) {
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
			srvCfg(),
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
		ctx := context.Background()
		err := svc.PublishStream(ctx, streamUUID)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrCannotPublish)
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
			srvCfg(),
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
				assert.Equal(t, stream.Status, models.StatusReady)
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
			srvCfg(),
		)
		streamUUID := uuid.New()
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(nil, fmt.Errorf("read error"))
		ctx := context.Background()
		err := svc.UnpublishStream(ctx, streamUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
	})
	t.Run("unpublish non-published stream returns ErrCannotUnpublish", func(t *testing.T) {
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
			srvCfg(),
		)
		streamUUID := uuid.New()
		stream := &models.Stream{
			BaseModel: models.BaseModel{
				ID: streamUUID,
			},
			Title:  "draft stream",
			Status: models.StatusDraft,
		}
		mockRepo.EXPECT().Read(gomock.Any(), streamUUID).Return(stream, nil)
		ctx := context.Background()
		err := svc.UnpublishStream(ctx, streamUUID)
		require.ErrorIs(t, err, service.ErrCannotUnpublish)
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
			srvCfg(),
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
				assert.Equal(t, stream.Status, models.StatusReady)
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
		thumbID := "thumbs-task-1"
		mockQueue.EXPECT().
			DistributeThumbsnailProcessor(
				gomock.Any(),
				streamUUID,
				storageInfo.Key).
			Return(&thumbID, nil)
		facesID := "faces-task-1"
		mockQueue.EXPECT().
			DistributeFacesProcessor(
				gomock.Any(),
				streamUUID,
				storageInfo.Key).
			Return(&facesID, nil)
		mockRepo.EXPECT().
			Update(
				gomock.Any(),
				gomock.Any()).
			Do(func(ctx context.Context, s *models.Stream) error {
				assert.Contains(t, s.Processing.String(), models.TaskTypeTranscode)
				assert.Contains(t, s.Processing.String(), models.TaskTypeThumbnail)
				assert.Contains(t, s.Processing.String(), models.TaskTypeFaces)
				return nil
			}).
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
		mockQueue.EXPECT().
			DistributeThumbsnailProcessor(
				gomock.Any(),
				streamUUID,
				storageInfo.Key).
			Return(nil, nil)
		mockQueue.EXPECT().
			DistributeFacesProcessor(
				gomock.Any(),
				streamUUID,
				storageInfo.Key).
			Return(nil, nil)
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
			srvCfg(),
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		taskID := "task-id"
		svcReq := &service.UpdateStreamProcessingRequest{
			StreamUUID: streamUUID,
			Processing: models.StreamProcessingTask{
				TaskType: models.TaskTypeTranscode,
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
			srvCfg(),
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		svcReq := &service.UpdateStreamProcessingRequest{
			StreamUUID: streamUUID,
			Processing: models.StreamProcessingTask{
				TaskType: models.TaskTypeTranscode,
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
			srvCfg(),
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		userUUID := uuid.New()
		svcReq := &service.UpdateStreamProcessingRequest{
			StreamUUID: streamUUID,
			Processing: models.StreamProcessingTask{
				TaskType: models.TaskTypeTranscode,
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
			srvCfg(),
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
	t.Run("transcoder metadata update preserves saved rotation", func(t *testing.T) {
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
			srvCfg(),
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
			Title:    "Stream",
			OwnerID:  userUUID,
			Status:   models.StatusUploading,
			Metadata: datatypes.JSON(`{"duration":50,"size":2048,"format":"hls","resolution":"720p","camera":"PixelCam","rotation":90}`),
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
			UploadID: "upload-id-740",
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
				var meta map[string]any
				json.Unmarshal(stream.Metadata, &meta)
				assert.Equal(t, 90, int(meta["rotation"].(float64)))
				assert.Equal(t, 100.0, meta["duration"].(float64))
				assert.Equal(t, float64(1024), meta["size"].(float64))
				assert.Equal(t, "1080", meta["resolution"])
				assert.Equal(t, "mp4", meta["format"])
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
			srvCfg(),
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
		assert.ErrorIs(t, err, service.ErrCannotWatch)
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
			srvCfg(),
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
	t.Run("missing ts segment falls back to concat neighbours", func(t *testing.T) {
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
			srvCfg(),
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
			FileName:   "seg_5.ts",
		}
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().
			Download(ctx, path.Join("processed", streamUUID.String(), "seg_5.ts")).
			Return(nil, int64(0), storage.ErrNotFound)
		// Neighbours seg_4.ts and seg_6.ts exist and are merged.
		mockStor.EXPECT().
			Download(ctx, path.Join("processed", streamUUID.String(), "seg_4.ts")).
			Return(io.NopCloser(strings.NewReader("part-4")), int64(6), nil)
		mockStor.EXPECT().
			Download(ctx, path.Join("processed", streamUUID.String(), "seg_6.ts")).
			Return(io.NopCloser(strings.NewReader("part-6")), int64(6), nil)
		// Default: no other neighbours exist.
		for i := 2; i <= 8; i++ {
			if i == 4 || i == 5 || i == 6 {
				continue
			}
			mockStor.EXPECT().
				Download(ctx, path.Join("processed", streamUUID.String(), fmt.Sprintf("seg_%d.ts", i))).
				Return(nil, int64(0), storage.ErrNotFound)
		}
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.NoError(t, err)
		assert.Equal(t, "video/MP2T", svcRes.ContentType)
		assert.Equal(t, int64(12), svcRes.Size)
		body, rErr := io.ReadAll(svcRes.Content)
		require.NoError(t, rErr)
		assert.Equal(t, "part-4part-6", string(body))
		assert.NoError(t, svcRes.Content.Close())
	})
	t.Run("missing ts segment with no neighbours returns storage error", func(t *testing.T) {
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
			srvCfg(),
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
			FileName:   "seg_5.ts",
		}
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().
			Download(ctx, path.Join("processed", streamUUID.String(), "seg_5.ts")).
			Return(nil, int64(0), storage.ErrNotFound)
		for i := 2; i <= 8; i++ {
			if i == 5 {
				continue
			}
			mockStor.EXPECT().
				Download(ctx, path.Join("processed", streamUUID.String(), fmt.Sprintf("seg_%d.ts", i))).
				Return(nil, int64(0), storage.ErrNotFound)
		}
		svcRes, err := svc.GetFileByKey(ctx, svcReq)
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrNotFound)
		assert.Nil(t, svcRes)
	})
}

func TestStreamServiceImpl_ProcessFacesStream(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
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
			srvCfg(),
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
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
		}
		require.NoError(t, expectedStream.SetStorageInfo(storageInfo))
		facesID := "faces-task-1"
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().Exists(ctx, storageInfo.Key).Return(true, nil)
		mockQueue.EXPECT().
			ReprocessFacesProcessor(ctx, streamUUID, storageInfo.Key).
			Return(&facesID, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).
			Do(func(_ context.Context, s *models.Stream) error {
				assert.Contains(t, s.Processing.String(), models.TaskTypeFaces)
				return nil
			}).
			Return(nil)
		err := svc.ProcessFacesStream(ctx, streamUUID)
		require.NoError(t, err)
	})

	t.Run("stream not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
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
			srvCfg(),
		)
		ctx := context.Background()
		streamUUID := uuid.New()
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(nil, gorm.ErrRecordNotFound)
		err := svc.ProcessFacesStream(ctx, streamUUID)
		require.ErrorIs(t, err, service.ErrStreamNotFound)
	})

	t.Run("source file missing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
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
			srvCfg(),
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
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
		}
		require.NoError(t, expectedStream.SetStorageInfo(storageInfo))
		hlsKey := fmt.Sprintf("processed/%s/index.m3u8", streamUUID)
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().Exists(ctx, storageInfo.Key).Return(false, nil)
		mockStor.EXPECT().Exists(ctx, hlsKey).Return(false, nil)
		err := svc.ProcessFacesStream(ctx, streamUUID)
		require.ErrorIs(t, err, service.ErrSourceFileMissing)
	})

	t.Run("source missing falls back to hls playlist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
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
			srvCfg(),
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
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file",
			Filename: "video.mp4",
		}
		require.NoError(t, expectedStream.SetStorageInfo(storageInfo))
		hlsKey := fmt.Sprintf("processed/%s/index.m3u8", streamUUID)
		facesID := "faces-task-2"
		mockRepo.EXPECT().Read(ctx, streamUUID).Return(expectedStream, nil)
		mockStor.EXPECT().Exists(ctx, storageInfo.Key).Return(false, nil)
		mockStor.EXPECT().Exists(ctx, hlsKey).Return(true, nil)
		mockQueue.EXPECT().
			ReprocessFacesProcessor(ctx, streamUUID, hlsKey).
			Return(&facesID, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).
			Do(func(_ context.Context, s *models.Stream) error {
				assert.Contains(t, s.Processing.String(), models.TaskTypeFaces)
				return nil
			}).
			Return(nil)
		err := svc.ProcessFacesStream(ctx, streamUUID)
		require.NoError(t, err)
	})
}

func TestStreamServiceImpl_ProcessFacesBatch(t *testing.T) {
	newSvc := func(ctrl *gomock.Controller) (*service.StreamServiceImpl, *repomock.MockStreamRepository, *mock.MockFileStorage, *queuemock.MockTaskDistributor) {
		mockRepo := repomock.NewMockStreamRepository(ctrl)
		mockAuth := authmock.NewMockPermissionClient(ctrl)
		mockStor := mock.NewMockFileStorage(ctrl)
		mockQueue := queuemock.NewMockTaskDistributor(ctrl)
		svc := service.NewStreamServiceImpl(mockRepo, mockAuth, mockStor, mockQueue, nil, srvCfg())
		return svc, mockRepo, mockStor, mockQueue
	}

	streamWithStorage := func(id, owner uuid.UUID) (*models.Stream, *models.StreamStorage) {
		st := &models.Stream{
			BaseModel: models.BaseModel{ID: id},
			Title:     "Stream",
			OwnerID:   owner,
			Status:    models.StatusReady,
		}
		storageInfo := &models.StreamStorage{
			Provider: "minio",
			Bucket:   "bucket",
			Key:      "file-" + id.String(),
			Filename: "video.mp4",
		}
		require.NoError(t, st.SetStorageInfo(storageInfo))
		return st, storageInfo
	}

	t.Run("mixed batch: processed + forbidden + not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockStor, mockQueue := newSvc(ctrl)

		ctx := context.Background()
		userUUID := uuid.New()
		ownedID := uuid.New()
		foreignID := uuid.New()
		missingID := uuid.New()

		owned, ownedStorage := streamWithStorage(ownedID, userUUID)
		foreign, _ := streamWithStorage(foreignID, uuid.New())

		mockRepo.EXPECT().ReadMany(ctx, gomock.Any()).Return([]*models.Stream{owned, foreign}, nil)
		mockStor.EXPECT().Exists(ctx, ownedStorage.Key).Return(true, nil)
		facesID := "faces-batch-1"
		mockQueue.EXPECT().
			ReprocessFacesProcessor(ctx, ownedID, ownedStorage.Key).
			Return(&facesID, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).
			Do(func(_ context.Context, s *models.Stream) error {
				assert.Contains(t, s.Processing.String(), models.TaskTypeFaces)
				return nil
			}).
			Return(nil)

		res, err := svc.ProcessFacesBatch(ctx, userUUID, false, []uuid.UUID{ownedID, foreignID, missingID})
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{ownedID}, res.Processed)
		require.Len(t, res.Failed, 2)
		assert.Equal(t, foreignID, res.Failed[0].StreamID)
		assert.Equal(t, service.FacesBatchReasonForbidden, res.Failed[0].Reason)
		assert.Equal(t, missingID, res.Failed[1].StreamID)
		assert.Equal(t, service.FacesBatchReasonNotFound, res.Failed[1].Reason)
	})

	t.Run("admin bypasses ownership", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockStor, mockQueue := newSvc(ctrl)

		ctx := context.Background()
		adminID := uuid.New()
		foreignID := uuid.New()
		foreign, foreignStorage := streamWithStorage(foreignID, uuid.New())

		mockRepo.EXPECT().ReadMany(ctx, gomock.Any()).Return([]*models.Stream{foreign}, nil)
		mockStor.EXPECT().Exists(ctx, foreignStorage.Key).Return(true, nil)
		facesID := "faces-batch-admin"
		mockQueue.EXPECT().
			ReprocessFacesProcessor(ctx, foreignID, foreignStorage.Key).
			Return(&facesID, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		res, err := svc.ProcessFacesBatch(ctx, adminID, true, []uuid.UUID{foreignID})
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{foreignID}, res.Processed)
		assert.Empty(t, res.Failed)
	})

	t.Run("source missing reported per id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockStor, mockQueue := newSvc(ctrl)

		ctx := context.Background()
		userUUID := uuid.New()
		id1 := uuid.New()
		id2 := uuid.New()
		st1, storage1 := streamWithStorage(id1, userUUID)
		st2, storage2 := streamWithStorage(id2, userUUID)
		_ = st2
		hlsKey := fmt.Sprintf("processed/%s/index.m3u8", id2)

		mockRepo.EXPECT().ReadMany(ctx, gomock.Any()).Return([]*models.Stream{st1, st2}, nil)
		mockStor.EXPECT().Exists(ctx, storage1.Key).Return(true, nil)
		facesID := "faces-batch-ok"
		mockQueue.EXPECT().
			ReprocessFacesProcessor(ctx, id1, storage1.Key).
			Return(&facesID, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)
		mockStor.EXPECT().Exists(ctx, storage2.Key).Return(false, nil)
		mockStor.EXPECT().Exists(ctx, hlsKey).Return(false, nil)

		res, err := svc.ProcessFacesBatch(ctx, userUUID, false, []uuid.UUID{id1, id2})
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{id1}, res.Processed)
		require.Len(t, res.Failed, 1)
		assert.Equal(t, id2, res.Failed[0].StreamID)
		assert.Equal(t, service.FacesBatchReasonSource, res.Failed[0].Reason)
	})

	t.Run("empty ids returns empty result", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, _ := newSvc(ctrl)

		ctx := context.Background()
		mockRepo.EXPECT().ReadMany(ctx, gomock.Any()).Return([]*models.Stream{}, nil)

		res, err := svc.ProcessFacesBatch(ctx, uuid.New(), false, []uuid.UUID{})
		require.NoError(t, err)
		assert.Empty(t, res.Processed)
		assert.Empty(t, res.Failed)
	})
}
