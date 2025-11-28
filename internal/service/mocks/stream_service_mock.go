package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/stretchr/testify/mock"
)

type MockStreamService struct {
	mock.Mock
}

func (m *MockStreamService) CreateStream(ctx context.Context, req service.CreateStreamRequest) (*models.Stream, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Stream), args.Error(1)
}

func (m *MockStreamService) GetStream(ctx context.Context, id uuid.UUID) (*models.Stream, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Stream), args.Error(1)
}

func (m *MockStreamService) UpdateStream(ctx context.Context, req service.UpdateStreamRequest) (*models.Stream, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Stream), args.Error(1)
}

func (m *MockStreamService) DeleteStream(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStreamService) ListStreams(ctx context.Context, filter repository.StreamFilter) ([]*models.Stream, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Stream), args.Error(1)
}

func (m *MockStreamService) ListUserStreams(ctx context.Context, userID uuid.UUID) ([]*models.Stream, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Stream), args.Error(1)
}

func (m *MockStreamService) PublishStream(ctx context.Context, streamID uuid.UUID) error {
	args := m.Called(ctx, streamID)
	return args.Error(0)
}

func (m *MockStreamService) UnpublishStream(ctx context.Context, streamID uuid.UUID) error {
	args := m.Called(ctx, streamID)
	return args.Error(0)
}

func (m *MockStreamService) UpdateStreamStatus(ctx context.Context, streamID uuid.UUID, status models.StreamStatus) error {
	args := m.Called(ctx, streamID, status)
	return args.Error(0)
}

func (m *MockStreamService) StartStreamUpload(ctx context.Context, streamID uuid.UUID) (*service.UploadInfo, error) {
	args := m.Called(ctx, streamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.UploadInfo), args.Error(1)
}

func (m *MockStreamService) CompleteStreamUpload(ctx context.Context, streamID uuid.UUID) error {
	args := m.Called(ctx, streamID)
	return args.Error(0)
}

func (m *MockStreamService) CanUserAccessStream(ctx context.Context, userID uuid.UUID, streamID uuid.UUID) (bool, error) {
	args := m.Called(ctx, userID, streamID)
	return args.Bool(0), args.Error(1)
}
