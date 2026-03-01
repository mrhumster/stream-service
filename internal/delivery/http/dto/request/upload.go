package request

import (
	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/service"
)

type StartUploadRequest struct {
	FileName    string `json:"file_name" binding:"required,min=3,max=128"`
	TotalSize   int64  `json:"total_size" binding:"required,gt=0"`
	ContentType string `json:"content_type" binding:"required"`
}

func (s *StartUploadRequest) ToService(streamID, userID uuid.UUID) *service.StartUploadRequest {
	return &service.StartUploadRequest{
		StreamID:  streamID,
		UserID:    userID,
		Filename:  s.FileName,
		TotalSize: s.TotalSize,
	}
}
