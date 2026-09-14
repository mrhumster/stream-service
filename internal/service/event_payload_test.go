package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	queuemock "github.com/mrhumster/stream-service/internal/queue/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEventPayload(t *testing.T) {
	stream := &models.Stream{
		Title:      "My Awesome Stream",
		Visibility: models.VisibilityPrivate,
	}

	t.Run("nil extra includes title and visibility", func(t *testing.T) {
		p := eventPayload(stream, nil)
		assert.Equal(t, "My Awesome Stream", p["title"])
		assert.Equal(t, models.VisibilityPrivate, p["visibility"])
		assert.Len(t, p, 2)
	})

	t.Run("extra fields merged with title", func(t *testing.T) {
		p := eventPayload(stream, map[string]any{"filename": "clip.mp4", "visibility": models.VisibilityPublic})
		assert.Equal(t, "My Awesome Stream", p["title"])
		assert.Equal(t, "clip.mp4", p["filename"])
		assert.Equal(t, models.VisibilityPrivate, p["visibility"], "stream visibility wins")
	})

	t.Run("extra title does not override stream title", func(t *testing.T) {
		p := eventPayload(stream, map[string]any{"title": "spoofed"})
		assert.Equal(t, "My Awesome Stream", p["title"])
	})

	t.Run("non-map extra logs warning but keeps base", func(t *testing.T) {
		p := eventPayload(stream, "not-a-map")
		assert.Equal(t, "My Awesome Stream", p["title"])
		assert.Len(t, p, 2)
	})
}

func TestRecordEventIncludesTitle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	recorder := queuemock.NewMockActivityEventRecorder(ctrl)
	svc := &StreamServiceImpl{eventsRecorder: recorder}

	streamID := uuid.New()
	ownerID := uuid.New()
	stream := &models.Stream{
		BaseModel:  models.BaseModel{ID: streamID},
		Title:      "Injected Title",
		Visibility: models.VisibilityUnlisted,
		OwnerID:    ownerID,
	}

	recorder.EXPECT().
		RecordActivityEvent(gomock.Any(), ownerID, "stream.ready", &streamID, gomock.Any()).
		Do(func(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID, payload any) {
			p, ok := payload.(map[string]any)
			require.True(t, ok, "payload must be a map")
			assert.Equal(t, "Injected Title", p["title"])
			assert.Equal(t, models.VisibilityUnlisted, p["visibility"])
		}).
		Return(nil)

	svc.recordEvent(context.Background(), stream, "stream.ready", nil)
}