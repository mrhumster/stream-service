package request

import (
	"strings"
	"testing"

	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string {
	return &s
}

func TestUpdateStreamRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request UpdateStreamRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: UpdateStreamRequest{
				Title:      strPtr("My Stream"),
				Visibility: (*models.StreamVisibility)(strPtr("public")),
			},
			wantErr: false,
		},
		{
			name: "empty title",
			request: UpdateStreamRequest{
				Title:      strPtr(""),
				Visibility: (*models.StreamVisibility)(strPtr("public")),
			},
			wantErr: true,
		},
		{
			name: "invalid Visibility",
			request: UpdateStreamRequest{
				Title:      strPtr("My Stream"),
				Visibility: (*models.StreamVisibility)(strPtr("invalid")),
			},
			wantErr: true,
		},
		{
			name: "title to long",
			request: UpdateStreamRequest{
				Title: strPtr(strings.Repeat("a", 257)),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateStreamRequest_ToServiceRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		updReq := UpdateStreamRequest{
			Title: strPtr("title"),
		}
		svcReq, err := updReq.ToServiceRequest()
		require.NoError(t, err)
		assert.NotNil(t, svcReq)
	})

	t.Run("validate error", func(t *testing.T) {
		updReq := UpdateStreamRequest{
			Title: strPtr(""),
		}
		svcReq, err := updReq.ToServiceRequest()
		require.Error(t, err)
		assert.Nil(t, svcReq)
	})
}
