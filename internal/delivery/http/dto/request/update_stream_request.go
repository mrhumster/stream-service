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
	return nil
}

func (r *UpdateStreamRequest) ToServiceRequest() (service.UpdateStreamRequest, error) {
	if err := r.Validate(); err != nil {
		return service.UpdateStreamRequest{}, fmt.Errorf("validate failed: %w", err)
	}
	return service.UpdateStreamRequest{
		Title:       r.Title,
		Description: r.Description,
		Visibility:  r.Visibility,
		Tags:        r.Tags,
	}, nil
}
