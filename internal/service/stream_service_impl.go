package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/identity-service/pkg/auth"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/domain/models"
	streammetrics "github.com/mrhumster/stream-service/internal/metrics"
	"github.com/mrhumster/stream-service/internal/queue"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/storage"
	"github.com/mrhumster/stream-service/internal/wss"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var segmentNamePattern = regexp.MustCompile(`^seg_(\d+)\.ts$`)

type StreamServiceImpl struct {
	repo             repository.StreamRepository
	permissionClient auth.PermissionClient
	storage          storage.FileStorage
	queue            queue.TaskDistributor
	hub              wss.Hub
	cfg              *config.Server
	eventsRecorder   queue.ActivityEventRecorder
	cascadeRecorder  queue.FacesCascadeRecorder
	exportRepo       repository.StreamExportRepository
	exportQueue      queue.ExportTaskDistributor
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

// WithActivityRecorder attaches the activity-event recorder (best-effort,
// nil-safe). Events for the stream owner are emitted through the events queue.
func (s *StreamServiceImpl) WithActivityRecorder(r queue.ActivityEventRecorder) *StreamServiceImpl {
	s.eventsRecorder = r
	return s
}

// WithFacesCascadeRecorder attaches the faces cascade recorder (best-effort,
// nil-safe): when a stream is deleted, faces-worker is asked to drop all stored
// face data for it.
func (s *StreamServiceImpl) WithFacesCascadeRecorder(r queue.FacesCascadeRecorder) *StreamServiceImpl {
	s.cascadeRecorder = r
	return s
}

// WithExportQueue attaches the distributor used to ask the export worker for a
// single-file rendition. It is separate from TaskDistributor because the export
// queue lives in its own Redis DB, so the export service can be scaled and
// restarted without touching transcoding.
func (s *StreamServiceImpl) WithExportQueue(q queue.ExportTaskDistributor) *StreamServiceImpl {
	s.exportQueue = q
	return s
}

// WithExportRepository attaches the store for export state.
func (s *StreamServiceImpl) WithExportRepository(r repository.StreamExportRepository) *StreamServiceImpl {
	s.exportRepo = r
	return s
}

// recordFaceCascade best-effort enqueues a faces cascade task after a stream
// has been deleted from the database.
func (s *StreamServiceImpl) recordFaceCascade(ctx context.Context, streamID uuid.UUID) {
	if s.cascadeRecorder == nil {
		return
	}
	if err := s.cascadeRecorder.RecordStreamDeleted(ctx, streamID); err != nil {
		slog.Warn("record faces cascade failed", "stream", streamID, "error", err)
	}
}

// eventPayload builds the activity-event payload for a stream event. The
// stream title (and visibility) are always included so the feed can render
// the title; optional fields (filename, progress, error, ...) are merged.
// Stream fields take precedence over any extra values with the same key.
func eventPayload(stream *models.Stream, extra any) map[string]any {
	p := map[string]any{}
	switch m := extra.(type) {
	case nil:
	case map[string]any:
		for k, v := range m {
			p[k] = v
		}
	default:
		slog.Warn("unexpected activity event payload type", "type", fmt.Sprintf("%T", extra))
	}
	p["title"] = stream.Title
	p["visibility"] = stream.Visibility
	return p
}

func (s *StreamServiceImpl) recordEvent(ctx context.Context, stream *models.Stream, eventType string, payload any) {
	if s.eventsRecorder == nil {
		return
	}
	if err := s.eventsRecorder.RecordActivityEvent(ctx, stream.OwnerID, eventType, &stream.ID, eventPayload(stream, payload)); err != nil {
		slog.Warn("record activity event failed", "stream", stream.ID, "event", eventType, "error", err)
	}
}

var (
	ErrStreamNotFound        = errors.New("stream not found")
	ErrCannotUpdatePublished = errors.New("cannot update published stream")
	ErrCannotDeletePublished = errors.New("cannot delete published stream")
	ErrStreamNotReady        = errors.New("stream not ready for download")
	ErrInvalidFileName       = errors.New("invalid file name")
	ErrCannotWatch           = errors.New("stream is not available for watching")
	ErrStreamNotInErrorState = errors.New("stream is not in an error state")
	ErrStreamNotProcessing   = errors.New("stream is not processing")
	ErrSourceFileMissing     = errors.New("source file is missing")
	ErrCannotPublish         = errors.New("stream must be ready before publishing")
	ErrCannotUnpublish       = errors.New("only published streams can be unpublished")
	ErrStreamForbidden       = errors.New("only the stream owner can do this")
	ErrExportNotFound        = errors.New("no export has been prepared for this stream")
	ErrExportPending         = errors.New("export is still being prepared")
)

const (
	// defaultDownloadName is used when a title sanitizes down to nothing.
	defaultDownloadName = "stream.mp4"
	// maxDownloadNameLen keeps the attachment name inside what common
	// filesystems accept once a multi-byte title is counted in bytes.
	maxDownloadNameLen = 120
)

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
	s.recordEvent(ctx, stream, "stream.created", map[string]any{"title": stream.Title, "visibility": stream.Visibility})
	return stream, nil
}

