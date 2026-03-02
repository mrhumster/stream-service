package response

import "github.com/mrhumster/stream-service/internal/domain/models"

type StartUploadResponse struct {
	UploadID string `json:"upload_id"`
	StreamID string `json:"stream_id"`
}

type PartUploadResponse struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

type CompleteUploadRequest struct {
	Parts []models.MultipartPart `json:"parts" binding:"required,min=1"`
}
