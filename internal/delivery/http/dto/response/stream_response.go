package response

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
)

type StreamResponse struct {
	ID            uuid.UUID                     `json:"id"`
	Title         string                        `json:"title"`
	Description   string                        `json:"description"`
	Status        models.StreamStatus           `json:"status"`
	OwnerID       uuid.UUID                     `json:"owner_id"`
	Visibility    models.StreamVisibility       `json:"visibility"`
	Tags          []string                      `json:"tags"`
	Metadata      models.StreamMetadata         `json:"metadata"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
	PublishedAt   *time.Time                    `json:"published_at"`
	Storage       models.StreamStorage          `json:"storage"`
	Processing    []models.StreamProcessingTask `json:"processing"`
	FacesDetected bool                          `json:"faces_detected"`
}

func FromDomainModel(stream *models.Stream) StreamResponse {
	resp := StreamResponse{
		ID:            stream.ID,
		Title:         stream.Title,
		Description:   stream.Description,
		Status:        stream.Status,
		OwnerID:       stream.OwnerID,
		Visibility:    stream.Visibility,
		CreatedAt:     stream.CreatedAt,
		UpdatedAt:     stream.UpdatedAt,
		PublishedAt:   stream.PublishedAt,
		FacesDetected: stream.FacesDetected,
	}

	// Tags, Metadata, Storage and Processing are JSONB columns that may hold
	// SQL NULL. Unmarshalling into a bare var leaves the slice nil, and these
	// fields have no omitempty, so a stream that never entered processing would
	// be served as "processing": null and take any client reading .length down
	// with it. Seed the slice and only replace it with what was decoded.
	resp.Tags = []string{}
	if len(stream.Tags) > 0 {
		var tags []string
		if json.Unmarshal(stream.Tags, &tags) == nil && tags != nil {
			resp.Tags = tags
		}
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

	resp.Processing = []models.StreamProcessingTask{}
	if len(stream.Processing) > 0 {
		var processing []models.StreamProcessingTask
		if json.Unmarshal(stream.Processing, &processing) == nil && processing != nil {
			resp.Processing = processing
		}
	}

	return resp
}
