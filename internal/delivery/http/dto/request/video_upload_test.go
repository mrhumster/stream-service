package request

import (
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoUploadRequest_ToServiceRequest(t *testing.T) {
	streamID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name        string
		contentType string
		filename    string
		wantErr     bool
	}{
		{name: "valid mp4", contentType: "video/mp4", filename: "video.mp4", wantErr: false},
		{name: "valid webm", contentType: "video/webm", filename: "clip.webm", wantErr: false},
		{name: "valid mov", contentType: "video/quicktime", filename: "clip.mov", wantErr: false},
		{name: "valid avi", contentType: "video/x-msvideo", filename: "clip.avi", wantErr: false},
		{name: "valid mkv", contentType: "video/x-matroska", filename: "clip.mkv", wantErr: false},
		{name: "valid 3gp", contentType: "video/3gpp", filename: "clip.3gp", wantErr: false},
		{name: "valid 3g2", contentType: "video/3gpp2", filename: "clip.3g2", wantErr: false},
		{name: "valid uppercase extension 3GP", contentType: "video/3gpp", filename: "CLIP.3GP", wantErr: false},
		{name: "valid octet-stream", contentType: "application/octet-stream", filename: "video.mp4", wantErr: false},
		{name: "valid empty content type", contentType: "", filename: "video.mp4", wantErr: false},
		{name: "invalid 3gp content type", contentType: "video/x-nonsense", filename: "clip.3gp", wantErr: true},
		{name: "invalid extension 3gp", contentType: "video/mp4", filename: "clip.3gpx", wantErr: true},
		{name: "invalid content type", contentType: "text/plain", filename: "video.mp4", wantErr: true},
		{name: "invalid extension", contentType: "video/mp4", filename: "video.exe", wantErr: true},
		{name: "invalid content type and extension", contentType: "text/plain", filename: "video.exe", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := textproto.MIMEHeader{}
			if tt.contentType != "" {
				header.Set("Content-Type", tt.contentType)
			}

			req := &VideoUploadRequest{
				StreamID:   streamID,
				UserID:     userID,
				File:       nil,
				FileHeader: &multipart.FileHeader{
					Filename: tt.filename,
					Header:   header,
				},
			}

			res, err := req.ToServiceRequest()
			if tt.wantErr {
				require.Error(t, err)
				_, ok := err.(*ValidationError)
				assert.True(t, ok, "expected ValidationError")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, streamID, res.StreamID)
			assert.Equal(t, userID, res.UserID)
			assert.Equal(t, tt.filename, res.FileName)
		})
	}
}

func TestIsValidVideoContentType(t *testing.T) {
	valid := []string{
		"video/mp4",
		"video/webm",
		"video/quicktime",
		"video/x-msvideo",
		"video/x-matroska",
		"video/3gpp",
		"video/3gpp2",
		"application/octet-stream",
		"",
	}
	for _, ct := range valid {
		assert.Truef(t, isValidVideoContentType(ct), "expected %q to be valid", ct)
	}

	invalid := []string{"text/plain", "video/3gpp3", "image/png", "video/xyz"}
	for _, ct := range invalid {
		assert.Falsef(t, isValidVideoContentType(ct), "expected %q to be invalid", ct)
	}
}

func TestIsValidVideoExtension(t *testing.T) {
	valid := []string{"video.mp4", "clip.webm", "clip.mov", "clip.avi", "clip.mkv", "clip.3gp", "clip.3g2", "CLIP.3GP"}
	for _, name := range valid {
		assert.Truef(t, isValidVideoExtension(name), "expected %q to be valid", name)
	}

	invalid := []string{"video.exe", "clip.3gpx", "video", "clip.m4v", "clip"}
	for _, name := range invalid {
		assert.Falsef(t, isValidVideoExtension(name), "expected %q to be invalid", name)
	}
}