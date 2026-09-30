package response

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExportResponse(t *testing.T) {
	streamID := uuid.New()

	t.Run("carries requested so the UI can tell idle from building", func(t *testing.T) {
		got := NewExportResponse(&service.StreamExportInfo{
			StreamID: streamID, Status: models.ExportStatusPending,
		})

		assert.Equal(t, streamID.String(), got.StreamID)
		assert.Equal(t, models.ExportStatusPending, got.Status)
		assert.False(t, got.Requested)
		// omitempty keeps a zero size out of the payload, but requested is a
		// plain bool: false is the answer, not the absence of one.
		assert.Equal(t, "", got.Error)
	})

	t.Run("ready export reports its size", func(t *testing.T) {
		got := NewExportResponse(&service.StreamExportInfo{
			StreamID:  streamID,
			Status:    models.ExportStatusReady,
			Size:      2048,
			Requested: true,
		})

		assert.Equal(t, models.ExportStatusReady, got.Status)
		assert.Equal(t, int64(2048), got.Size)
		assert.True(t, got.Requested)
	})

	t.Run("failed export carries the reason", func(t *testing.T) {
		got := NewExportResponse(&service.StreamExportInfo{
			StreamID:  streamID,
			Status:    models.ExportStatusFailed,
			Error:     "ffmpeg mux failed: no streams",
			Requested: true,
		})

		assert.Equal(t, models.ExportStatusFailed, got.Status)
		assert.Equal(t, "ffmpeg mux failed: no streams", got.Error)
	})

	t.Run("carries the name the client suggests to the save dialog", func(t *testing.T) {
		got := NewExportResponse(&service.StreamExportInfo{
			StreamID:  streamID,
			Status:    models.ExportStatusReady,
			Requested: true,
			FileName:  "My vacation.mp4",
		})

		assert.Equal(t, "My vacation.mp4", got.FileName)
		// It has to survive the trip to the browser, or the client is back to
		// inventing a name of its own.
		encoded, err := json.Marshal(got)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"file_name":"My vacation.mp4"`)
	})
}
