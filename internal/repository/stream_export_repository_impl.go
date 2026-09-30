package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"gorm.io/gorm"
)

type GormStreamExportRepository struct {
	db *gorm.DB
}

func NewGormStreamExportRepository(db *gorm.DB) *GormStreamExportRepository {
	return &GormStreamExportRepository{db: db}
}

func (r *GormStreamExportRepository) Create(ctx context.Context, export *models.StreamExport) error {
	if err := r.db.WithContext(ctx).Create(export).Error; err != nil {
		return fmt.Errorf("failed to create stream export: %w", err)
	}
	return nil
}

func (r *GormStreamExportRepository) ReadByStream(ctx context.Context, streamID uuid.UUID) (*models.StreamExport, error) {
	var export *models.StreamExport
	if err := r.db.WithContext(ctx).Where("stream_id = ?", streamID).First(&export).Error; err != nil {
		return nil, err
	}
	return export, nil
}

func (r *GormStreamExportRepository) Update(ctx context.Context, export *models.StreamExport) error {
	if export.ID == uuid.Nil {
		return fmt.Errorf("stream export ID can't be nil")
	}
	if err := r.db.WithContext(ctx).Save(export).Error; err != nil {
		return fmt.Errorf("failed to update stream export: %w", err)
	}
	return nil
}

// ResetFailed moves a failed export back to pending and reports whether this
// caller was the one that did it. The status is part of the WHERE clause, so
// two retries racing each other cannot both win and both queue a mux for the
// same stream.
func (r *GormStreamExportRepository) ResetFailed(ctx context.Context, exportID uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.StreamExport{}).
		Where("id = ? AND status = ?", exportID, models.ExportStatusFailed).
		Updates(map[string]any{
			"status": models.ExportStatusPending,
			"error":  "",
			"size":   0,
		})
	if res.Error != nil {
		return false, fmt.Errorf("failed to reset stream export: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// IsExportNotFound reports whether err came from a missing export row, so
// callers can map it to their own error without importing gorm.
func IsExportNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
