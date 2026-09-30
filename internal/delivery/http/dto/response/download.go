package response

import (
	"errors"
	"time"

	"github.com/mrhumster/stream-service/internal/service"
)

type DownloadResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Filename  string    `json:"filename,omitempty"`
	Size      int64     `json:"size,omitempty"`
	Message   string    `json:"message,omitempty"`
}

func NewDownloadResponse(serviceResponse *service.GenerateDownloadURLInfo) (*DownloadResponse, error) {
	if serviceResponse.DownloadURL == nil {
		return nil, errors.New("url in response from GenerateDownloadURL is nil")
	}

	if serviceResponse.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("expire urls date in response from GenerateDownloadURL incorrect")
	}
	return &DownloadResponse{
		URL:       serviceResponse.DownloadURL.String(),
		ExpiresAt: serviceResponse.ExpiresAt,
		Filename:  serviceResponse.FileName,
		Size:      serviceResponse.Size,
		Message:   "Use this URL to download the file. URL expires at " + serviceResponse.ExpiresAt.Format(time.RFC3339),
	}, nil
}
