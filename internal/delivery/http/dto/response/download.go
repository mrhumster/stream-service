package response

import (
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service"
)

// ExportResponse reports the state of a stream's single-file export. The
// download itself is a separate stream endpoint, so no URL is handed out: the
// browser gets bytes from stream-service, which is the only component that can
// read the private bucket.
type ExportResponse struct {
	StreamID string                    `json:"stream_id"`
	Status   models.StreamExportStatus `json:"status"`
	Size     int64                     `json:"size,omitempty"`
	Error    string                    `json:"error,omitempty"`
}

func NewExportResponse(export *service.StreamExportInfo) *ExportResponse {
	return &ExportResponse{
		StreamID: export.StreamID.String(),
		Status:   export.Status,
		Size:     export.Size,
		Error:    export.Error,
	}
}