func (s *StreamServiceImpl) GetStream(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	stream, err := s.repo.Read(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStreamNotFound
		}
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}
	return stream, nil
}

func (s *StreamServiceImpl) GetStreamStatus(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	stream, err := s.repo.Read(ctx, id)
	if err != nil {
		return nil, ErrStreamNotFound
	}
	return stream, nil
}

func (s *StreamServiceImpl) UpdateStream(ctx context.Context, id uuid.UUID, req UpdateStreamRequest) (*models.Stream, error) {
	stream, err := s.repo.Read(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStreamNotFound
		}
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}
	if stream.Status == models.StatusPublished {
		return nil, ErrCannotUpdatePublished
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
	if req.Rotation != nil {
		if err := setStreamRotation(stream, *req.Rotation); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return nil, fmt.Errorf("failed to update stream: %w", err)
	}

	s.notifyUpdate(stream)
	return stream, nil
}

// setStreamRotation writes "rotation" into the stream's metadata JSONB while
// preserving every other key already present (duration, size, resolution,
// recorded_at, location, camera, ...). A nil/empty metadata blob is treated
// as an empty object.
func setStreamRotation(stream *models.Stream, rotation int) error {
	meta := map[string]any{}
	if len(stream.Metadata) > 0 && string(stream.Metadata) != "null" {
		if err := json.Unmarshal(stream.Metadata, &meta); err != nil {
			return fmt.Errorf("failed to read stream metadata: %w", err)
		}
	}
	meta["rotation"] = rotation
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to encode stream metadata: %w", err)
	}
	stream.Metadata = datatypes.JSON(data)
	return nil
}

func (s *StreamServiceImpl) DeleteStream(ctx context.Context, id uuid.UUID) error {
	slog.Info("start deleting", "stream id", id)
	stream, err := s.repo.Read(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStreamNotFound
		}
		return fmt.Errorf("delete stream error: %w", err)
	}
	if stream.Status == models.StatusPublished {
		return ErrCannotDeletePublished
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

		thumbKey := fmt.Sprintf("thumbnails/%s.jpg", id)
		if err := s.storage.Delete(ctx, thumbKey); err != nil {
			slog.Error("failed to delete thumbnail", "key", thumbKey, "error", err)
		}
	}

	if tasks, err := stream.ProcessingTasks(); err == nil && stream.Status == models.StatusProcessing {
		for _, t := range tasks {
			if t.TaskID != nil {
				if err := s.queue.TerminateTask(ctx, *t.TaskID); err != nil {
					slog.Error("terminate task",
						"stream uuid", stream.ID,
						"task id", *t.TaskID,
						"error", err.Error())
				}
			}
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete stream: %w", err)
	}
	s.recordEvent(ctx, stream, "stream.deleted", nil)
	s.recordFaceCascade(ctx, id)

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

func (s *StreamServiceImpl) ListUserStreams(ctx context.Context, userID uuid.UUID, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
	filter.OwnerID = &userID
	return s.ListStreams(ctx, filter)
}

func (s *StreamServiceImpl) PublishStream(ctx context.Context, streamID uuid.UUID) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		return fmt.Errorf("error read stream from repo: %w", err)
	}
	if stream.Status != models.StatusReady {
		return ErrCannotPublish
	}
	stream.Status = models.StatusPublished
	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyComplete(stream)
	s.recordEvent(ctx, stream, "stream.published", nil)
	return nil
}

func (s *StreamServiceImpl) UnpublishStream(ctx context.Context, streamID uuid.UUID) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		return fmt.Errorf("error read stream from repo: %w", err)
	}
	if stream.Status != models.StatusPublished {
		return ErrCannotUnpublish
	}
	stream.Status = models.StatusReady
	if err = s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyUpdate(stream)
	s.recordEvent(ctx, stream, "stream.unpublished", nil)
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
	if stream.Status == models.StatusReady {
		s.recordEvent(ctx, stream, "stream.ready", nil)
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
	if taskID != nil {
		slog.Info("send transcoder task", "TaskID", *taskID)
	}

	thumbID, err := s.queue.DistributeThumbsnailProcessor(
		ctx,
		stream.ID,
		storageInfo.Key,
	)
	if err != nil {
		slog.Error("failed to enqueue thumbsnail task for", "stream", stream.ID, "error", err)
	}
	if thumbID != nil {
		slog.Debug("Task for generate thumbsnail in queue", "taskID", *thumbID)
	}

	facesID, err := s.queue.DistributeFacesProcessor(
		ctx,
		stream.ID,
		storageInfo.Key,
	)
	if err != nil {
		slog.Error("failed to enqueue faces task for", "stream", stream.ID, "error", err)
	}
	if facesID != nil {
		slog.Debug("Task for detect faces in queue", "taskID", *facesID)
	}

	tasks := []models.StreamProcessingTask{
		{TaskType: models.TaskTypeTranscode, Progress: 0, Steps: []string{"convertation"}, Error: nil, TaskID: taskID},
		{TaskType: models.TaskTypeThumbnail, Progress: 0, Steps: []string{"Generating thumbnail preview"}, Error: nil, TaskID: thumbID},
		{TaskType: models.TaskTypeFaces, Progress: 0, Steps: []string{"Detecting faces"}, Error: nil, TaskID: facesID},
	}
	if err := stream.SetInitialTasks(tasks); err != nil {
		return fmt.Errorf("failed to set initial processing: %w", err)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("failed to update processing: %w", err)
	}
	s.notifyUpdate(stream)
	s.recordEvent(ctx, stream, "stream.upload.completed", nil)

	return nil
}

