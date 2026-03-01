package request

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service"
)

type CreateStreamRequest struct {
	Title       string                  `json:"title" binding:"required,min=1,max=255"`
	Description string                  `json:"description"`
	Visibility  models.StreamVisibility `json:"visibility" binding:"required,oneof=public private unlisted"`
	Tags        []string                `json:"tags"`
}

func (r *CreateStreamRequest) ToServiceRequest(ownerID uuid.UUID) (service.CreateStreamRequest, error) {
	return service.CreateStreamRequest{
		Title:       r.Title,
		Description: r.Description,
		Visibility:  r.Visibility,
		Tags:        r.Tags,
		OwnerID:     ownerID,
	}, nil
}

func (r *CreateStreamRequest) Validate() error {
	if r.Title == "" {
		return fmt.Errorf("title is required")
	}

	if len(r.Title) > 255 {
		return fmt.Errorf("title is too long")
	}

	switch r.Visibility {
	case models.VisibilityPrivate, models.VisibilityPublic, models.VisibilityUnlisted:
	default:
		return fmt.Errorf("invalid visibility: %s", r.Visibility)
	}

	return nil
}
