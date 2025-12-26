package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	repomock "github.com/mrhumster/stream-service/internal/repository/mock"
	"github.com/mrhumster/stream-service/internal/service"
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

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)

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
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)

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
func TestStreamServicImpl_DeleteStream(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repomock.NewMockStreamRepository(ctrl)
	mockPermissionClient := authmock.NewMockPermissionClient(ctrl)
	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)

	t.Run("successful delete", func(t *testing.T) {
		existingStream := &models.Stream{
			Title:  "Stream for delete",
			Status: models.StatusDraft,
		}
		streamID := uuid.New()
		existingStream.ID = streamID
		mockRepo.EXPECT().Read(gomock.Any(), streamID).Return(existingStream, nil)
		mockRepo.EXPECT().Delete(gomock.Any(), streamID).Return(nil)

		err := serviceImpl.DeleteStream(ctx, streamID)
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

}
func TestStreamServicImpl_GetStream(t *testing.T) {
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repomock.NewMockStreamRepository(ctrl)
	mockPermissionClient := authmock.NewMockPermissionClient(ctrl)

	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)

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

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)
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

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)
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

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)
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
		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)
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

		serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)
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

	serviceImpl := service.NewStreamServiceImpl(mockRepo, mockPermissionClient)

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
