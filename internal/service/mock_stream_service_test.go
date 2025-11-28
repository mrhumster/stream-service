package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/service/mocks"
	"github.com/stretchr/testify/suite"
)

type MockStreamServiceTestSuite struct {
	suite.Suite
	service *mocks.MockStreamService
	ctx     context.Context
}

func TestMockStreamServiceTestSuite(t *testing.T) {
	suite.Run(t, new(MockStreamServiceTestSuite))
}

func (s *MockStreamServiceTestSuite) SetupTest() {
	s.service = &mocks.MockStreamService{}
	s.ctx = context.Background()
}

// TestCreateStream tests the CreateStream method
func (s *MockStreamServiceTestSuite) TestCreateStream() {
	userID := uuid.New()
	streamID := uuid.New()
	req := service.CreateStreamRequest{
		Title:       "Test Stream",
		Description: "Test Description",
		Visibility:  models.VisibilityPublic,
		Tags:        []string{"test", "go"},
		OwnerID:     userID,
	}
	tags, _ := json.Marshal(req.Tags)
	expectedStream := &models.Stream{
		Title:       req.Title,
		Description: req.Description,
		Visibility:  req.Visibility,
		Tags:        tags,
		OwnerID:     req.OwnerID,
		Status:      models.StatusDraft,
	}
	expectedStream.ID = streamID

	s.service.On("CreateStream", s.ctx, req).Return(expectedStream, nil)

	stream, err := s.service.CreateStream(s.ctx, req)

	s.NoError(err)
	s.Equal(expectedStream, stream)
	s.service.AssertCalled(s.T(), "CreateStream", s.ctx, req)

	errorReq := service.CreateStreamRequest{
		Title:   "",
		OwnerID: userID,
	}
	s.service.On("CreateStream", s.ctx, errorReq).Return(nil, errors.New("title is required"))

	stream, err = s.service.CreateStream(s.ctx, errorReq)

	s.Error(err)
	s.Nil(stream)
	s.Equal("title is required", err.Error())
}

func (s *MockStreamServiceTestSuite) TestGetStream() {
	streamID := uuid.New()
	expectedStream := &models.Stream{
		Title: "Test Stream",
	}
	expectedStream.ID = streamID
	s.service.On("GetStream", s.ctx, streamID).Return(expectedStream, nil)

	stream, err := s.service.GetStream(s.ctx, streamID)

	s.NoError(err)
	s.Equal(expectedStream, stream)

	notFoundID := uuid.New()
	s.service.On("GetStream", s.ctx, notFoundID).Return(nil, errors.New("stream not found"))

	stream, err = s.service.GetStream(s.ctx, notFoundID)

	s.Error(err)
	s.Nil(stream)
	s.service.AssertCalled(s.T(), "GetStream", s.ctx, notFoundID)
}

func (s *MockStreamServiceTestSuite) TestUpdateStream() {
	streamID := uuid.New()
	newTitle := "Updated Title"
	newVisibility := models.VisibilityPrivate

	req := service.UpdateStreamRequest{
		Title:      &newTitle,
		Visibility: &newVisibility,
	}

	expectedStream := &models.Stream{
		Title:      newTitle,
		Visibility: newVisibility,
	}

	expectedStream.ID = streamID
	s.service.On("UpdateStream", s.ctx, req).Return(expectedStream, nil)

	stream, err := s.service.UpdateStream(s.ctx, req)

	s.NoError(err)
	s.Equal(expectedStream, stream)

	partialReq := service.UpdateStreamRequest{
		Title: &newTitle,
	}
	partialStream := &models.Stream{
		Title: newTitle,
	}
	partialStream.ID = streamID
	s.service.On("UpdateStream", s.ctx, partialReq).Return(partialStream, nil)

	stream, err = s.service.UpdateStream(s.ctx, partialReq)
	s.NoError(err)
	s.Equal(partialStream, stream)
}

