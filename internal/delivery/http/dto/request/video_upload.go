package request

import (
	"mime/multipart"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/service"
)

type VideoUploadRequest struct {
	StreamID   uuid.UUID             `json:"-"`
	UserID     string                `json:"-"`
	File       multipart.File        `json:"-"`
	FileHeader *multipart.FileHeader `json:"-"`
}

func (r *VideoUploadRequest) ToServiceRequest() (*service.UploadVideoRequest, error) {
	userUUID, err := uuid.Parse(r.UserID)
	if err != nil {
		return nil, NewValidationError("invalid user id format")
	}
	contentType := r.FileHeader.Header.Get("Content-Type")
	if !isValidVideoContentType(contentType) {
		return nil, NewValidationError("invalid video format. allowed: mp4, webm, mov")
	}

	const maxFileSize = 5 * 1024 * 1024 * 1024
	if r.FileHeader.Size > maxFileSize {
		return nil, NewValidationError("file too large. maximum size is 5GB")
	}

	filename := r.FileHeader.Filename
	if !isValidVideoExtension(filename) {
		return nil, NewValidationError("invalid file extension. allowed: mp4, webm, mov")
	}

	return &service.UploadVideoRequest{
		StreamID: r.StreamID,
		UserID:   userUUID,
		File:     r.File,
		FileName: filename,
		Size:     r.FileHeader.Size,
	}, nil
}

func isValidVideoContentType(contentType string) bool {
	allowedTypes := []string{
		"video/mp4",
		"video/webm",
		"video/quicktime",
		"video/x-msvideo",
		"video/x-matroska",
		"application/octet-stream",
		"",
	}
	return slices.Contains(allowedTypes, contentType)
}

func isValidVideoExtension(filename string) bool {
	allowedExtensions := []string{".mp4", ".webm", ".mov", ".avi", ".mkv"}
	filename = strings.ToLower(filename)
	for _, ext := range allowedExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}
