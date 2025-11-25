package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"gorm.io/gorm"
)

type GormStreamRepository struct {
	db *gorm.DB
}

func NewGormStreamRepository(db *gorm.DB) *GormStreamRepository {
	return &GormStreamRepository{db: db}
}

func (r *GormStreamRepository) Create(ctx context.Context, stream *models.Stream) error {
	result := r.db.WithContext(ctx).Create(&stream)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (r *GormStreamRepository) Read(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	var stream *models.Stream
	result := r.db.WithContext(ctx).First(&stream, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return stream, nil
}
func (r *GormStreamRepository) Update(ctx context.Context, stream *models.Stream) error {
	if stream.ID == uuid.Nil {
		return fmt.Errorf("stream ID can not be nil")
	}
	var existing *models.Stream
	if err := r.db.WithContext(ctx).First(&existing, "id = ?", stream.ID).Error; err != nil {
		return fmt.Errorf("stream not found: %w", err)
	}
	result := r.db.WithContext(ctx).Save(stream)
	if result.Error != nil {
		return fmt.Errorf("failed to update stream: %w", result.Error)
	}
	return nil
}
func (r *GormStreamRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var existing *models.Stream
	if err := r.db.WithContext(ctx).First(&existing, "id = ?", id).Error; err != nil {
		return fmt.Errorf("stream not found: %w", err)
	}
	if err := r.db.WithContext(ctx).Delete(&models.Stream{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete stream: %w", err)
	}
	return nil
}

func (r *GormStreamRepository) List(ctx context.Context, filter StreamFilter) ([]*models.Stream, error) {
	query := r.db.WithContext(ctx).Model(&models.Stream{})

	if filter.OwnerID != nil {
		query = query.Where("owner_id = ?", filter.OwnerID)
	}

	if filter.Status != nil {
		query = query.Where("status = ?", filter.Status)
	}

	if filter.Visibility != nil {
		query = query.Where("visibility = ?", filter.Visibility)
	}

	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		query = query.Where("title ILIKE ? or description ILIKE ?", searchPattern, searchPattern)
	}

	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	query = query.Order("created_at DESC")

	var streams []*models.Stream
	result := query.Find(&streams)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list stream: %w", result.Error)
	}
	return streams, nil
}

func (r *GormStreamRepository) GetByOwner(ctx context.Context, ownerID uuid.UUID) ([]*models.Stream, error) {
	query := r.db.WithContext(ctx).Model(&models.Stream{})
	query = query.Where("owner_id = ?", ownerID)
	var streams []*models.Stream
	result := query.Find(&streams)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list stream: %w", result.Error)
	}
	return streams, nil
}

func (r *GormStreamRepository) Exists(ctx context.Context, id uuid.UUID) bool {
	var stream *models.Stream
	result := r.db.WithContext(ctx).First(&stream, id)
	if result.Error != nil {
		return false
	}
	return true
}

func (r *GormStreamRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.StreamStatus) error {
	var stream *models.Stream
	result := r.db.WithContext(ctx).First(&stream, id)
	if result.Error != nil {
		return fmt.Errorf("stream id not found")
	}
	stream.Status = status
	result = r.db.WithContext(ctx).Save(stream)
	if result.Error != nil {
		return fmt.Errorf("update stream error: %w", result.Error)
	}
	return nil
}

func (r *GormStreamRepository) UpdateProcessing(ctx context.Context, id uuid.UUID, processing models.StreamProcessing) error {
	var stream *models.Stream
	result := r.db.WithContext(ctx).First(&stream, id)
	if result.Error != nil {
		return fmt.Errorf("stream id not found")
	}
	if err := stream.UpdateProcessing(processing.Progress, processing.Steps, processing.Error); err != nil {
		return fmt.Errorf("update stream processing error: %w", err)
	}
	result = r.db.WithContext(ctx).Save(stream)
	if result.Error != nil {
		return fmt.Errorf("update stream precessing error: %w", result.Error)
	}
	return nil
}

func (r *GormStreamRepository) IncrementViews(ctx context.Context, id uuid.UUID) error {
	var stream *models.Stream
	if err := r.db.WithContext(ctx).First(&stream, id).Error; err != nil {
		return fmt.Errorf("stream id not found")
	}
	analitics, err := stream.GetAnalitics()
	if err != nil {
		return fmt.Errorf("increment views error: %w", err)
	}
	analitics.Views += 1
	if err := stream.SetAnalitics(analitics); err != nil {
		return fmt.Errorf("increment views errror: %w", err)
	}
	if err := r.db.WithContext(ctx).Save(stream).Error; err != nil {
		return fmt.Errorf("increment views errror: %w", err)
	}
	return nil
}