func (s *MockStreamServiceTestSuite) TestDeleteStream() {
	streamID := uuid.New()

	s.service.On("DeleteStream", s.ctx, streamID).Return(nil)

	err := s.service.DeleteStream(s.ctx, streamID)

	s.NoError(err)
	s.service.AssertCalled(s.T(), "DeleteStream", s.ctx, streamID)

	errorID := uuid.New()
	s.service.On("DeleteStream", s.ctx, errorID).Return(errors.New("delete failed"))

	err = s.service.DeleteStream(s.ctx, errorID)
	s.Error(err)
	s.Equal("delete failed", err.Error())
}

func (s *MockStreamServiceTestSuite) TestListStreams() {
	status := models.StatusPublished
	public := models.VisibilityPublic
	filter := repository.StreamFilter{
		Status:     &status,
		Visibility: &public,
	}

	expectedStreams := []*models.Stream{
		{Title: "Stream 1", Status: models.StatusPublished},
		{Title: "Stream 2", Status: models.StatusPublished},
	}

	s.service.On("ListStreams", s.ctx, filter).Return(expectedStreams, nil)

	streams, err := s.service.ListStreams(s.ctx, filter)

	s.NoError(err)
	s.Len(streams, 2)
	s.Equal(expectedStreams, streams)

	draft := models.StatusDraft
	emptyFilter := repository.StreamFilter{Status: &draft}
	s.service.On("ListStreams", s.ctx, emptyFilter).Return([]*models.Stream{}, nil)

	streams, err = s.service.ListStreams(s.ctx, emptyFilter)
	s.NoError(err)
	s.Empty(streams)
}

func (s *MockStreamServiceTestSuite) TestListUserStreams() {
	userID := uuid.New()

	expectedStreams := []*models.Stream{
		{Title: "User Stream 1", OwnerID: userID},
		{Title: "User Stream 2", OwnerID: userID},
	}

	s.service.On("ListUserStreams", s.ctx, userID).Return(expectedStreams, nil)

	streams, err := s.service.ListUserStreams(s.ctx, userID)

	s.NoError(err)
	s.Len(streams, 2)
	for _, stream := range streams {
		s.Equal(userID, stream.OwnerID)
	}

	emptyUserID := uuid.New()
	s.service.On("ListUserStreams", s.ctx, emptyUserID).Return([]*models.Stream{}, nil)

	streams, err = s.service.ListUserStreams(s.ctx, emptyUserID)
	s.NoError(err)
	s.Empty(streams)
}

func (s *MockStreamServiceTestSuite) TestPublishUnpublishStream() {
	streamID := uuid.New()

	s.service.On("PublishStream", s.ctx, streamID).Return(nil)

	err := s.service.PublishStream(s.ctx, streamID)
	s.NoError(err)
	s.service.AssertCalled(s.T(), "PublishStream", s.ctx, streamID)

	s.service.On("UnpublishStream", s.ctx, streamID).Return(nil)

	err = s.service.UnpublishStream(s.ctx, streamID)
	s.NoError(err)
	s.service.AssertCalled(s.T(), "UnpublishStream", s.ctx, streamID)

	errorID := uuid.New()
	s.service.On("PublishStream", s.ctx, errorID).Return(errors.New("publish failed"))

	err = s.service.PublishStream(s.ctx, errorID)
	s.Error(err)
	s.Equal("publish failed", err.Error())
}

func (s *MockStreamServiceTestSuite) TestUpdateStreamStatus() {
	streamID := uuid.New()

	testCases := []struct {
		name     string
		status   models.StreamStatus
		expected error
	}{
		{"Active", models.StatusReady, nil},
		{"Published", models.StatusPublished, nil},
		{"Error", models.StatusDraft, errors.New("status update failed")},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			s.service.On("UpdateStreamStatus", s.ctx, streamID, tc.status).Return(tc.expected)

			err := s.service.UpdateStreamStatus(s.ctx, streamID, tc.status)

			if tc.expected == nil {
				s.NoError(err)
			} else {
				s.Error(err)
				s.Equal(tc.expected.Error(), err.Error())
			}
			s.service.AssertCalled(s.T(), "UpdateStreamStatus", s.ctx, streamID, tc.status)
		})
	}
}