// RequestStreamExport queues a single-file export of the stream and returns the
// resulting state. Re-requesting an export that is already ready is a no-op;
// one that is pending is not re-queued, because the worker is already on it.
func (s *StreamServiceImpl) RequestStreamExport(ctx context.Context, streamID uuid.UUID, userUUID uuid.UUID, email string) (*StreamExportInfo, error) {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStreamNotFound
		}
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	if stream.OwnerID != userUUID {
		return nil, ErrStreamForbidden
	}
	if stream.Status != models.StatusReady && stream.Status != models.StatusPublished {
		return nil, ErrStreamNotReady
	}

	existing, err := s.exportRepo.ReadByStream(ctx, streamID)
	switch {
	case err == nil:
		if existing.Status == models.ExportStatusReady {
			return exportInfo(existing, stream.Title), nil
		}
		if existing.Status == models.ExportStatusPending {
			return exportInfo(existing, stream.Title), nil
		}
		// Failed: retry it below. The reset is conditional on the row still
		// being failed, so of two retries that arrive together only one gets
		// to queue a mux and the other is answered with the pending row.
		reset, rerr := s.exportRepo.ResetFailed(ctx, existing.ID)
		if rerr != nil {
			return nil, rerr
		}
		if !reset {
			current, cerr := s.exportRepo.ReadByStream(ctx, streamID)
			if cerr != nil {
				return nil, cerr
			}
			return exportInfo(current, stream.Title), nil
		}
		existing.Status = models.ExportStatusPending
		existing.Error = ""
		existing.Size = 0
	case repository.IsExportNotFound(err):
		export := &models.StreamExport{
			StreamID: streamID,
			UserID:   userUUID.String(),
			Status:   models.ExportStatusPending,
		}
		if err := s.exportRepo.Create(ctx, export); err != nil {
			return nil, err
		}
		existing = export
	default:
		return nil, fmt.Errorf("failed to read stream export: %w", err)
	}

	if _, err := s.exportQueue.DistributeVideoExport(ctx, streamID, userUUID, email); err != nil {
		// Surface the queue failure and drop the row back to a clean state so a
		// retry does not look like a duplicate no-op.
		existing.Status = models.ExportStatusFailed
		existing.Error = "failed to queue export"
		if uerr := s.exportRepo.Update(ctx, existing); uerr != nil {
			slog.Error("failed to mark export as failed", "stream", streamID, "error", uerr)
		}
		return nil, fmt.Errorf("failed to queue export: %w", err)
	}

	return exportInfo(existing, stream.Title), nil
}

// GetStreamExport reports the current export state for the owner. A stream that
// was never exported reports pending=false rather than an error, so the UI can
// tell "nothing yet" from "still working".
func (s *StreamServiceImpl) GetStreamExport(ctx context.Context, streamID uuid.UUID, userUUID uuid.UUID) (*StreamExportInfo, error) {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStreamNotFound
		}
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}
	if stream.OwnerID != userUUID {
		return nil, ErrStreamForbidden
	}

	export, err := s.exportRepo.ReadByStream(ctx, streamID)
	if repository.IsExportNotFound(err) {
		// Not an error: the UI asks before anyone has pressed the button, and
		// has to be able to say so. The name is filled in anyway so the client
		// can offer the same one for an export started later.
		return &StreamExportInfo{
			StreamID:  streamID,
			Status:    models.ExportStatusPending,
			Requested: false,
			FileName:  downloadFileName(stream.Title),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read stream export: %w", err)
	}
	return exportInfo(export, stream.Title), nil
}

// OpenStreamDownload opens the cached mp4 for the owner. It is a straight proxy
// of the object the export worker wrote: no transcoding happens here, so the
// cost is one storage read regardless of the video length.
func (s *StreamServiceImpl) OpenStreamDownload(ctx context.Context, streamID uuid.UUID, userUUID uuid.UUID) (*DownloadStreamInfo, error) {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStreamNotFound
		}
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}
	if stream.OwnerID != userUUID {
		return nil, ErrStreamForbidden
	}

	export, err := s.exportRepo.ReadByStream(ctx, streamID)
	if repository.IsExportNotFound(err) {
		return nil, ErrExportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read stream export: %w", err)
	}
	switch export.Status {
	case models.ExportStatusPending:
		return nil, ErrExportPending
	case models.ExportStatusFailed:
		return nil, fmt.Errorf("export failed: %s", export.Error)
	}

	key := export.ObjectKey()
	content, size, err := s.storage.Download(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to open export: %w", err)
	}
	if size <= 0 {
		_ = content.Close()
		return nil, fmt.Errorf("export object is empty")
	}

	return &DownloadStreamInfo{
		Content:     content,
		ContentType: "video/mp4",
		FileName:    downloadFileName(stream.Title),
		Size:        size,
	}, nil
}

