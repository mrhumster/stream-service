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

// A stream that has not entered processing yet has SQL NULL in the JSONB
// columns. Processing and Tags have no omitempty, so a nil slice there is
// served as null, and the stream page reads both with .length.
func TestStreamResponse_FromDomainModel_NullJSONBSlicesBecomeArrays(t *testing.T) {
	t.Run("SQL NULL columns", func(t *testing.T) {
		resp := FromDomainModel(&models.Stream{Title: "fresh"})

		assert.NotNil(t, resp.Processing)
		assert.Empty(t, resp.Processing)
		assert.NotNil(t, resp.Tags)
		assert.Empty(t, resp.Tags)

		encoded, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.NotContains(t, string(encoded), `"processing":null`)
		assert.NotContains(t, string(encoded), `"tags":null`)
		assert.Contains(t, string(encoded), `"processing":[]`)
		assert.Contains(t, string(encoded), `"tags":[]`)
	})

	t.Run("JSON null columns", func(t *testing.T) {
		resp := FromDomainModel(&models.Stream{
			Title:      "null json",
			Processing: datatypes.JSON(`null`),
			Tags:       datatypes.JSON(`null`),
		})

		assert.NotNil(t, resp.Processing)
		assert.Empty(t, resp.Processing)
		assert.NotNil(t, resp.Tags)

		encoded, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.NotContains(t, string(encoded), `"processing":null`)
		assert.NotContains(t, string(encoded), `"tags":null`)
	})

	t.Run("empty arrays are preserved", func(t *testing.T) {
		resp := FromDomainModel(&models.Stream{
			Title:      "empty",
			Processing: datatypes.JSON(`[]`),
			Tags:       datatypes.JSON(`[]`),
		})

		assert.NotNil(t, resp.Processing)
		assert.Empty(t, resp.Processing)
		assert.NotNil(t, resp.Tags)
		assert.Empty(t, resp.Tags)
	})

	t.Run("tasks keep their steps array", func(t *testing.T) {
		resp := FromDomainModel(&models.Stream{
			Title:      "processing",
			Processing: datatypes.JSON(`[{"task_type":"transcode","progress":50,"steps":[],"error":null,"task_id":null}]`),
		})

		assert.Len(t, resp.Processing, 1)
		assert.NotNil(t, resp.Processing[0].Steps)
		assert.Empty(t, resp.Processing[0].Steps)

		encoded, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.NotContains(t, string(encoded), `"steps":null`)
	})
}
