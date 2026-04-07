package response

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

func TestStreamResponse_FromDomainModel(t *testing.T) {
	streamID := uuid.New()
	ownerID := uuid.New()
	now := time.Now()

	storage := models.StreamStorage{
		Key:      "file",
		Bucket:   "bucket",
		Provider: "minio",
		Filename: "file.mp4",
	}

	storageJSON, err := json.Marshal(storage)
	if err != nil {
		t.Errorf("Error marshal storage to JSON error: %s", err.Error())
	}

	streamMetaData := models.StreamMetadata{
		Duration:   321,
		Size:       123,
		Format:     "video",
		Resolution: "2x2",
	}

	streamMetaDataJSON, err := json.Marshal(streamMetaData)
	assert.NoError(t, err)

	stream := &models.Stream{
		Title:       "Test stream",
		Description: "Test Description",
		Status:      models.StatusPublished,
		OwnerID:     ownerID,
		Visibility:  models.VisibilityPublic,
		PublishedAt: &now,
		Storage:     storageJSON,
		Tags:        datatypes.JSON(`["tag1", "tag2"]`),
		Metadata:    datatypes.JSON(streamMetaDataJSON),
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

	assert.Equal(t, storage.Key, resp.Storage.Key)
	assert.Equal(t, storage.Bucket, resp.Storage.Bucket)
	assert.Equal(t, storage.Provider, resp.Storage.Provider)
	assert.Equal(t, storage.Filename, resp.Storage.Filename)
	assert.Contains(t, resp.Tags, "tag1")
	assert.Equal(t, resp.Metadata.Resolution, "2x2")
}
