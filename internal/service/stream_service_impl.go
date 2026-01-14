package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/storage"
	"github.com/mrhumster/web-server-gin/pkg/auth"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type StreamServiceImpl struct {
	repo             repository.StreamRepository
	permissionClient auth.PermissionClient
	storage          storage.FileStorage
}

func NewStreamServiceImpl(repo repository.StreamRepository, perm auth.PermissionClient, stor storage.FileStorage) *StreamServiceImpl {
	return &StreamServiceImpl{
		repo:             repo,
		permissionClient: perm,
		storage:          stor,
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
	if len(req.Tags) > 0 {
		tagsJSON, err := json.Marshal(req.Tags)
		if err != nil {
			return nil, fmt.Errorf("invalid tags format: %w", err)
		}
		stream.Tags = datatypes.JSON(tagsJSON)
	}
	if err := s.repo.Create(ctx, stream); err != nil {
		return nil, err
	}
	sub := stream.OwnerID.String()
	obj := fmt.Sprintf("stream/%s", stream.ID.String())
	acts := []string{"write", "read", "delete"}
	for _, act := range acts {
		added, err := s.permissionClient.AddPolicy(ctx, sub, obj, act)
		if err != nil {
			log.Printf("Error creating permission. permissionClient.AddPolicy(sub = %s,obj = %s, act = %s)", sub, obj, act)
		}
		if added {
			log.Printf("Permission added successfully. permissionClient.AddPolicy(sub = %s,obj = %s, act = %s)", sub, obj, act)
		}
	}
	return stream, nil
}

func (s *StreamServiceImpl) GetStream(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	stream, err := s.repo.Read(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("stream not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}
	return stream, nil
}

func (s *StreamServiceImpl) UpdateStream(ctx context.Context, id uuid.UUID, req UpdateStreamRequest) (*models.Stream, error) {
	stream, err := s.repo.Read(ctx, id)
	if stream.Status == models.StatusPublished {
		return nil, fmt.Errorf("cannot update published stream")
	}
	if err != nil {
		return nil, fmt.Errorf("stream not found: %w", err)
	}
	if req.Title != nil {
		if *req.Title == "" {
			return nil, fmt.Errorf("stream title connot be empty")
		}
		if len(*req.Title) > 255 {
			return nil, fmt.Errorf("stream title is too long")
		}
		stream.Title = *req.Title
	}
	if req.Description != nil {
		stream.Description = *req.Description
	}
	if req.Visibility != nil {
		stream.Visibility = *req.Visibility
	}
	if req.Tags != nil {
		tagsJSON, err := json.Marshal(req.Tags)
		if err != nil {
			return nil, fmt.Errorf("invalid tags format: %w", err)
		}
		stream.Tags = datatypes.JSON(tagsJSON)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return nil, fmt.Errorf("failed to update stream: %w", err)
	}
	return stream, nil
}

func (s *StreamServiceImpl) DeleteStream(ctx context.Context, id uuid.UUID) error {
	stream, err := s.repo.Read(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("stream not found")
		}
		return fmt.Errorf("delete stream error: %w", err)
	}
	if stream.Status == models.StatusPublished {
		return fmt.Errorf("cannot delete published stream")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete stream: %w", err)
	}

	// PERMS
	obj := stream.OwnerID.String()
	sub := fmt.Sprintf("stream/%s", stream.ID.String())
	acts := []string{"read", "write", "delete"}
	for _, act := range acts {
		removed, err := s.permissionClient.RemovePolicy(ctx, obj, sub, act)
		if err != nil {
			log.Printf("Error creating permission. permissionClient.RemovePolicy(sub = %s,obj = %s, act = %s)", sub, obj, act)
		}

		if removed {
			log.Printf("Permission removed successfully. permissionClient.RemovePolicy(sub = %s,obj = %s, act = %s)", sub, obj, act)
		}

	}
	return nil
}

func (s *StreamServiceImpl) ListStreams(ctx context.Context, filter repository.StreamFilter) ([]*models.Stream, error) {
	streams, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list streams: %w", err)
	}
	return streams, nil
}

func (s *StreamServiceImpl) ListUserStreams(ctx context.Context, userID uuid.UUID) ([]*models.Stream, error) {
	filter := repository.StreamFilter{
		OwnerID: &userID,
		Limit:   100,
	}
	return s.ListStreams(ctx, filter)
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
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("stream not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	if stream.Status == models.StatusPublished {
		return nil, fmt.Errorf("cannot upload to published stream")
	}

	videoKey := fmt.Sprintf("videos/%s/%s/original", stream.OwnerID.String(), streamID.String())

	uploadURL, err := s.storage.GeneratePresignedURL(ctx, videoKey, 1*time.Hour)

	if err != nil {
		return nil, fmt.Errorf("failed to generate upload URL: %w", err)
	}

	storageInfo := models.StreamStorage{
		Provider: "minio",
		Key:      videoKey,
	}

	storageJSON, err := json.Marshal(storageInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal storage info: %w", err)
	}

	stream.Storage = datatypes.JSON(storageJSON)
	stream.Status = models.StatusProcessing

	if err := s.repo.Update(ctx, stream); err != nil {
		return nil, fmt.Errorf("failed to update stream: %w", err)
	}
	return &UploadInfo{
		UploadURL: uploadURL,
		StreamID:  streamID,
	}, nil
}

func (s *StreamServiceImpl) CompleteStreamUpload(ctx context.Context, streamID uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) CanUserAccessStream(ctx context.Context, userID uuid.UUID, streamID uuid.UUID) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
