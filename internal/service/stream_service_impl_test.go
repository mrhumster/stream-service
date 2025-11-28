package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStreamServicImpl_UpdateStream(t *testing.T) {
	ctx := context.Background()
	t.Run("succesful stream update", func(t *testing.T) {
		mockRepo := &repository.StreamRepositoryMock{}
		serviceImpl := service.NewStreamServiceImpl(mockRepo)

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

		mockRepo.On("Read", ctx, streamID).Return(existingStream, nil)
		mockRepo.On("Update", ctx, mock.MatchedBy(func(stream *models.Stream) bool {
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

		mockRepo.AssertExpectations(t)
	})
}

func TestStreamServicImpl_CreateStream(t *testing.T) {
	ctx := context.Background()

	t.Run("succesful stream creation", func(t *testing.T) {
		mockRepo := &repository.StreamRepositoryMock{}
		serviceImplementation := service.NewStreamServiceImpl(mockRepo)

		ownerID := uuid.New()

		req := service.CreateStreamRequest{
			Title:       "My Awesome Stream",
			Description: "This is a test stream description",
			Visibility:  models.VisibilityPrivate,
			Tags:        []string{"gaming", "live"},
			OwnerID:     ownerID,
		}
		mockRepo.On("Create", ctx, mock.MatchedBy(func(stream *models.Stream) bool {
			return stream.Title == req.Title &&
				stream.Description == req.Description &&
				stream.OwnerID == ownerID &&
				stream.Status == models.StatusDraft &&
				stream.Visibility == req.Visibility
		})).Return(nil)

		stream, err := serviceImplementation.CreateStream(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, stream)
		assert.Equal(t, req.Title, stream.Title)
		assert.Equal(t, req.Description, stream.Description)
		assert.Equal(t, ownerID, stream.OwnerID)
		assert.Equal(t, models.StatusDraft, stream.Status)
		assert.Equal(t, models.VisibilityPrivate, stream.Visibility)

		mockRepo.AssertExpectations(t)
	})

	t.Run("empty title should fail", func(t *testing.T) {
		mocRepo := &repository.StreamRepositoryMock{}
		srv := service.NewStreamServiceImpl(mocRepo)

		req := service.CreateStreamRequest{
			Title:   "",
			OwnerID: uuid.New(),
		}

		stream, err := srv.CreateStream(ctx, req)

		require.Error(t, err)
		assert.Nil(t, stream)
		mocRepo.AssertNotCalled(t, "Create")
	})

	t.Run("repository error should propagate", func(t *testing.T) {
		mockRepo := &repository.StreamRepositoryMock{}
		srv := service.NewStreamServiceImpl(mockRepo)

		req := service.CreateStreamRequest{
			Title:   "Test stream",
			OwnerID: uuid.New(),
		}

		mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Stream")).Return(assert.AnError)
		stream, err := srv.CreateStream(ctx, req)

		require.Error(t, err)
		assert.Nil(t, stream)
		assert.ErrorIs(t, err, assert.AnError)
		mockRepo.AssertExpectations(t)
	})
}