// CompleteStreamExport records the worker's outcome and tells the owner's open
// websocket connections how it went, ready or failed.
func (s *StreamServiceImpl) CompleteStreamExport(ctx context.Context, streamID uuid.UUID, size int64, exportErr string) error {
	export, err := s.exportRepo.ReadByStream(ctx, streamID)
	if err != nil {
		if repository.IsExportNotFound(err) {
			return ErrExportNotFound
		}
		return fmt.Errorf("failed to read stream export: %w", err)
	}

	// The worker reports the outcome itself and asynq's error handler reports
	// the same task again if it ends in error, so a second report for one
	// export is expected rather than exceptional. Only a pending row has an
	// outcome still to record: a retry resets the row to pending first, so its
	// own report is never swallowed by this guard. Taking the first report also
	// keeps the stored error the real one instead of a wrapper around it.
	if export.Status != models.ExportStatusPending {
		slog.Warn("ignoring duplicate export report", "stream", streamID, "status", export.Status, "error", exportErr)
		return nil
	}

	if exportErr != "" {
		export.Status = models.ExportStatusFailed
		export.Error = exportErr
		export.Size = 0
	} else {
		export.Status = models.ExportStatusReady
		export.Error = ""
		export.Size = size
	}
	if err := s.exportRepo.Update(ctx, export); err != nil {
		return err
	}

	if s.hub != nil {
		// user_id is a text column, so a bad value must not take the process
		// down from an RPC handler.
		ownerID, err := uuid.Parse(export.UserID)
		if err != nil {
			slog.Error("export owner is not a uuid, cannot notify", "stream", streamID, "user_id", export.UserID)
			return nil
		}
		// Both outcomes are announced. A silent failure would leave the owner
		// watching a spinner until they reloaded the page.
		if export.Status == models.ExportStatusReady {
			s.hub.SendMessgeToOwner(ownerID, gin.H{
				"type": "STREAM_EXPORT_READY",
				"payload": gin.H{
					"stream_id": streamID,
					"size":      export.Size,
				},
			})
		} else {
			s.hub.SendMessgeToOwner(ownerID, gin.H{
				"type": "STREAM_EXPORT_FAILED",
				"payload": gin.H{
					"stream_id": streamID,
					"error":     export.Error,
				},
			})
		}
	}
	return nil
}

func exportInfo(export *models.StreamExport, title string) *StreamExportInfo {
	return &StreamExportInfo{
		StreamID:  export.StreamID,
		Status:    export.Status,
		Size:      export.Size,
		Error:     export.Error,
		Requested: true,
		FileName:  downloadFileName(title),
	}
}

// downloadFileName turns a stream title into a safe attachment name. Titles are
// user input, so anything that could break out of the header, confuse a path or
// upset a filesystem is replaced; the extension is always mp4 because that is
// what the export worker writes.
func downloadFileName(title string) string {
	name := strings.TrimSpace(title)
	if ext := path.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7f:
			return -1
		case strings.ContainsRune(`\/:*?"<>|`, r):
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(strings.Trim(name, "."))
	if name == "" {
		return defaultDownloadName
	}
	if len(name) > maxDownloadNameLen {
		name = strings.TrimSpace(name[:maxDownloadNameLen])
	}
	return name + ".mp4"
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
	s.recordEvent(ctx, stream, "stream.upload.started", map[string]any{"filename": req.Filename})
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
	if taskID != nil {
		slog.Info("send transcoder task", "TaskID", *taskID)
	}

	thumbID, err := s.queue.DistributeThumbsnailProcessor(
		ctx,
		stream.ID,
		storageInfo.Key,
	)
	if err != nil {
		slog.Error("failed to enqueue thumbsnail task for", "stream", stream.ID, "error", err)
	}
	if thumbID != nil {
		slog.Debug("Task for generate thumbsnail in queue", "taskID", *thumbID)
	}

	facesID, err := s.queue.DistributeFacesProcessor(
		ctx,
		stream.ID,
		storageInfo.Key,
	)
	if err != nil {
		slog.Error("failed to enqueue faces task for", "stream", stream.ID, "error", err)
	}
	if facesID != nil {
		slog.Debug("Task for detect faces in queue", "taskID", *facesID)
	}

	tasks := []models.StreamProcessingTask{
		{TaskType: models.TaskTypeTranscode, Progress: 0, Steps: []string{"convertation"}, Error: nil, TaskID: taskID},
		{TaskType: models.TaskTypeThumbnail, Progress: 0, Steps: []string{"Generating thumbnail preview"}, Error: nil, TaskID: thumbID},
		{TaskType: models.TaskTypeFaces, Progress: 0, Steps: []string{"Detecting faces"}, Error: nil, TaskID: facesID},
	}
	if err := stream.SetInitialTasks(tasks); err != nil {
		return fmt.Errorf("failed to set initial processing: %w", err)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("failed to update processing: %w", err)
	}
	s.notifyUpdate(stream)
	s.recordEvent(ctx, stream, "stream.upload.completed", nil)

	return nil
}

