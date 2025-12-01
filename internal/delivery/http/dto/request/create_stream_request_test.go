package request

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateStreamRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request CreateStreamRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: CreateStreamRequest{
				Title:      "My Stream",
				Visibility: "public",
			},
			wantErr: false,
		},
		{
			name: "empty title",
			request: CreateStreamRequest{
				Title:      "",
				Visibility: "public",
			},
			wantErr: true,
		},
		{
			name: "invalid Visibility",
			request: CreateStreamRequest{
				Title:      "My Stream",
				Visibility: "invalid",
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
