package queue

import "github.com/google/uuid"

const (
	TaskVideoTranscoding    = "video:transcode"
	TaskThumbsnailProcessor = "video:thumbsnail"
)

type VideoTranscodingPayload struct {
	StreamUUID uuid.UUID `json:"stream_uuid"`
	InputPath  string    `json:"input_path"`
}

type ThumbsnailProcessorPayload struct {
	StreamUUID uuid.UUID `json:"stream_uuid"`
	InputPath  string    `json:"input_path"`
}
