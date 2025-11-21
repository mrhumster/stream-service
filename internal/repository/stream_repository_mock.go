package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/stretchr/testify/mock"
)

type StreamRepositoryMock struct {
	mock.Mock
}

func (m *StreamRepositoryMock) Create(ctx context.Context, stream *models.Stream) error {
	args := m.Called(ctx, stream)
	return args.Error(0)
}

func (m *StreamRepositoryMock) Read(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*models.Stream), args.Error(1)
}

func (m *StreamRepositoryMock) GetByOwner(ctx context.Context, id uuid.UUID) ([]*models.Stream, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Stream), args.Error(1)
}

func (m *StreamRepositoryMock) List(ctx context.Context, filter StreamFilter) ([]*models.Stream, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Stream), args.Error(1)
}

func (m *StreamRepositoryMock) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *StreamRepositoryMock) Exists(ctx context.Context, id uuid.UUID) bool {
	args := m.Called(ctx, id)
	return args.Bool(0)
}

func (m *StreamRepositoryMock) IncrementViews(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *StreamRepositoryMock) UpdateStatus(ctx context.Context, id uuid.UUID, status models.StreamStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *StreamRepositoryMock) UpdateProcessing(ctx context.Context, id uuid.UUID, processing models.StreamProcessing) error {
	args := m.Called(ctx, id, processing)
	return args.Error(0)
}

func (m *StreamRepositoryMock) Update(ctx context.Context, stream *models.Stream) error {
	args := m.Called(ctx, stream)
	return args.Error(0)
}
