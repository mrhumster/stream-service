package response

import (
	"time"
)

type DownloadResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Filename  string    `json:"filename,omitempty"`
	Size      int64     `json:"size,omitempty"`
	Message   string    `json:"message,omitempty"`
}

func NewDownloadResponse(url string, expiresAt time.Time, filename string, size int64) *DownloadResponse {
	return &DownloadResponse{
		URL:       url,
		ExpiresAt: expiresAt,
		Filename:  filename,
		Size:      size,
		Message:   "Use this URL to download the file. URL expires at " + expiresAt.Format(time.RFC3339),
	}
}
