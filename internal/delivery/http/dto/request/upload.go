package request

import (
	"fmt"
	"mime/multipart"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
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

type UploadPartRequest struct {
	UploadID   string                `form:"uploadID" binding:"required"`
	Partnumber int                   `form:"partNumber" binding:"required,gt=0"`
	Video      *multipart.FileHeader `form:"video" binding:"required"`
}

func (r *UploadPartRequest) ToService(streamUUID, userUUID uuid.UUID) (*service.UploadPartRequest, error) {
	file, err := r.Video.Open()
	if err != nil {
		return nil, err
	}

	if streamUUID == uuid.Nil {
		return nil, fmt.Errorf("StreamUUID can not be nil")
	}

	if userUUID == uuid.Nil {
		return nil, fmt.Errorf("UserUUID can not be nil")
	}

	return &service.UploadPartRequest{
		StreamID:   streamUUID,
		UserID:     userUUID,
		UploadID:   r.UploadID,
		PartNumber: r.Partnumber,
		Data:       file,
		Size:       r.Video.Size,
	}, nil
}

type CompleteUploadRequest struct {
	Parts []models.MultipartPart `json:"parts" binding:"required,min=1,dive"`
}

func (r *CompleteUploadRequest) ToService(streamUUID, userUUID uuid.UUID) (*service.CompleteStreamUploadRequest, error) {
	if streamUUID == uuid.Nil {
		return nil, fmt.Errorf("StreamUUID can not be nil")
	}

	if userUUID == uuid.Nil {
		return nil, fmt.Errorf("UserUUID can not be nil")
	}

	return &service.CompleteStreamUploadRequest{
		UserID:   userUUID,
		StreamID: streamUUID,
		Parts:    r.Parts,
	}, nil
}
