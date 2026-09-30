package models

import (
	"fmt"

	"github.com/google/uuid"
)

type (
	StreamExportStatus string
)

const (
	ExportStatusPending StreamExportStatus = "pending"
	ExportStatusReady   StreamExportStatus = "ready"
	ExportStatusFailed  StreamExportStatus = "failed"
)

// StreamExport tracks the cached single-file rendition of a stream. The mp4
// itself is written by the export worker to a fixed key, so a stream has at
// most one live export row and the row is only the state of that object.
//
// Status moves pending -> ready, or pending -> failed. Only the worker reports
// the outcome, over the CompleteExport gRPC method.
type StreamExport struct {
	BaseModel

	StreamID uuid.UUID          `gorm:"not null;index"`
	UserID   string             `gorm:"not null"`
	Status   StreamExportStatus `gorm:"not null;default:'pending'"`
	Size     int64              `gorm:"not null;default:0"`
	Error    string
}

// ObjectKey is where the muxed mp4 lives in object storage. The export worker
// derives the same key from the stream id, so this is the single definition of
// the contract between the two sides.
func (e *StreamExport) ObjectKey() string {
	return ExportObjectKey(e.StreamID)
}

// ExportObjectKey builds the storage key of a stream's single-file rendition.
func ExportObjectKey(streamID uuid.UUID) string {
	return fmt.Sprintf("processed/%s/video.mp4", streamID)
}