func (s *StreamServiceImpl) UpdateStreamMetadata(ctx context.Context, req *UpdateStreamMetadataRequest) error {
	stream, err := s.repo.Read(ctx, req.StreamUUID)
	if err != nil {
		return fmt.Errorf("error read stream from repository: %w", err)
	}
	meta := req.Metadata
	// The transcoder never sends a rotation value, so a zero rotation in the
	// incoming metadata means "not provided" — carry over any rotation that was
	// saved by the owner through the edit form instead of silently resetting it
	// to 0 on every (re-)transcode.
	if meta.Rotation == 0 {
		if existing, err := stream.GetMetadata(); err == nil && existing != nil {
			meta.Rotation = existing.Rotation
		}
	}
	if err := stream.SetMetadata(&meta); err != nil {
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

	tasks, err := stream.ProcessingTasks()
	if err != nil {
		return fmt.Errorf("error read processing tasks: %w", err)
	}

	if req.Processing.TaskID == nil {
		for _, t := range tasks {
			if t.TaskType == req.Processing.TaskType && t.TaskID != nil {
				req.Processing.TaskID = t.TaskID
				break
			}
		}
	}

	errMsg := ""
	if req.Processing.Error != nil {
		errMsg = *req.Processing.Error
	}
	var processingErr *string
	if errMsg != "" {
		processingErr = req.Processing.Error
	}

	if err := stream.SetTaskProgress(req.Processing.TaskType, req.Processing.Progress, req.Processing.Steps, processingErr, req.Processing.TaskID); err != nil {
		return fmt.Errorf("error update processing: %w", err)
	}

	if errMsg != "" && req.Processing.TaskType == models.TaskTypeTranscode && stream.Status == models.StatusProcessing {
		stream.Status = models.StatusError
	}

	if req.Processing.TaskType == models.TaskTypeFaces &&
		req.Processing.Progress >= 100 &&
		errMsg == "" {
		stream.FacesDetected = true
	}

	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}

	switch req.Processing.TaskType {
	case models.TaskTypeTranscode:
		switch {
		case errMsg != "":
			s.recordEvent(ctx, stream, "stream.transcode.failed", map[string]any{"error": errMsg})
		case req.Processing.Progress >= 100:
			s.recordEvent(ctx, stream, "stream.transcode.finish", map[string]any{"progress": req.Processing.Progress})
		case req.Processing.Progress == 0:
			s.recordEvent(ctx, stream, "stream.transcode.started", nil)
		}
	default:
		// thumbnail progress is only written into the task; no feed event.
	}

	s.notifyUpdate(stream)
	return nil
}

func (s *StreamServiceImpl) ReprocessStream(ctx context.Context, streamID uuid.UUID) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStreamNotFound
		}
		return fmt.Errorf("error read stream from repository: %w", err)
	}
	return s.reprocessStreamTask(ctx, stream)
}

