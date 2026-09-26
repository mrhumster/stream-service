package request

import (
	"fmt"

	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service"
)

type UpdateStreamRequest struct {
	Title       *string                  `json:"title"`
	Description *string                  `json:"description"`
	Visibility  *models.StreamVisibility `json:"visibility"`
	Tags        *[]string                `json:"tags"`
	Rotation    *int                     `json:"rotation"`
}

func (r *UpdateStreamRequest) Validate() error {
	if r.Title != nil {
		if *r.Title == "" {
			return fmt.Errorf("title cannot be empty")
		}
		if len(*r.Title) > 255 {
			return fmt.Errorf("tiitle is too long")
		}
	}

	if r.Visibility != nil {
		switch *r.Visibility {
		case models.VisibilityPrivate, models.VisibilityPublic, models.VisibilityUnlisted:
		default:
			return fmt.Errorf("invalid visibility: %s", *r.Visibility)
		}
	}

	if r.Rotation != nil {
		switch *r.Rotation {
		case 0, 90, 180, 270:
		default:
			return fmt.Errorf("invalid rotation: %d (must be one of 0, 90, 180, 270)", *r.Rotation)
		}
	}
	return nil
}

func (r *UpdateStreamRequest) ToServiceRequest() (*service.UpdateStreamRequest, error) {
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("validate failed: %w", err)
	}
	return &service.UpdateStreamRequest{
		Title:       r.Title,
		Description: r.Description,
		Visibility:  r.Visibility,
		Tags:        r.Tags,
		Rotation:    r.Rotation,
	}, nil
}
