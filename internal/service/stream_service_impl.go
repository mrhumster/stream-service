package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
)

type StreamServiceImpl struct {
	repo repository.StreamRepository
}

func NewStreamServiceImpl(repo repository.StreamRepository) *StreamServiceImpl {
	return &StreamServiceImpl{
		repo: repo,
	}
}

func (s *StreamServiceImpl) CreateStream(ctx context.Context, req CreateStreamRequest) (*models.Stream, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("stream title is required")
	}

	if req.OwnerID == uuid.Nil {
		return nil, fmt.Errorf("owner ID is required")
	}

	stream := &models.Stream{
		Title:       req.Title,
		Description: req.Description,
		OwnerID:     req.OwnerID,
		Status:      models.StatusDraft,
		Visibility:  req.Visibility,
	}

	if err := s.repo.Create(ctx, stream); err != nil {
		return nil, err
	}

	return stream, nil
}
func (s *StreamServiceImpl) GetStream(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) UpdateStream(ctx context.Context, id uuid.UUID, req UpdateStreamRequest) (*models.Stream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) DeleteStream(ctx context.Context, id uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) ListStreams(ctx context.Context, filter repository.StreamFilter) ([]*models.Stream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) ListUserStreams(ctx context.Context, userID uuid.UUID) ([]*models.Stream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) PublishStream(ctx context.Context, streamID uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) UnpublishStream(ctx context.Context, streamID uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) UpdateStreamStatus(ctx context.Context, streamID uuid.UUID, status models.StreamStatus) error {
	return fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) StartStreamUpload(ctx context.Context, streamID uuid.UUID) (*UploadInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) CompleteStreamUpload(ctx context.Context, streamID uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) CanUserAccessStream(ctx context.Context, userID uuid.UUID, streamID uuid.UUID) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
