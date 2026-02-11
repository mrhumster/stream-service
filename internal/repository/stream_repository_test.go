package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStreamRepositoryInterface(t *testing.T) {
	var _ repository.StreamRepository = (*repository.GormStreamRepository)(nil)
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.Stream{})
	require.NoError(t, err)
	return db
}

func TestGormStreamRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()

	stream := &models.Stream{
		Title:       "Test Stream",
		Description: "Test Description",
		OwnerID:     uuid.New(),
		Status:      models.StatusDraft,
		Visibility:  models.VisibilityPrivate,
	}

	err := repo.Create(ctx, stream)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, stream.ID)

	var count int64
	db.Model(&models.Stream{}).Count(&count)
	assert.Equal(t, count, int64(1))
}

func TestGormStreamRepository_Read(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()

	stream := &models.Stream{
		Title:       "Test Stream",
		Description: "Test Description",
		OwnerID:     uuid.New(),
		Status:      models.StatusDraft,
		Visibility:  models.VisibilityPrivate,
	}

	err := repo.Create(ctx, stream)
	require.NoError(t, err)

	err = repo.Delete(ctx, stream.ID)
	require.NoError(t, err)

	var count int64
	db.Model(&models.Stream{}).Count(&count)
	assert.Equal(t, count, int64(0))

	deleted, err := repo.Read(ctx, stream.ID)
	require.Nil(t, deleted)
}

func TestGormStreamRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()

	streamID := uuid.New()
	stream := &models.Stream{
		Title:       "Test Stream",
		Description: "Test Description",
		OwnerID:     uuid.New(),
		Status:      models.StatusDraft,
		Visibility:  models.VisibilityPrivate,
	}
	stream.ID = streamID

	repo.Create(ctx, stream)

	stream.Title = "Updated title"
	err := repo.Update(ctx, stream)
	require.NoError(t, err)

	updated, err := repo.Read(ctx, streamID)
	require.NotNil(t, updated)
	require.Equal(t, stream.Title, updated.Title)
}

func TestGormStreamRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()
	streamID := uuid.New()

	stream := &models.Stream{
		Title:   "For delete",
		OwnerID: uuid.New(),
	}
	stream.ID = streamID

	repo.Create(ctx, stream)

	err := repo.Delete(ctx, streamID)
	require.NoError(t, err)

	var count int64
	db.Model(&models.Stream{}).Count(&count)
	assert.Equal(t, count, int64(0))
}

func TestGormStreamRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()
	ownerID := uuid.New()
	stream1 := &models.Stream{
		Title:   "stream 1",
		OwnerID: ownerID,
	}
	repo.Create(ctx, stream1)
	stream2 := &models.Stream{
		Title:   "stream 2",
		OwnerID: ownerID,
	}
	repo.Create(ctx, stream2)

	filter := repository.StreamFilter{
		OwnerID: &ownerID,
	}
	streams, err := repo.List(ctx, filter)
	require.NoError(t, err)
	require.Len(t, streams, 2)
}

func TestGormStreamRepository_ListWithLimit(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()
	filter := repository.StreamFilter{
		Limit:  2,
		Offset: 0,
	}

	for i := range 10 {
		s := &models.Stream{
			Title:      fmt.Sprintf("Strema %d", i),
			OwnerID:    uuid.New(),
			Visibility: models.VisibilityPublic,
		}

		err := repo.Create(ctx, s)
		require.NoError(t, err)
	}

	streams, err := repo.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, streams, 2)
}

func TestGormStreamRepository_GetByOwner(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()
	ownerID := uuid.New()
	stream1 := &models.Stream{
		Title:   "stream 1",
		OwnerID: ownerID,
		Status:  models.StatusDraft,
	}
	repo.Create(ctx, stream1)
	stream2 := &models.Stream{
		Title:   "stream 2",
		OwnerID: ownerID,
		Status:  models.StatusDraft,
	}
	repo.Create(ctx, stream2)

	streams, err := repo.GetByOwner(ctx, ownerID)
	require.NoError(t, err)
	require.Len(t, streams, 2)
}

func TestGormStreamRepository_Exists(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()
	ownerID := uuid.New()
	streamID := uuid.New()
	stream := &models.Stream{
		Title:   "stream",
		OwnerID: ownerID,
		Status:  models.StatusDraft,
	}
	stream.ID = streamID
	repo.Create(ctx, stream)
	exists := repo.Exists(ctx, streamID)
	require.True(t, exists)
}

func TestGormStreamRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()
	ownerID := uuid.New()
	streamID := uuid.New()
	stream := &models.Stream{
		Title:   "stream",
		OwnerID: ownerID,
		Status:  models.StatusDraft,
	}
	stream.ID = streamID
	repo.Create(ctx, stream)
	err := repo.UpdateStatus(ctx, streamID, models.StatusPublished)
	require.NoError(t, err)

	updated, err := repo.Read(ctx, stream.ID)
	assert.Equal(t, updated.Status, models.StatusPublished)
}

func TestGormStreamRepository_UpdateProcessing(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewGormStreamRepository(db)
	ctx := context.Background()
	ownerID := uuid.New()
	streamID := uuid.New()
	stream := &models.Stream{
		Title:   "stream",
		OwnerID: ownerID,
		Status:  models.StatusDraft,
	}
	stream.ID = streamID
	repo.Create(ctx, stream)
	processing := models.StreamProcessing{
		Steps:    []string{"step one", "step two"},
		Progress: 50,
		Error:    nil,
	}
	err := repo.UpdateProcessing(ctx, stream.ID, processing)
	require.NoError(t, err)

	updated, err := repo.Read(ctx, stream.ID)
	require.NoError(t, err)
	var updProcessing *models.StreamProcessing
	if err := json.Unmarshal(updated.Processing, &updProcessing); err != nil {
		t.Errorf("error unmarshal precessing: %v", err)
	}
	require.Equal(t, processing, *updProcessing)
}
