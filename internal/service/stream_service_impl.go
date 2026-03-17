package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/identity-service/pkg/auth"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/queue"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type StreamServiceImpl struct {
	repo             repository.StreamRepository
	permissionClient auth.PermissionClient
	storage          storage.FileStorage
	queue            queue.TaskDistributor
}

func NewStreamServiceImpl(repo repository.StreamRepository, perm auth.PermissionClient, stor storage.FileStorage, queue queue.TaskDistributor) *StreamServiceImpl {
	return &StreamServiceImpl{
		repo:             repo,
		permissionClient: perm,
		storage:          stor,
		queue:            queue,
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

	if stream.Storage != nil {
		var stor models.StreamStorage

		err = json.Unmarshal(stream.Storage, &stor)
		if err != nil {
			return fmt.Errorf("unmarshaling storage error: %w", err)
		}
		if stor.Key != "" {
			err = s.storage.Delete(ctx, stor.Key)
			if err != nil {
				return fmt.Errorf("error delete stream file from storage: %w", err)
			}
		}
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
			log.Printf("Error removing permission. permissionClient.RemovePolicy(sub = %s,obj = %s, act = %s)", sub, obj, act)
		}

		if removed {
			log.Printf("Permission removed successfully. permissionClient.RemovePolicy(sub = %s,obj = %s, act = %s)", sub, obj, act)
		}

	}
	return nil
}

func (s *StreamServiceImpl) ListStreams(ctx context.Context, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
	streams, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, total, fmt.Errorf("failed to list streams: %w", err)
	}
	return streams, total, nil
}

func (s *StreamServiceImpl) ListUserStreams(ctx context.Context, userID uuid.UUID) ([]*models.Stream, int64, error) {
	filter := repository.StreamFilter{
		OwnerID: &userID,
		Limit:   100,
	}
	return s.ListStreams(ctx, filter)
}

func (s *StreamServiceImpl) PublishStream(ctx context.Context, streamID uuid.UUID) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		return fmt.Errorf("error read stream from repo: %w", err)
	}
	stream.Status = models.StatusPublished
	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	return nil
}

func (s *StreamServiceImpl) UnpublishStream(ctx context.Context, streamID uuid.UUID) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		return fmt.Errorf("error read stream from repo: %w", err)
	}
	stream.Status = models.StatusDraft
	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	return nil
}

func (s *StreamServiceImpl) UpdateStreamStatus(ctx context.Context, streamID uuid.UUID, status models.StreamStatus) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		return fmt.Errorf("error read stream from repo: %w", err)
	}
	stream.Status = status
	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	return nil
}

func (s *StreamServiceImpl) CanUserAccessStream(ctx context.Context, userID uuid.UUID, streamID uuid.UUID) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (s *StreamServiceImpl) UploadVideo(ctx context.Context, req UploadVideoRequest) error {
	stream, err := s.repo.Read(ctx, req.StreamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("stream not found")
		}
		return fmt.Errorf("failed to get stream: %w", err)
	}
	if stream.OwnerID != req.UserID {
		return fmt.Errorf("forbidden: user does not own this stream")
	}

	if stream.Status != models.StatusDraft {
		return fmt.Errorf("cannot upload video stream with status: %s", stream.Status)
	}

	storageKey := fmt.Sprintf("streams/%s/videos/%s_%s",
		req.UserID.String(),
		req.StreamID.String(),
		uuid.New().String())

	err = s.storage.Upload(ctx, storageKey, req.File, req.Size)
	if err != nil {
		return fmt.Errorf("failed to upload to storage: %w", err)
	}

	storageInfo := models.StreamStorage{
		Provider: "minio",
		Key:      storageKey,
		Filename: req.FileName,
		Bucket:   s.storage.GetBucketName(),
	}

	storageJSON, err := json.Marshal(storageInfo)
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return fmt.Errorf("failed to marshal storgae info: %w", err)
	}

	metadata := models.StreamMetadata{
		Size: req.Size,
		// TODO: Fill in remaining fields (duration, resolution)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return fmt.Errorf("failed to marshal metadata info: %w", err)
	}

	stream.Storage = datatypes.JSON(storageJSON)
	stream.Metadata = datatypes.JSON(metadataJSON)

	stream.Status = models.StatusProcessing
	stream.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, stream); err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return fmt.Errorf("failed to update stream: %w", err)
	}

	// more interesting
	// TODO: in this part, you can work with channels and gorutines
	//	go s.processVideoAsync(req.StreamID, storageKey, req.FileName, req.Size)
	return nil
}

func (s *StreamServiceImpl) processVideoAsync(streamID uuid.UUID, storageKey, filename string, size int64) {
	ctx := context.Background()

	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		log.Printf("Failed to get stream for processing: %v", err)
		return
	}

	// TODO: Реальная обработка видео с ffmpeg
	// Пока просто имитируем и обновляем дополнительные метаданные

	time.Sleep(2 * time.Second)

	var metadata models.StreamMetadata
	if err := json.Unmarshal(stream.Metadata, &metadata); err != nil {
		metadata = models.StreamMetadata{}
	}

	metadata.Duration = 3600
	metadata.Format = ""
	metadata.Resolution = "1920x1080" // Должно определяться из видео

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		log.Printf("Failed to marshal metadata: %v", err)
		return
	}

	stream.Metadata = datatypes.JSON(metadataJSON)
	stream.Status = models.StatusReady
	stream.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, stream); err != nil {
		log.Printf("Failed to update stream after processing: %v", err)
	}

	log.Printf("Video processing completed for stream %s", streamID)
}