func (s *MockStreamServiceTestSuite) TestStartStreamUpload() {
	streamID := uuid.New()

	expectedUploadInfo := &service.UploadInfo{
		UploadURL: "https://storage.example.com/upload/" + streamID.String(),
		StreamID:  streamID,
	}

	s.service.On("StartStreamUpload", s.ctx, streamID).Return(expectedUploadInfo, nil)

	uploadInfo, err := s.service.StartStreamUpload(s.ctx, streamID)

	s.NoError(err)
	s.Equal(expectedUploadInfo, uploadInfo)
	s.Equal(streamID, uploadInfo.StreamID)
	s.NotEmpty(uploadInfo.UploadURL)

	errorID := uuid.New()
	s.service.On("StartStreamUpload", s.ctx, errorID).Return(nil, errors.New("upload service unavailable"))

	uploadInfo, err = s.service.StartStreamUpload(s.ctx, errorID)
	s.Error(err)
	s.Nil(uploadInfo)
	s.Equal("upload service unavailable", err.Error())
}

func (s *MockStreamServiceTestSuite) TestCompleteStreamUpload() {
	streamID := uuid.New()

	s.service.On("CompleteStreamUpload", s.ctx, streamID).Return(nil)

	err := s.service.CompleteStreamUpload(s.ctx, streamID)
	s.NoError(err)
	s.service.AssertCalled(s.T(), "CompleteStreamUpload", s.ctx, streamID)

	errorID := uuid.New()
	s.service.On("CompleteStreamUpload", s.ctx, errorID).Return(errors.New("upload not found"))

	err = s.service.CompleteStreamUpload(s.ctx, errorID)
	s.Error(err)
	s.Equal("upload not found", err.Error())
}

func (s *MockStreamServiceTestSuite) TestCanUserAccessStream() {
	userID := uuid.New()
	streamID := uuid.New()

	s.service.On("CanUserAccessStream", s.ctx, userID, streamID).Return(true, nil)

	canAccess, err := s.service.CanUserAccessStream(s.ctx, userID, streamID)
	s.NoError(err)
	s.True(canAccess)

	deniedStreamID := uuid.New()
	s.service.On("CanUserAccessStream", s.ctx, userID, deniedStreamID).Return(false, nil)

	canAccess, err = s.service.CanUserAccessStream(s.ctx, userID, deniedStreamID)
	s.NoError(err)
	s.False(canAccess)

	errorStreamID := uuid.New()
	s.service.On("CanUserAccessStream", s.ctx, userID, errorStreamID).Return(false, errors.New("access check failed"))

	canAccess, err = s.service.CanUserAccessStream(s.ctx, userID, errorStreamID)
	s.Error(err)
	s.False(canAccess)
	s.Equal("access check failed", err.Error())
}

func (s *MockStreamServiceTestSuite) TestMethodCallCount() {
	streamID := uuid.New()
	userID := uuid.New()
	stream := models.Stream{Title: "test title"}
	stream.ID = streamID
	s.service.On("GetStream", s.ctx, streamID).Return(&stream, nil)
	s.service.On("DeleteStream", s.ctx, streamID).Return(nil)
	s.service.On("ListUserStreams", s.ctx, userID).Return([]*models.Stream{}, nil)

	_, _ = s.service.GetStream(s.ctx, streamID)
	_, _ = s.service.GetStream(s.ctx, streamID)
	_ = s.service.DeleteStream(s.ctx, streamID)
	_, _ = s.service.ListUserStreams(s.ctx, userID)

	s.service.AssertNumberOfCalls(s.T(), "GetStream", 2)
	s.service.AssertNumberOfCalls(s.T(), "DeleteStream", 1)
	s.service.AssertNumberOfCalls(s.T(), "ListUserStreams", 1)
}

func (s *MockStreamServiceTestSuite) TearDownTest() {
	s.service.AssertExpectations(s.T())
}