// reprocessStreamTask re-enqueues every task that carries an error. Both the
// single and the batch entry point go through it so they cannot drift apart.
func (s *StreamServiceImpl) reprocessStreamTask(ctx context.Context, stream *models.Stream) error {
	tasks, err := stream.ProcessingTasks()
	if err != nil {
		return fmt.Errorf("error read processing tasks: %w", err)
	}

	failed := false
	for _, t := range tasks {
		if t.Error != nil && *t.Error != "" {
			failed = true
			break
		}
	}
	if !failed {
		return ErrStreamNotInErrorState
	}

	storageInfo, err := stream.GetStorageInfo()
	if err != nil {
		return fmt.Errorf("error read storage info: %w", err)
	}
	exists, err := s.storage.Exists(ctx, storageInfo.Key)
	if err != nil {
		return fmt.Errorf("error check source file: %w", err)
	}
	if !exists {
		return ErrSourceFileMissing
	}

	for i := range tasks {
		if tasks[i].Error == nil || *tasks[i].Error == "" {
			continue
		}

		var taskID *string
		switch tasks[i].TaskType {
		case models.TaskTypeTranscode:
			taskID, err = s.queue.ReprocessVideoTranscoding(ctx, stream.ID, storageInfo.Key)
		case models.TaskTypeThumbnail:
			taskID, err = s.queue.ReprocessThumbsnailProcessor(ctx, stream.ID, storageInfo.Key)
		case models.TaskTypeFaces:
			taskID, err = s.queue.ReprocessFacesProcessor(ctx, stream.ID, storageInfo.Key)
		default:
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to enqueue %s reprocess: %w", tasks[i].TaskType, err)
		}

		tasks[i].TaskID = taskID
		tasks[i].Error = nil
		tasks[i].Progress = 0
		tasks[i].Steps = []string{}
		if taskID != nil {
			slog.Info("reprocess enqueued", "stream", stream.ID, "task_type", tasks[i].TaskType, "task_id", *taskID)
		}
	}

	if stream.Status == models.StatusError {
		stream.Status = models.StatusProcessing
	}
	if err := stream.SetInitialTasks(tasks); err != nil {
		return fmt.Errorf("error reset processing: %w", err)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyUpdate(stream)
	s.recordEvent(ctx, stream, "stream.reprocessed", nil)
	return nil
}

// ReprocessStreamBatch is ReprocessStream for a selection of streams, reporting
// per-id results. Ownership is checked here rather than left to middleware: the
// single endpoint is guarded per resource, a batch carrying many ids cannot be.
func (s *StreamServiceImpl) ReprocessStreamBatch(ctx context.Context, userID uuid.UUID, isAdmin bool, ids []uuid.UUID) (*StreamBatchResult, error) {
	streams, err := s.repo.ReadMany(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("error read streams from repository: %w", err)
	}

	byID := make(map[uuid.UUID]*models.Stream, len(streams))
	for _, st := range streams {
		byID[st.ID] = st
	}

	result := &StreamBatchResult{}
	for _, id := range ids {
		stream, ok := byID[id]
		if !ok {
			result.Failed = append(result.Failed, StreamBatchFailure{StreamID: id, Reason: StreamBatchReasonNotFound})
			continue
		}
		if stream.OwnerID != userID && !isAdmin {
			result.Failed = append(result.Failed, StreamBatchFailure{StreamID: id, Reason: StreamBatchReasonForbidden})
			continue
		}
		if err := s.reprocessStreamTask(ctx, stream); err != nil {
			reason := StreamBatchReasonInternal
			switch {
			case errors.Is(err, ErrStreamNotInErrorState):
				reason = StreamBatchReasonNotFailed
			case errors.Is(err, ErrSourceFileMissing):
				reason = StreamBatchReasonSourceMissing
			case errors.Is(err, ErrStreamForbidden):
				reason = StreamBatchReasonForbidden
			case errors.Is(err, ErrStreamNotFound):
				reason = StreamBatchReasonNotFound
			default:
				slog.Error("reprocess batch: stream failed",
					"stream", id, "error", err.Error())
			}
			result.Failed = append(result.Failed, StreamBatchFailure{StreamID: id, Reason: reason})
			continue
		}
		result.Processed = append(result.Processed, id)
	}
	return result, nil
}

// ForceStreamError moves a stream that is stuck in processing into the error
// state so the owner can hit Reprocess. Nothing else can do this: the
// transition into error is normally written by a worker reporting its own
// failure, so a stream whose worker died waits in processing forever.
func (s *StreamServiceImpl) ForceStreamError(ctx context.Context, streamID, userID uuid.UUID, isAdmin bool) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStreamNotFound
		}
		return fmt.Errorf("error read stream from repository: %w", err)
	}
	if stream.OwnerID != userID && !isAdmin {
		return ErrStreamForbidden
	}
	return s.forceProcessingError(ctx, stream)
}

// ForceStreamErrorBatch is ForceStreamError for several streams at once,
// reporting per-id results so the owner sees which ones refused.
func (s *StreamServiceImpl) ForceStreamErrorBatch(ctx context.Context, userID uuid.UUID, isAdmin bool, ids []uuid.UUID) (*StreamBatchResult, error) {
	streams, err := s.repo.ReadMany(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("error read streams from repository: %w", err)
	}

	byID := make(map[uuid.UUID]*models.Stream, len(streams))
	for _, st := range streams {
		byID[st.ID] = st
	}

	result := &StreamBatchResult{}
	for _, id := range ids {
		stream, ok := byID[id]
		if !ok {
			result.Failed = append(result.Failed, StreamBatchFailure{StreamID: id, Reason: StreamBatchReasonNotFound})
			continue
		}
		if stream.OwnerID != userID && !isAdmin {
			result.Failed = append(result.Failed, StreamBatchFailure{StreamID: id, Reason: StreamBatchReasonForbidden})
			continue
		}
		if err := s.forceProcessingError(ctx, stream); err != nil {
			reason := StreamBatchReasonInternal
			switch {
			case errors.Is(err, ErrStreamNotProcessing):
				reason = StreamBatchReasonNotProcessing
			case errors.Is(err, ErrStreamForbidden):
				reason = StreamBatchReasonForbidden
			case errors.Is(err, ErrStreamNotFound):
				reason = StreamBatchReasonNotFound
			default:
				slog.Error("force error batch: stream failed",
					"stream", id, "error", err.Error())
			}
			result.Failed = append(result.Failed, StreamBatchFailure{StreamID: id, Reason: reason})
			continue
		}
		result.Processed = append(result.Processed, id)
	}
	return result, nil
}

