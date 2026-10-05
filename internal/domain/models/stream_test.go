package models

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestStream_SetMetadata(t *testing.T) {
	t.Run("setting", func(t *testing.T) {
		stream := Stream{Title: "Meta set test"}
		meta := StreamMetadata{Size: 1}
		err := stream.SetMetadata(&meta)
		assert.NoError(t, err)
		metaOut, err := stream.GetMetadata()
		assert.NoError(t, err)
		require.Equal(t, &meta, metaOut)
	})
}

// Steps has no omitempty, so a nil slice is served as "steps": null and breaks
// clients reading task.steps.length. Both writers must keep it an array: the
// proto3 handler path (absent repeated field decodes to nil) and the reprocess
// path (steps reset before the worker reports).
func TestStream_TaskStepsAlwaysSerializeAsArray(t *testing.T) {
	t.Run("SetTaskProgress with nil steps", func(t *testing.T) {
		stream := Stream{Title: "nil steps"}
		require.NoError(t, stream.SetInitialTasks([]StreamProcessingTask{{
			TaskType: TaskTypeTranscode,
		}}))

		require.NoError(t, stream.SetTaskProgress(TaskTypeTranscode, 100, nil, nil, nil))

		assert.NotContains(t, string(stream.Processing), `"steps":null`)
		tasks, err := stream.ProcessingTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.NotNil(t, tasks[0].Steps)
		assert.Empty(t, tasks[0].Steps)
	})

	t.Run("SetTaskProgress appends a task with nil steps", func(t *testing.T) {
		stream := Stream{Title: "new task nil steps"}
		require.NoError(t, stream.SetInitialTasks([]StreamProcessingTask{}))

		require.NoError(t, stream.SetTaskProgress(TaskTypeFaces, 0, nil, nil, nil))

		assert.NotContains(t, string(stream.Processing), `"steps":null`)
	})

	t.Run("SetTaskProgress keeps real steps", func(t *testing.T) {
		stream := Stream{Title: "real steps"}
		require.NoError(t, stream.SetInitialTasks([]StreamProcessingTask{{
			TaskType: TaskTypeTranscode,
			Steps:    []string{"Transcoding"},
		}}))

		require.NoError(t, stream.SetTaskProgress(TaskTypeTranscode, 50, []string{"Transcoding", "Uploading"}, nil, nil))

		tasks, err := stream.ProcessingTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.Equal(t, []string{"Transcoding", "Uploading"}, tasks[0].Steps)
	})

	t.Run("SetInitialTasks with nil steps", func(t *testing.T) {
		stream := Stream{Title: "initial nil steps"}
		require.NoError(t, stream.SetInitialTasks([]StreamProcessingTask{
			{TaskType: TaskTypeTranscode},
			{TaskType: TaskTypeThumbnail, Steps: []string{"Generating thumbnail"}},
		}))

		assert.NotContains(t, string(stream.Processing), `"steps":null`)
		tasks, err := stream.ProcessingTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 2)
		assert.NotNil(t, tasks[0].Steps)
		assert.Empty(t, tasks[0].Steps)
		assert.Equal(t, []string{"Generating thumbnail"}, tasks[1].Steps)
	})
}
