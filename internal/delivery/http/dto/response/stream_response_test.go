package response

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/stretchr/testify/assert"
)

func TestStreamResponse_FromDomainModel(t *testing.T) {
	streamID := uuid.New()
	ownerID := uuid.New()
	now := time.Now()

	stream := &models.Stream{
		Title:       "Test stream",
		Description: "Test Description",
		Status:      models.StatusPublished,
		OwnerID:     ownerID,
		Visibility:  models.VisibilityPublic,
		PublishedAt: &now,
	}
	stream.ID = streamID
	stream.CreatedAt = now
	stream.UpdatedAt = now

	resp := FromDomainModel(stream)

	assert.Equal(t, streamID, resp.ID)
	assert.Equal(t, "Test stream", resp.Title)
	assert.Equal(t, ownerID, resp.OwnerID)
	assert.Equal(t, models.StatusPublished, resp.Status)
	assert.Equal(t, models.VisibilityPublic, resp.Visibility)
	assert.Equal(t, now, resp.CreatedAt)
}
