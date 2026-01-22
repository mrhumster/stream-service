package response

import (
	"encoding/json"
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

	storage := models.StreamStorage{
		Key:      "file",
		Bucket:   "bucket",
		Provider: "minio",
		Url:      "http://localhost:9000",
		Filename: "file.mp4",
	}

	storageJSON, err := json.Marshal(storage)

	if err != nil {
		t.Errorf("Error marshal storage to JSON error: %s", err.Error())
	}

	stream := &models.Stream{
		Title:       "Test stream",
		Description: "Test Description",
		Status:      models.StatusPublished,
		OwnerID:     ownerID,
		Visibility:  models.VisibilityPublic,
		PublishedAt: &now,
		Storage:     storageJSON,
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

	assert.Equal(t, storage.Key, resp.Storage["key"])
	assert.Equal(t, storage.Bucket, resp.Storage["bucket"])
	assert.Equal(t, storage.Provider, resp.Storage["provider"])
	assert.Equal(t, storage.Url, resp.Storage["url"])
	assert.Equal(t, storage.Filename, resp.Storage["filename"])
}
