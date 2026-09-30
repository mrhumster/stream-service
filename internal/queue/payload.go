package queue

import (
	"time"

	"github.com/google/uuid"
)

const (
	TaskVideoTranscoding    = "video:transcode"
	TaskThumbsnailProcessor = "video:thumbsnail"
	TaskFacesProcessor      = "video:faces"
	TaskVideoExport         = "video:export"
)

// exportTaskTimeout is generous: a long single-file rendition of a long video
// is CPU bound, and asynq would otherwise let a retry start a second mux over
// the same output key.
const exportTaskTimeout = 30 * time.Minute

type VideoTranscodingPayload struct {
	StreamUUID uuid.UUID `json:"stream_uuid"`
	InputPath  string    `json:"input_path"`
}

type ThumbsnailProcessorPayload struct {
	StreamUUID uuid.UUID `json:"stream_uuid"`
	InputPath  string    `json:"input_path"`
}

type FacesProcessorPayload struct {
	StreamUUID uuid.UUID `json:"stream_uuid"`
	InputPath  string    `json:"input_path"`
}

// VideoExportPayload asks the export worker to mux the HLS rendition that
// already exists under processed/<stream_uuid>/ into a single mp4. OwnerEmail
// rides along so the worker can send the ready notification without calling
// identity-service, which has no user lookup.
type VideoExportPayload struct {
	StreamUUID uuid.UUID `json:"stream_uuid"`
	OwnerUUID  uuid.UUID `json:"owner_uuid"`
	OwnerEmail string    `json:"owner_email"`
}
