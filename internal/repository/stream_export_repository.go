//go:generate mockgen -source=stream_export_repository.go -destination=./mock/stream_export_repository_mock.go -package=repomock

package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
)

// StreamExportRepository persists the state of a stream's cached mp4. There is
// at most one live row per stream (partial unique index on stream_id), so
// ReadByStream is the lookup used by the status endpoint and the download.
type StreamExportRepository interface {
	Create(ctx context.Context, export *models.StreamExport) error
	ReadByStream(ctx context.Context, streamID uuid.UUID) (*models.StreamExport, error)
	Update(ctx context.Context, export *models.StreamExport) error
	// ResetFailed puts a failed export back to pending, returning false if it
	// was no longer failed, which is how two concurrent retries are resolved.
	ResetFailed(ctx context.Context, exportID uuid.UUID) (bool, error)
}