// forceProcessingError does the actual transition. Both the single and the
// batch entry point go through it so they cannot drift apart.
//
// The tasks are terminated before the error is recorded. A worker that is
// still alive would otherwise report completion afterwards and flip the
// stream back to ready, undoing what the owner just asked for. Termination
// failures are logged and ignored: a task that no longer exists is the
// common case here, not a problem.
func (s *StreamServiceImpl) forceProcessingError(ctx context.Context, stream *models.Stream) error {
	if stream.Status != models.StatusProcessing {
		return ErrStreamNotProcessing
	}

	tasks, err := stream.ProcessingTasks()
	if err != nil {
		return fmt.Errorf("error read processing tasks: %w", err)
	}

	for _, t := range tasks {
		if t.TaskID == nil {
			continue
		}
		if err := s.queue.TerminateTask(ctx, *t.TaskID); err != nil {
			slog.Warn("force error: terminate task failed",
				"stream", stream.ID,
				"task_id", *t.TaskID,
				"error", err.Error())
		}
	}

	// ReprocessStream only re-enqueues tasks that carry an error, so a stream
	// with an empty processing array would stay stuck even after this call.
	// Seed the full pipeline in that case: the point of the button is to run
	// the whole thing again.
	if len(tasks) == 0 {
		tasks = []models.StreamProcessingTask{
			{TaskType: models.TaskTypeTranscode},
			{TaskType: models.TaskTypeThumbnail},
			{TaskType: models.TaskTypeFaces},
		}
	}

	for i := range tasks {
		msg := ForceErrorMessage
		tasks[i].Error = &msg
	}

	stream.Status = models.StatusError
	if err := stream.SetInitialTasks(tasks); err != nil {
		return fmt.Errorf("error set processing: %w", err)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	slog.Info("stream forced to error", "stream", stream.ID, "tasks", len(tasks))
	s.notifyUpdate(stream)
	return nil
}

func (s *StreamServiceImpl) ProcessFacesStream(ctx context.Context, streamID uuid.UUID) error {
	stream, err := s.repo.Read(ctx, streamID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStreamNotFound
		}
		return fmt.Errorf("error read stream from repository: %w", err)
	}

	if err := s.enqueueFaces(ctx, stream); err != nil {
		return err
	}
	return nil
}

func (s *StreamServiceImpl) enqueueFaces(ctx context.Context, stream *models.Stream) error {
	storageInfo, err := stream.GetStorageInfo()
	if err != nil {
		return fmt.Errorf("error read storage info: %w", err)
	}
	exists, err := s.storage.Exists(ctx, storageInfo.Key)
	if err != nil {
		return fmt.Errorf("error check source file: %w", err)
	}
	inputPath := storageInfo.Key
	if !exists {
		hlsKey := fmt.Sprintf("processed/%s/index.m3u8", stream.ID)
		hlsExists, err := s.storage.Exists(ctx, hlsKey)
		if err != nil {
			return fmt.Errorf("error check hls playlist: %w", err)
		}
		if !hlsExists {
			return ErrSourceFileMissing
		}
		slog.Info("source missing, falling back to hls", "stream", stream.ID, "hls", hlsKey)
		inputPath = hlsKey
	}

	facesID, err := s.queue.ReprocessFacesProcessor(ctx, stream.ID, inputPath)
	if err != nil {
		return fmt.Errorf("failed to enqueue faces task: %w", err)
	}

	if err := stream.SetTaskProgress(models.TaskTypeFaces, 0, []string{"Detecting faces"}, nil, facesID); err != nil {
		return fmt.Errorf("error reset processing: %w", err)
	}
	if err := s.repo.Update(ctx, stream); err != nil {
		return fmt.Errorf("error update stream in repo: %w", err)
	}
	s.notifyUpdate(stream)
	if facesID != nil {
		slog.Info("faces task enqueued", "stream", stream.ID, "task_id", *facesID)
	}
	return nil
}

type FacesBatchResult struct {
	Processed []uuid.UUID         `json:"processed"`
	Failed    []FacesBatchFailure `json:"failed"`
}

type FacesBatchFailure struct {
	StreamID uuid.UUID `json:"stream_id"`
	Reason   string    `json:"reason"`
}

// StreamBatchResult reports per-stream outcomes of a batch mutation over
// streams. It mirrors FacesBatchResult deliberately: both report which ids
// were touched and why the rest were skipped, so the client renders partial
// success the same way either way.
type StreamBatchResult struct {
	Processed []uuid.UUID          `json:"processed"`
	Failed    []StreamBatchFailure `json:"failed"`
}

type StreamBatchFailure struct {
	StreamID uuid.UUID `json:"stream_id"`
	Reason   string    `json:"reason"`
}

const (
	StreamBatchReasonForbidden     = "forbidden"
	StreamBatchReasonNotFound      = "not found"
	StreamBatchReasonNotProcessing = "not processing"

	// Reprocess-only reasons. A stream can refuse to reprocess while owning no
	// processing task at all, so "not processing" cannot stand in for it.
	StreamBatchReasonNotFailed     = "not failed"
	StreamBatchReasonSourceMissing = "source removed"

	// StreamBatchReasonInternal covers failures that are neither a refused
	// request nor a state mismatch, e.g. the database write. It is reported
	// separately so an operator can tell "your request was declined" apart
	// from "something on our side broke".
	StreamBatchReasonInternal = "internal error"

	// ForceErrorMessage is written into every processing task when the owner
	// gives up on a stuck stream. It is what the owner reads on the stream
	// page instead of a worker-reported failure, so it says who stopped it.
	ForceErrorMessage = "processing was stopped manually by the owner"
)

