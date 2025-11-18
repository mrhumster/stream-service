package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/stretchr/testify/mock"
)

type StreamRepositoryMock struct {
	mock.Mock
}

func (m *StreamRepositoryMock) Create(ctx context.Context, stream *models.Stream) error {
	args := m.Called(ctx, stream)
	return args.Error(0)
}

func (m *StreamRepositoryMock) GetByID(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Stream), args.Error(1)
}

func (m *StreamRepositoryMock) List(ctx context.Context, filter repository.StreamFilter) ([]*models.Stream, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Stream), args.Error(1)
}
