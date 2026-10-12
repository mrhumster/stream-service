package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	authmock "github.com/mrhumster/identity-service/pkg/auth/mock"
	"github.com/mrhumster/stream-service/internal/domain/models"
	queuemock "github.com/mrhumster/stream-service/internal/queue/mock"
	repomock "github.com/mrhumster/stream-service/internal/repository/mock"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/storage/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

func newDuplicatesService(t *testing.T, ctrl *gomock.Controller) (*service.StreamServiceImpl, *repomock.MockStreamRepository) {
	t.Helper()
	mockRepo := repomock.NewMockStreamRepository(ctrl)
	svc := service.NewStreamServiceImpl(
		mockRepo,
		authmock.NewMockPermissionClient(ctrl),
		mock.NewMockFileStorage(ctrl),
		queuemock.NewMockTaskDistributor(ctrl),
		nil,
		srvCfg(),
	)
	return svc, mockRepo
}

func dupStream(title string, status models.StreamStatus, meta string) *models.Stream {
	s := &models.Stream{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Title:     title,
		Status:    status,
	}
	if meta != "" {
		s.Metadata = datatypes.JSON([]byte(meta))
	}
	return s
}

func TestFindDuplicateStreams_GroupsAndSplits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, mockRepo := newDuplicatesService(t, ctrl)
	owner := uuid.New()

	// The repository returns rows already ordered by title/key and newest first.
	// "clip" carries metadata, so it forms one group; "other" has no metadata on
	// either row, which the both-missing rule treats as the same key.
	processing := dupStream("clip.mp4", models.StatusProcessing, `{"duration":10,"size":100}`)
	ready := dupStream("clip.mp4", models.StatusReady, `{"duration":10,"size":100}`)
	published := dupStream("clip.mp4", models.StatusPublished, `{"duration":10,"size":100}`)
	otherReady := dupStream("other.mov", models.StatusReady, "")
	otherDraft := dupStream("other.mov", models.StatusDraft, "")

	mockRepo.EXPECT().
		ListDuplicateStreams(gomock.Any(), owner).
		Return([]*models.Stream{processing, ready, published, otherReady, otherDraft}, nil)

	groups, err := svc.FindDuplicateStreams(context.Background(), owner)
	require.NoError(t, err)
	require.Len(t, groups, 2)

	clip := groups[0]
	assert.Equal(t, "clip.mp4", clip.Title)
	// keep = newest usable, i.e. the ready one (newer than the published one).
	assert.Equal(t, ready.ID, clip.Keep.ID)
	require.Len(t, clip.Deletable, 1)
	assert.Equal(t, processing.ID, clip.Deletable[0].ID)
	require.Len(t, clip.Protected, 1)
	assert.Equal(t, published.ID, clip.Protected[0].ID)

	other := groups[1]
	assert.Equal(t, "other.mov", other.Title)
	assert.Equal(t, otherReady.ID, other.Keep.ID)
	require.Len(t, other.Deletable, 1)
	assert.Equal(t, otherDraft.ID, other.Deletable[0].ID)
	assert.Empty(t, other.Protected)
}

func TestFindDuplicateStreams_KeepFallsBackToNewestWhenNoneUsable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, mockRepo := newDuplicatesService(t, ctrl)
	owner := uuid.New()

	newest := dupStream("x", models.StatusDraft, "")
	older := dupStream("x", models.StatusError, "")

	mockRepo.EXPECT().
		ListDuplicateStreams(gomock.Any(), owner).
		Return([]*models.Stream{newest, older}, nil)

	groups, err := svc.FindDuplicateStreams(context.Background(), owner)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, newest.ID, groups[0].Keep.ID)
	require.Len(t, groups[0].Deletable, 1)
	assert.Equal(t, older.ID, groups[0].Deletable[0].ID)
}

func TestFindDuplicateStreams_SeparatesDifferentMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, mockRepo := newDuplicatesService(t, ctrl)
	owner := uuid.New()

	// Same title, different size -> different keys -> two distinct groups.
	a := dupStream("same", models.StatusReady, `{"duration":10,"size":100}`)
	b := dupStream("same", models.StatusReady, `{"duration":10,"size":200}`)

	mockRepo.EXPECT().
		ListDuplicateStreams(gomock.Any(), owner).
		Return([]*models.Stream{a, b}, nil)

	groups, err := svc.FindDuplicateStreams(context.Background(), owner)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, a.ID, groups[0].Keep.ID)
	assert.Equal(t, b.ID, groups[1].Keep.ID)
}

func TestFindDuplicateStreams_RepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc, mockRepo := newDuplicatesService(t, ctrl)
	owner := uuid.New()

	mockRepo.EXPECT().
		ListDuplicateStreams(gomock.Any(), owner).
		Return(nil, errors.New("boom"))

	_, err := svc.FindDuplicateStreams(context.Background(), owner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
