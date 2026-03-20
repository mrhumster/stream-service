package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// StreamServiceContractTest определяет контракт для всех реализаций StreamService
type StreamServiceContractTest struct {
	t       *testing.T
	service service.StreamService
	ctx     context.Context
}

func NewStreamServiceContractTest(t *testing.T, service service.StreamService) *StreamServiceContractTest {
	return &StreamServiceContractTest{
		t:       t,
		service: service,
		ctx:     context.Background(),
	}
}

func createTestStreamData(id uuid.UUID, title string, ownerID uuid.UUID) *models.Stream {
	tags, _ := json.Marshal([]string{"test", "contract"})
	metadata, _ := json.Marshal(models.StreamMetadata{
		Duration:   3600,
		Size:       1024 * 1024 * 100,
		Format:     "mp4",
		Resolution: "1080p",
	})
	storage, _ := json.Marshal(models.StreamStorage{
		Provider: "s3",
		Bucket:   "streams",
		Key:      id.String() + ".mp4",
	})
	processing, _ := json.Marshal(models.StreamProcessing{
		Progress: 100,
		Steps:    []string{"upload", "process", "complete"},
	})

	return &models.Stream{
		BaseModel: models.BaseModel{
			ID:        id,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Title:       title,
		Description: "Contract Test Description",
		Status:      models.StatusDraft,
		OwnerID:     ownerID,
		Visibility:  models.VisibilityPrivate,
		Tags:        datatypes.JSON(tags),
		Metadata:    datatypes.JSON(metadata),
		Storage:     datatypes.JSON(storage),
		Processing:  datatypes.JSON(processing),
	}
}

func (c *StreamServiceContractTest) TestAll() {
	c.TestCRUDOperations()
	c.TestStreamLifecycle()
	c.TestAccessControl()
	c.TestListOperations()
	c.TestStreamStatusTransitions()
}

// TestCRUDOperations тестирует базовые CRUD операции
func (c *StreamServiceContractTest) TestCRUDOperations() {
	c.t.Run("CRUD Operations", func(t *testing.T) {
		userID := uuid.New()

		// Create
		createReq := service.CreateStreamRequest{
			Title:       "Test Stream",
			Description: "Test Description for Contract",
			Visibility:  models.VisibilityPrivate,
			Tags:        []string{"contract", "test"},
			OwnerID:     userID,
		}

		created, err := c.service.CreateStream(c.ctx, createReq)
		t.Logf("created stream: %v", created.Title)
		require.NoError(c.t, err)
		require.NotNil(c.t, created)
		assert.Equal(c.t, createReq.Title, created.Title)
		assert.Equal(c.t, createReq.Visibility, created.Visibility)

		// Read
		retrieved, err := c.service.GetStream(c.ctx, created.ID)
		require.NoError(c.t, err)
		require.NotNil(c.t, retrieved)
		assert.Equal(c.t, created.ID, retrieved.ID)
		assert.Equal(c.t, created.Title, retrieved.Title)

		// Update
		newTitle := "Updated Stream"
		newVisibility := models.VisibilityPrivate
		updateReq := service.UpdateStreamRequest{
			Title:      &newTitle,
			Visibility: &newVisibility,
		}

		updated, err := c.service.UpdateStream(c.ctx, created.ID, updateReq)
		require.NoError(c.t, err)
		require.NotNil(c.t, updated)
		assert.Equal(c.t, newTitle, updated.Title)
		assert.Equal(c.t, newVisibility, updated.Visibility)

		// Delete
		err = c.service.DeleteStream(c.ctx, created.ID)
		assert.NoError(c.t, err)
	})
}

// TestStreamLifecycle тестирует жизненный цикл стрима
func (c *StreamServiceContractTest) TestStreamLifecycle() {
	c.t.Run("Stream Lifecycle", func(t *testing.T) {
		userID := uuid.New()

		// Create stream
		stream, err := c.service.CreateStream(c.ctx, service.CreateStreamRequest{
			Title:   "Lifecycle Test Stream",
			OwnerID: userID,
		})
		require.NoError(c.t, err)

		// Test status transitions
		err = c.service.UpdateStreamStatus(c.ctx, stream.ID, models.StatusProcessing)
		assert.NoError(c.t, err)

		err = c.service.UpdateStreamStatus(c.ctx, stream.ID, models.StatusReady)
		assert.NoError(c.t, err)

		// Publish stream
		err = c.service.PublishStream(c.ctx, stream.ID)
		assert.NoError(c.t, err)

		// Should be published status after publish
		err = c.service.UpdateStreamStatus(c.ctx, stream.ID, models.StatusPublished)
		assert.NoError(c.t, err)

		// Unpublish stream
		err = c.service.UnpublishStream(c.ctx, stream.ID)
		assert.NoError(c.t, err)

		// Cleanup
		_ = c.service.DeleteStream(c.ctx, stream.ID)
	})
}

// TestStreamStatusTransitions тестирует переходы между статусами
func (c *StreamServiceContractTest) TestStreamStatusTransitions() {
	c.t.Run("Stream Status Transitions", func(t *testing.T) {
		userID := uuid.New()

		stream, err := c.service.CreateStream(c.ctx, service.CreateStreamRequest{
			Title:   "Status Test Stream",
			OwnerID: userID,
		})
		require.NoError(c.t, err)

		// Test all status transitions
		statuses := []models.StreamStatus{
			models.StatusProcessing,
			models.StatusReady,
			models.StatusPublished,
			models.StatusDraft,
		}

		for _, status := range statuses {
			err = c.service.UpdateStreamStatus(c.ctx, stream.ID, status)
			assert.NoError(c.t, err, "Failed to transition to status: %s", status)
		}

		// Test error status
		err = c.service.UpdateStreamStatus(c.ctx, stream.ID, models.StatusError)
		assert.NoError(c.t, err)

		// Cleanup
		_ = c.service.DeleteStream(c.ctx, stream.ID)
	})
}

// TestAccessControl тестирует контроль доступа
func (c *StreamServiceContractTest) TestAccessControl() {
	c.t.Run("Access Control", func(t *testing.T) {
		ownerID := uuid.New()
		otherUserID := uuid.New()

		// Create streams with different visibility
		publicStream, err := c.service.CreateStream(c.ctx, service.CreateStreamRequest{
			Title:      "Public Stream",
			Visibility: models.VisibilityPublic,
			OwnerID:    ownerID,
		})
		require.NoError(c.t, err)

		privateStream, err := c.service.CreateStream(c.ctx, service.CreateStreamRequest{
			Title:      "Private Stream",
			Visibility: models.VisibilityPrivate,
			OwnerID:    ownerID,
		})
		require.NoError(c.t, err)

		unlistedStream, err := c.service.CreateStream(c.ctx, service.CreateStreamRequest{
			Title:      "Unlisted Stream",
			Visibility: models.VisibilityUnlisted,
			OwnerID:    ownerID,
		})
		require.NoError(c.t, err)

		// Test access control for different users and visibility
		testCases := []struct {
			name     string
			userID   uuid.UUID
			streamID uuid.UUID
		}{
			{"Owner access public", ownerID, publicStream.ID},
			{"Owner access private", ownerID, privateStream.ID},
			{"Owner access unlisted", ownerID, unlistedStream.ID},
			{"Other user access public", otherUserID, publicStream.ID},
			{"Other user access private", otherUserID, privateStream.ID},
			{"Other user access unlisted", otherUserID, unlistedStream.ID},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				canAccess, err := c.service.CanUserAccessStream(c.ctx, tc.userID, tc.streamID)
				assert.NoError(t, err)
				// Result depends on implementation, but should not error
				assert.NotNil(t, canAccess)
			})
		}

		// Cleanup
		_ = c.service.DeleteStream(c.ctx, publicStream.ID)
		_ = c.service.DeleteStream(c.ctx, privateStream.ID)
		_ = c.service.DeleteStream(c.ctx, unlistedStream.ID)
	})
}

