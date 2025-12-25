//go:generate mockgen -source=stream_repository.go -destination=./mock/stream_repository_mock.go -package=repomock

package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
)

type StreamRepository interface {
	Create(ctx context.Context, stream *models.Stream) error
	Read(ctx context.Context, id uuid.UUID) (*models.Stream, error)
	Update(ctx context.Context, stream *models.Stream) error
	Delete(ctx context.Context, id uuid.UUID) error

	List(ctx context.Context, filter StreamFilter) ([]*models.Stream, error)
	GetByOwner(ctx context.Context, ownerID uuid.UUID) ([]*models.Stream, error)
	Exists(ctx context.Context, id uuid.UUID) bool

	UpdateStatus(ctx context.Context, id uuid.UUID, status models.StreamStatus) error
	UpdateProcessing(ctx context.Context, id uuid.UUID, processing models.StreamProcessing) error

	IncrementViews(ctx context.Context, id uuid.UUID) error
}

type StreamFilter struct {
	OwnerID    *uuid.UUID
	Status     *models.StreamStatus
	Visibility *models.StreamVisibility
	Limit      int
	Offset     int
	Search     string
}
