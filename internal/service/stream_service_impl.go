package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/identity-service/pkg/auth"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/queue"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/storage"
	"github.com/mrhumster/stream-service/internal/wss"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type StreamServiceImpl struct {
	repo             repository.StreamRepository
	permissionClient auth.PermissionClient
	storage          storage.FileStorage
	queue            queue.TaskDistributor
	hub              wss.Hub
	cfg              *config.Server
}

func NewStreamServiceImpl(repo repository.StreamRepository, perm auth.PermissionClient, stor storage.FileStorage, queue queue.TaskDistributor, hub wss.Hub, cfg *config.Server) *StreamServiceImpl {
	return &StreamServiceImpl{
		repo:             repo,
		permissionClient: perm,
		storage:          stor,
		queue:            queue,
		hub:              hub,
		cfg:              cfg,
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

	s.notifyUpdate(stream)
	return stream, nil
}

func (s *StreamServiceImpl) DeleteStream(ctx context.Context, id uuid.UUID) error {
	slog.Info("start deleting", "stream id", id)
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
		slog.Info("stream info", "status", stream.Status)
		if stream.Status == models.StatusReady {
			dirPath := fmt.Sprintf("processed/%s", id)
			slog.Info("stream processed", "path", dirPath)
			err = s.storage.DeleteFolder(ctx, dirPath)
			if err != nil {
				slog.Error("failed to delete object", "error", err)
			}
		}
	}

	if stream.Processing != nil {
		var processing models.StreamProcessing
		err = json.Unmarshal(stream.Processing, &processing)
		if err != nil {
			return fmt.Errorf("umarshaling processing error: %w", err)
		}
		if processing.TaskID != nil && stream.Status == models.StatusProcessing {
			if err = s.queue.TerminateTask(ctx, *processing.TaskID); err != nil {
				slog.Error("terminate task",
					"stream uuid", stream.ID,
					"task id", processing.TaskID,
					"error", err.Error())
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
	if stream.Status != models.StatusReady {
		return fmt.Errorf("can't publish stream if they not ready")
	}
	stream.Status = models.StatusPublished
	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyComplete(stream)
	return nil
}

func (s *StreamServiceImpl) UnpublishStream(ctx context.Context, streamID uuid.UUID) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		return fmt.Errorf("error read stream from repo: %w", err)
	}
	stream.Status = models.StatusReady
	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyUpdate(stream)
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
	if stream.Status == models.StatusReady && !s.cfg.KeepOriginalFile {
		streamStorage, _ := stream.GetStorageInfo()
		if err := s.storage.Delete(ctx, streamStorage.Key); err != nil {
			slog.Error("Delete source", "error", err, "stream", stream.ID, "storage key", streamStorage.Key)
		}
	}
	s.notifyUpdate(stream)
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

	taskID, err := s.queue.DistributeVideoTranscoding(
		ctx,
		stream.ID,
		storageInfo.Key,
	)
	if err != nil {
		slog.Error("failed to enqueue transcoding task for", "stream", stream.ID, "error", err)
	}

	slog.Info("send transcoder task", "TaskID", *taskID)

	if err := s.UpdateStreamProcessing(ctx, &UpdateStreamProcessingRequest{
		StreamUUID: req.StreamID,
		Processing: models.StreamProcessing{
			Progress: 0,
			Steps:    []string{"convertation"},
			Error:    nil,
			TaskID:   taskID,
		},
	}); err != nil {
		return fmt.Errorf("failed to update processing: %w", err)
	}

	return nil
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
	// Glues together the parts of the file and sends them to the queue for processing
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

	if err = s.storage.CompleteMultipart(
		ctx,
		storageInfo.Key,
		storageInfo.UploadID,
		req.Parts,
	); err != nil {
		return fmt.Errorf("storage error with Multipart: %w", err)
	}

	storageInfo.UploadID = ""
	stream.Status = models.StatusProcessing

	if err = stream.SetStorageInfo(&storageInfo); err != nil {
		return fmt.Errorf("error with set storage info: %w", err)
	}

	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error with update stream in repo: %w", err)
	}

	taskID, err := s.queue.DistributeVideoTranscoding(
		ctx,
		stream.ID,
		storageInfo.Key,
	)
	if err != nil {
		slog.Error("failed to enqueue transcoding task for", "stream", stream.ID, "error", err)
	}

	if err := s.UpdateStreamProcessing(ctx, &UpdateStreamProcessingRequest{
		StreamUUID: req.StreamID,
		Processing: models.StreamProcessing{
			Progress: 0,
			Steps:    []string{"convertation"},
			Error:    nil,
			TaskID:   taskID,
		},
	}); err != nil {
		return fmt.Errorf("failed to update processing: %w", err)
	}
	s.notifyUpdate(stream)

	// Thumbsnail processing
	taksID, err := s.queue.DistributeThumbsnailProcessor(
		ctx,
		stream.ID,
		storageInfo.Key,
	)
	if err != nil {
		slog.Error("failed to enqueue thumbsnail task for", "stream", stream.ID, "error", err)
	}

	slog.Debug("Task for generate thumbsnail in queue", "taskID", taksID)

	return nil
}