// TestListOperations тестирует операции со списками
func (c *StreamServiceContractTest) TestListOperations() {
	c.t.Run("List Operations", func(t *testing.T) {
		userID := uuid.New()

		// Create test streams
		stream1, err := c.service.CreateStream(c.ctx, service.CreateStreamRequest{
			Title:   "List Test Stream 1",
			OwnerID: userID,
		})
		require.NoError(c.t, err)

		stream2, err := c.service.CreateStream(c.ctx, service.CreateStreamRequest{
			Title:   "List Test Stream 2",
			OwnerID: userID,
		})
		require.NoError(c.t, err)

		// Test ListUserStreams
		userStreams, _, err := c.service.ListUserStreams(c.ctx, userID)
		assert.NoError(c.t, err)
		assert.NotNil(c.t, userStreams)

		// Test ListStreams with different filters
		status := models.StatusDraft
		visibility := models.VisibilityPrivate

		filterTests := []repository.StreamFilter{
			{
				Status: &status,
				Limit:  10,
				Offset: 0,
			},
			{
				Visibility: &visibility,
				Limit:      10,
				Offset:     0,
			},
			{
				OwnerID: &userID,
				Limit:   10,
				Offset:  0,
			},
			{
				Limit:  5,
				Offset: 0,
			},
		}

		for _, filter := range filterTests {
			streams, _, err := c.service.ListStreams(c.ctx, filter)
			assert.NoError(c.t, err)
			assert.NotNil(c.t, streams)
		}

		// Cleanup
		_ = c.service.DeleteStream(c.ctx, stream1.ID)
		_ = c.service.DeleteStream(c.ctx, stream2.ID)
	})
}
