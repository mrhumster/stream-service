package request

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartUploadRequest_ToService(t *testing.T) {
	streamID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name        string
		filename    string
		contentType string
		wantErr     bool
	}{
		{name: "valid mp4", filename: "video.mp4", contentType: "video/mp4", wantErr: false},
		{name: "valid 3gp", filename: "clip.3gp", contentType: "video/3gpp", wantErr: false},
		{name: "valid 3g2", filename: "clip.3g2", contentType: "video/3gpp2", wantErr: false},
		{name: "valid mkv", filename: "clip.mkv", contentType: "video/x-matroska", wantErr: false},
		{name: "invalid content type", filename: "video.mp4", contentType: "text/plain", wantErr: true},
		{name: "invalid extension", filename: "video.exe", contentType: "video/mp4", wantErr: true},
		{name: "invalid content type and extension", filename: "video.exe", contentType: "text/plain", wantErr: true},
		{name: "3gp extension with mp4 content type", filename: "clip.3gp", contentType: "video/mp4", wantErr: false},
		{name: "bad 3gp extension", filename: "clip.3gpx", contentType: "video/3gpp", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &StartUploadRequest{
				FileName:    tt.filename,
				TotalSize:   100,
				ContentType: tt.contentType,
			}

			res, err := s.ToService(streamID, userID)
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
			assert.Equal(t, tt.filename, res.Filename)
			assert.Equal(t, int64(100), res.TotalSize)
		})
	}
}