func (s *StreamServiceImpl) GenerateDownloadURL(ctx context.Context, streamID uuid.UUID) (*GenerateDownloadURLInfo, error) {
	stream, err := s.GetStream(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("stream not found")
		}
		return nil, fmt.Errorf("error getting stream: %w", err)
	}

	if stream.Status != models.StatusReady {
		return nil, fmt.Errorf("stream not ready for download (status: %s)", stream.Status)
	}

	var storageInfo models.StreamStorage
	if err := json.Unmarshal(stream.Storage, &storageInfo); err != nil {
		return nil, fmt.Errorf("failed to parse storage info: %w", err)
	}

	if storageInfo.Key == "" {
		return nil, fmt.Errorf("storage key is empty")
	}

	var streamMeta models.StreamMetadata
	if err := json.Unmarshal(stream.Metadata, &streamMeta); err != nil {
		return nil, fmt.Errorf("failed to parse meta info: %w", err)
	}

	expires := 1 * time.Hour

	url, err := s.storage.GeneratePresignedURL(ctx, storageInfo.Key, storageInfo.Filename, expires)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Download URL: %w", err)
	}

	resp := &GenerateDownloadURLInfo{
		DownloadURL: url,
		ExpiresAt:   time.Now().Add(expires),
		FileName:    storageInfo.Filename,
		Size:        streamMeta.Size,
	}

	return resp, nil
}

// Multipart upload methods

func (s *StreamServiceImpl) StartStreamUpload(ctx context.Context, req StartUploadRequest) (*UploadInfo, error) {
	stream, err := s.repo.Read(ctx, req.StreamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("stream not found")
		}
		return nil, fmt.Errorf("error getting stream: %w", err)
	}

	if stream.OwnerID != req.UserID {
		return nil, fmt.Errorf("forbidden: not a owner")
	}

	if stream.Status != models.StatusDraft {
		return nil, fmt.Errorf("cannot start upload for stream in status: %s", stream.Status)
	}

	storageKey := fmt.Sprintf("streams/%s/videos/%s_%s",
		req.UserID.String(),
		req.StreamID.String(),
		uuid.New().String())

	uID, err := s.storage.InitMultipart(ctx, storageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to init storage: %w", err)
	}
	storageInfo := models.StreamStorage{
		Provider: "minio",
		Bucket:   s.storage.GetBucketName(),
		Key:      storageKey,
		UploadID: uID,
		Filename: req.Filename,
	}

	metadata := models.StreamMetadata{
		Size: req.TotalSize,
	}

	stream.SetMetadata(&metadata)
	stream.SetStorageInfo(&storageInfo)
	stream.Status = models.StatusUploading
	stream.UpdatedAt = time.Now()

	err = s.repo.Update(ctx, stream)
	if err != nil {
		_ = s.storage.AbortMultipart(ctx, storageKey, uID)
		return nil, fmt.Errorf("failed to update stream: %w", err)
	}
	return &UploadInfo{
		UploadID: storageInfo.UploadID,
		StreamID: stream.ID,
	}, nil
}

func (s *StreamServiceImpl) UploadPart(ctx context.Context, req UploadPartRequest) (*models.MultipartPart, error) {
	stream, err := s.repo.Read(ctx, req.StreamID)
	if err != nil {
		return nil, fmt.Errorf("error read stream from repository: %w", err)
	}

	if stream.Status != models.StatusUploading {
		return nil, fmt.Errorf("cannot start upload for stream in status: %s", stream.Status)
	}

	var storageInfo models.StreamStorage
	err = json.Unmarshal(stream.Storage, &storageInfo)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling storage info: %w", err)
	}

	if req.UploadID != storageInfo.UploadID {
		return nil, fmt.Errorf("upload id from request not equal upload id from storage info")
	}

	if stream.OwnerID != req.UserID {
		return nil, fmt.Errorf("forbidden: not a owner")
	}

	etag, err := s.storage.UploadPart(
		ctx,
		storageInfo.Key,
		req.UploadID,
		req.PartNumber,
		req.Data,
		req.Size,
	)
	if err != nil {
		return nil, fmt.Errorf("error upload part to strage: %w", err)
	}
	return &models.MultipartPart{
		PartNumber: req.PartNumber,
		ETag:       etag,
	}, nil
}

func (s *StreamServiceImpl) CompleteStreamUpload(ctx context.Context, req CompleteStreamUploadRequest) error {
	stream, err := s.repo.Read(ctx, req.StreamID)
	if err != nil {
		return fmt.Errorf("error read stream from repository: %w", err)
	}

	if stream.OwnerID != req.UserID {
		return fmt.Errorf("forbidden: not a owner")
	}

	if stream.Status != models.StatusUploading {
		return fmt.Errorf("cannot start upload for stream in status: %s", stream.Status)
	}

	var storageInfo models.StreamStorage
	err = json.Unmarshal(stream.Storage, &storageInfo)
	if err != nil {
		return fmt.Errorf("error unmarshaling storage info: %w", err)
	}

	sort.Slice(req.Parts, func(i, j int) bool {
		return req.Parts[i].PartNumber < req.Parts[j].PartNumber
	})

	if err = s.storage.CompleteMultipart(ctx, storageInfo.Key, storageInfo.UploadID, req.Parts); err != nil {
		return fmt.Errorf("storage error with Multipart: %w", err)
	}

	storageInfo.UploadID = ""
	stream.Status = models.StatusProcessing

	if err = stream.SetStorageInfo(&storageInfo); err != nil {
		return err
	}

	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error with update stream in repo: %w", err)
	}

	if err = s.queue.DistributeVideoTranscoding(ctx, stream.ID, storageInfo.Key); err != nil {
		slog.Error("failed to enqueue transcoding task for", "stream", stream.ID, "error", err)
	}

	return nil
}
