package request

import (
	"testing"

	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/stretchr/testify/assert"
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
