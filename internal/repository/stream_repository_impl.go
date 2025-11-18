package repository

import (
	"context"
	"fmt"
	"log"

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
	// TODO: Под реализацию
	return fmt.Errorf("not implemented")
}

func (r *GormStreamRepository) Read(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	// TODO: Под реализацию
	return nil, fmt.Errorf("not implemented")
}
func (r *GormStreamRepository) Update(ctx context.Context, stream *models.Stream) error {
	// TODO: Под реализацию
	return fmt.Errorf("not implemented")
}
func (r *GormStreamRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// TODO: Под реализацию
	return fmt.Errorf("not implemented")
}

func (r *GormStreamRepository) List(ctx context.Context, filter StreamFilter) ([]*models.Stream, error) {

	// TODO: Под реализацию
	return nil, fmt.Errorf("not implemented")
}

func (r *GormStreamRepository) GetByOwner(ctx context.Context, ownerID uuid.UUID) ([]*models.Stream, error) {
	// TODO: Под реализацию
	return nil, fmt.Errorf("not implemented")
}

func (r *GormStreamRepository) Exists(ctx context.Context, id uuid.UUID) bool {
	// TODO: Под реализацию
	log.Printf("not implemented")
	return false
}

func (r *GormStreamRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.StreamStatus) error {
	// TODO: Под реализацию
	return fmt.Errorf("not implemented")
}

func (r *GormStreamRepository) UpdateProcessing(ctx context.Context, id uuid.UUID, processing models.StreamProcessing) error {
	// TODO: Под реализацию
	return fmt.Errorf("not implemented")
}

func (r *GormStreamRepository) IncrementViews(ctx context.Context, id uuid.UUID) error {
	// TODO: Под реализацию
	return fmt.Errorf("not implemented")
}