func (s *StreamServiceImpl) UpdateStreamMetadata(ctx context.Context, req *UpdateStreamMetadataRequest) error {
	stream, err := s.repo.Read(ctx, req.StreamUUID)
	if err != nil {
		return fmt.Errorf("error read stream from repository: %w", err)
	}
	if err := stream.SetMetadata(&req.Metadata); err != nil {
		return fmt.Errorf("error setting metadata to model: %w", err)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyUpdate(stream)
	return nil
}

func (s *StreamServiceImpl) UpdateStreamProcessing(ctx context.Context, req *UpdateStreamProcessingRequest) error {
	stream, err := s.repo.Read(ctx, req.StreamUUID)
	if err != nil {
		return fmt.Errorf("error read stream from repository: %w", err)
	}
	if req.Processing.TaskID == nil {
		var currentProcessing models.StreamProcessing
		json.Unmarshal(stream.Processing, &currentProcessing)
		req.Processing.TaskID = currentProcessing.TaskID
	}

	if err := stream.UpdateProcessing(req.Processing.Progress, req.Processing.Steps, req.Processing.Error, req.Processing.TaskID); err != nil {
		return fmt.Errorf("error update processing: %w", err)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyUpdate(stream)
	return nil
}

func getContentType(fileName string) string {
	switch {
	case strings.HasSuffix(fileName, ".m3u8"):
		return "application/x-mpegURL"
	case strings.HasSuffix(fileName, ".ts"):
		return "video/MP2T"
	default:
		return "application/octet-stream"
	}
}

func (s *StreamServiceImpl) notifyUpdate(stream *models.Stream) {
	if s.hub != nil {
		s.hub.SendMessgeToOwner(stream.OwnerID, gin.H{
			"type": "STREAM_UPDATED",
			"payload": gin.H{
				"stream_id": stream.ID,
			},
		})
	}
}

func (s *StreamServiceImpl) notifyComplete(stream *models.Stream) {
	if s.hub != nil {
		s.hub.SendMessgeToOwner(stream.OwnerID, gin.H{
			"type": "STREAM_READY",
			"payload": gin.H{
				"stream_id": stream.ID,
			},
		})
	}
}

func (s *StreamServiceImpl) GetFileByKey(ctx context.Context, req *GetFileByKeyRequest) (*GetFileByKeyResponse, error) {
	stream, err := s.GetStream(ctx, req.StreamUUID)
	if err != nil {
		return nil, fmt.Errorf("error get stream from repository: %w", err)
	}
	if stream.Status != models.StatusReady && stream.Status != models.StatusPublished {
		return nil, fmt.Errorf("you can't watch a stream with the status %s", stream.Status)
	}
	path := path.Join("processed", req.StreamUUID.String(), req.FileName)
	content, size, err := s.storage.Download(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("error download file from storage: %w", err)
	}
	return &GetFileByKeyResponse{
		Content:     content,
		ContentType: getContentType(req.FileName),
		Size:        size,
	}, nil
}
