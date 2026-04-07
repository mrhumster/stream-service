package response

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
)

type StreamResponse struct {
	ID          uuid.UUID               `json:"id"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Status      models.StreamStatus     `json:"status"`
	OwnerID     uuid.UUID               `json:"owner_id"`
	Visibility  models.StreamVisibility `json:"visibility"`
	Tags        []string                `json:"tags"`
	Metadata    models.StreamMetadata   `json:"metadata"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
	PublishedAt *time.Time              `json:"published_at"`
	Storage     models.StreamStorage    `json:"storage"`
	Processing  models.StreamProcessing `json:"processing"`
}

func FromDomainModel(stream *models.Stream) StreamResponse {
	resp := StreamResponse{
		ID:          stream.ID,
		Title:       stream.Title,
		Description: stream.Description,
		Status:      stream.Status,
		OwnerID:     stream.OwnerID,
		Visibility:  stream.Visibility,
		CreatedAt:   stream.CreatedAt,
		UpdatedAt:   stream.UpdatedAt,
		PublishedAt: stream.PublishedAt,
	}

	if len(stream.Tags) > 0 {
		var tags []string
		json.Unmarshal(stream.Tags, &tags)
		resp.Tags = tags
	}

	if len(stream.Metadata) > 0 {
		var metadata models.StreamMetadata
		json.Unmarshal(stream.Metadata, &metadata)
		resp.Metadata = metadata
	}

	if len(stream.Storage) > 0 {
		var storage models.StreamStorage
		json.Unmarshal(stream.Storage, &storage)
		resp.Storage = storage
	}

	if len(stream.Processing) > 0 {
		var processing models.StreamProcessing
		json.Unmarshal(stream.Processing, &processing)
		resp.Processing = processing
	}

	return resp
}
