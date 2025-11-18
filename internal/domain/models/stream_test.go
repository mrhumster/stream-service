package models

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestStream_Validate(t *testing.T) {
	validOwnerID := uuid.New()

	tests := []struct {
		name        string
		setupStream func() Stream
		wantErr     bool
		errorType   error
	}{
		{
			name: "Valid stream",
			setupStream: func() Stream {
				return Stream{
					Title:       "My Aswesome Stream",
					Description: "This is a test stream",
					OwnerID:     validOwnerID,
					Status:      StatusDraft,
					Visibility:  VisibilityPrivate,
				}
			},
			wantErr: false,
		},
		{
			name: "empty title should fail",
			setupStream: func() Stream {
				return Stream{
					Title:      "",
					OwnerID:    validOwnerID,
					Status:     StatusDraft,
					Visibility: VisibilityPrivate,
				}
			},
			wantErr: true,
		},
		{
			name: "title too long should fail",
			setupStream: func() Stream {
				longTitle := ""
				for i := 0; i < 256; i++ {
					longTitle += "a"
				}
				return Stream{
					Title:      longTitle,
					OwnerID:    validOwnerID,
					Status:     StatusDraft,
					Visibility: VisibilityPrivate,
				}
			},
			wantErr:   true,
			errorType: ErrStreamTitleIsTooLong,
		},
		{
			name: "empty owner ID should fail",
			setupStream: func() Stream {
				return Stream{
					Title:      "Test Stream",
					OwnerID:    uuid.Nil,
					Status:     StatusDraft,
					Visibility: VisibilityPrivate,
				}
			},
			wantErr:   true,
			errorType: ErrOwnerIDRequired,
		},
		{
			name: "invalid status should fail",
			setupStream: func() Stream {
				return Stream{
					Title:      "Test Stream",
					OwnerID:    validOwnerID,
					Status:     "invalid_status",
					Visibility: VisibilityPrivate,
				}
			},
			wantErr:   true,
			errorType: ErrInvalidStatus,
		},
		{
			name: "invalid visibility should fail",
			setupStream: func() Stream {
				return Stream{
					Title:      "Test Stream",
					OwnerID:    validOwnerID,
					Status:     StatusDraft,
					Visibility: "invalid_visibility",
				}
			},
			wantErr:   true,
			errorType: ErrInvalidVisibility,
		},
		{
			name: "all valid statuses should pass",
			setupStream: func() Stream {
				return Stream{
					Title:      "Test Stream",
					OwnerID:    validOwnerID,
					Status:     StatusReady,
					Visibility: VisibilityPublic,
				}
			},
			wantErr: false,
		},
		{
			name: "all valid visibilities should pass",
			setupStream: func() Stream {
				return Stream{
					Title:      "Test Stream",
					OwnerID:    validOwnerID,
					Status:     StatusDraft,
					Visibility: VisibilityUnlisted,
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := tt.setupStream()

			err := stream.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStream_StatusMethods(t *testing.T) {
	t.Run("check status methods", func(t *testing.T) {
		stream := Stream{Status: StatusDraft}

		assert.True(t, stream.IsDraft())
		assert.False(t, stream.IsPublished())
		assert.True(t, stream.CanEdit())
	})

	t.Run("check ownership", func(t *testing.T) {
		ownerID := uuid.New()
		otherID := uuid.New()

		stream := Stream{OwnerID: ownerID}

		assert.True(t, stream.IsOwnedBy(ownerID))
		assert.False(t, stream.IsOwnedBy(otherID))
	})
}
