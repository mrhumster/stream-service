package request

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{
			name: "title to long",
			request: CreateStreamRequest{
				Title:      strings.Repeat("A", 257),
				Visibility: "public",
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

func TestCreateStreamRequest_ToServiceRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dtoReq := CreateStreamRequest{
			Title:       "title",
			Description: "description",
			Visibility:  models.VisibilityPrivate,
		}
		ownerUUID := uuid.New()
		svcReq, err := dtoReq.ToServiceRequest(ownerUUID)
		require.NoError(t, err)
		assert.NotNil(t, svcReq)
		assert.Equal(t, svcReq.OwnerID, ownerUUID)
	})

	t.Run("validate error", func(t *testing.T) {
		dtoReq := CreateStreamRequest{
			Title:       strings.Repeat("a", 257),
			Description: "description",
			Visibility:  models.VisibilityPrivate,
		}
		ownerUUID := uuid.New()
		svcReq, err := dtoReq.ToServiceRequest(ownerUUID)
		require.Error(t, err)
		assert.Nil(t, svcReq)
		assert.Contains(t, err.Error(), "validate error")
	})
	t.Run("owner uuid error", func(t *testing.T) {
		dtoReq := CreateStreamRequest{
			Title:       "title",
			Description: "description",
			Visibility:  models.VisibilityPrivate,
		}
		ownerUUID := uuid.Nil
		svcReq, err := dtoReq.ToServiceRequest(ownerUUID)
		require.Error(t, err)
		assert.Nil(t, svcReq)
		assert.Contains(t, err.Error(), "owner uuid can not be nil")
	})
}
