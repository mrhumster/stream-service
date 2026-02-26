//go:generate mockgen -source=stream_service.go -destination=./mock/stream_service_mock.go -package=servicemock

package service

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
)

type StreamService interface {
	CreateStream(ctx context.Context, req CreateStreamRequest) (*models.Stream, error)
	GetStream(ctx context.Context, id uuid.UUID) (*models.Stream, error)
	UpdateStream(ctx context.Context, id uuid.UUID, req UpdateStreamRequest) (*models.Stream, error)
	DeleteStream(ctx context.Context, id uuid.UUID) error

	ListStreams(ctx context.Context, filter repository.StreamFilter) ([]*models.Stream, int64, error)
	ListUserStreams(ctx context.Context, userID uuid.UUID) ([]*models.Stream, int64, error)

	PublishStream(ctx context.Context, streamID uuid.UUID) error
	UnpublishStream(ctx context.Context, streamID uuid.UUID) error
	UpdateStreamStatus(ctx context.Context, streamID uuid.UUID, status models.StreamStatus) error

	CanUserAccessStream(ctx context.Context, userID uuid.UUID, streamID uuid.UUID) (bool, error)
	UploadVideo(ctx context.Context, req UploadVideoRequest) error
	GenerateDownloadURL(ctx context.Context, streamID uuid.UUID) (*GenerateDownloadURLInfo, error)

	UploadPart(ctx context.Context, req UploadPartRequest) (*PartInfo, error)
	StartStreamUpload(ctx context.Context, streamID uuid.UUID, userID uuid.UUID) (*UploadInfo, error)
	CompleteStreamUpload(ctx context.Context, streamID uuid.UUID, userID uuid.UUID, parts []PartInfo) error
}

type UploadPartRequest struct {
	StreamID   uuid.UUID
	UserID     uuid.UUID
	UploadID   string
	PartNumber int
	Data       io.Reader
	Size       int64
}

type PartInfo struct {
	PartNumber int
	ETag       string
}

type GenerateDownloadURLInfo struct {
	DownloadURL *url.URL
	ExpiresAt   time.Time
	FileName    string
	Size        int64
}

type CreateStreamRequest struct {
	Title       string
	Description string
	Visibility  models.StreamVisibility
	Tags        []string
	OwnerID     uuid.UUID
}

type UpdateStreamRequest struct {
	Title       *string
	Description *string
	Visibility  *models.StreamVisibility
	Tags        *[]string
}

type UploadInfo struct {
	UploadID string
	StreamID uuid.UUID
}

type UploadVideoRequest struct {
	StreamID uuid.UUID
	UserID   uuid.UUID
	File     io.Reader
	FileName string
	Size     int64
}