const (
	FacesBatchReasonForbidden = "forbidden"
	FacesBatchReasonNotFound  = "not found"
	FacesBatchReasonSource    = "source missing"
	FacesBatchReasonError     = "error"
)

func (s *StreamServiceImpl) ProcessFacesBatch(ctx context.Context, userID uuid.UUID, isAdmin bool, ids []uuid.UUID) (*FacesBatchResult, error) {
	streams, err := s.repo.ReadMany(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("error read streams from repository: %w", err)
	}

	byID := make(map[uuid.UUID]*models.Stream, len(streams))
	for _, st := range streams {
		byID[st.ID] = st
	}

	result := &FacesBatchResult{}
	for _, id := range ids {
		stream, ok := byID[id]
		if !ok {
			result.Failed = append(result.Failed, FacesBatchFailure{StreamID: id, Reason: FacesBatchReasonNotFound})
			continue
		}
		if stream.OwnerID != userID && !isAdmin {
			result.Failed = append(result.Failed, FacesBatchFailure{StreamID: id, Reason: FacesBatchReasonForbidden})
			continue
		}
		if err := s.enqueueFaces(ctx, stream); err != nil {
			reason := FacesBatchReasonError
			if errors.Is(err, ErrSourceFileMissing) {
				reason = FacesBatchReasonSource
			}
			result.Failed = append(result.Failed, FacesBatchFailure{StreamID: id, Reason: reason})
			continue
		}
		result.Processed = append(result.Processed, id)
	}

	if n := len(result.Processed); n > 0 {
		streammetrics.Lifecycle.WithLabelValues("faces").Add(float64(n))
	}
	return result, nil
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
		return nil, ErrCannotWatch
	}
	fileName := strings.TrimPrefix(req.FileName, "/")
	if fileName == "" || strings.Contains(fileName, "..") || strings.Contains(fileName, "\\") {
		return nil, ErrInvalidFileName
	}
	key := path.Join("processed", req.StreamUUID.String(), fileName)
	content, size, err := s.storage.Download(ctx, key)
	if err != nil {
		// Fallback: a missing TS segment (a "dead" HLS segment) is replaced
		// with the concatenation of its direct neighbours, so playback does
		// not hard-fail on 404-level gaps in the playlist.
		if errors.Is(err, storage.ErrNotFound) && strings.HasSuffix(fileName, ".ts") {
			merged, mergedSize, ferr := s.concatNeighbourSegments(ctx, req.StreamUUID, fileName)
			if ferr != nil {
				return nil, fmt.Errorf("error download file from storage: %w", ferr)
			}
			if merged != nil {
				streammetrics.HLSRequests.WithLabelValues("200-fallback").Inc()
				slog.Warn("hls segment missing, served concatenated neighbours",
					"stream", req.StreamUUID.String(), "file", fileName, "size", mergedSize)
				return &GetFileByKeyResponse{
					Content:     merged,
					ContentType: getContentType(fileName),
					Size:        mergedSize,
				}, nil
			}
		}
		return nil, fmt.Errorf("error download file from storage: %w", err)
	}
	return &GetFileByKeyResponse{
		Content:     content,
		ContentType: getContentType(req.FileName),
		Size:        size,
	}, nil
}

// concatNeighbourSegments merges the TS segments directly around a missing one
// (seg_<n-1> + seg_<n+1>, walking outwards until a run of missing segments ends)
// into a single byte slice. It returns a nil reader when there is no recoverable
// neighbour, letting the caller surface the original storage error.
func (s *StreamServiceImpl) concatNeighbourSegments(ctx context.Context, streamUUID uuid.UUID, fileName string) (io.ReadCloser, int64, error) {
	idx, ok := segmentIndex(fileName)
	if !ok {
		return nil, 0, nil
	}

	const maxRadius = 3
	var (
		merged bytes.Buffer
		total  int64
		found  bool
	)
	for i := idx - maxRadius; i <= idx+maxRadius; i++ {
		if i == idx || i < 0 {
			continue
		}
		key := path.Join("processed", streamUUID.String(), fmt.Sprintf("seg_%d.ts", i))
		rc, size, err := s.storage.Download(ctx, key)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				slog.Debug("hls neighbour segment missing", "stream", streamUUID.String(), "file", key)
				continue
			}
			return nil, 0, err
		}
		n, copyErr := io.Copy(&merged, io.LimitReader(rc, size))
		rc.Close()
		if copyErr != nil {
			return nil, 0, copyErr
		}
		total += n
		found = true
	}
	if !found {
		return nil, 0, nil
	}
	return io.NopCloser(bytes.NewReader(merged.Bytes())), total, nil
}

// segmentIndex extracts the numeric part of an HLS segment filename of the form
// seg_<n>.ts (as produced by ffmpeg's hls_segment_filename), returning false for
// any other filename.
func segmentIndex(fileName string) (int, bool) {
	m := segmentNamePattern.FindStringSubmatch(fileName)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}
