// service/contract_implementation_test.go
package service_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/service/mocks"
	"github.com/stretchr/testify/mock"
)

func TestMockStreamService_Contract(t *testing.T) {
	mockService := &mocks.MockStreamService{}

	// Настройка мока для контрактных тестов
	setupMockForContractTests(mockService)

	contractTest := NewStreamServiceContractTest(t, mockService)
	contractTest.TestAll()
}

// setupMockForContractTests настраивает мок для прохождения контрактных тестов
func setupMockForContractTests(mockService *mocks.MockStreamService) {
	testStreamID := uuid.New()
	testUserID := uuid.New()

	// Настраиваем ожидания для всех методов с правильными структурами
	mockService.On("CreateStream", mock.Anything, mock.AnythingOfType("service.CreateStreamRequest")).
		Return(createTestStreamData(testStreamID, "Test Stream", testUserID), nil)

	mockService.On("GetStream", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(createTestStreamData(testStreamID, "Test Stream", testUserID), nil)

	mockService.On("UpdateStream", mock.Anything, mock.AnythingOfType("service.UpdateStreamRequest")).
		Return(createTestStreamData(testStreamID, "Updated Stream", testUserID), nil)

	mockService.On("DeleteStream", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(nil)

	mockService.On("PublishStream", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(nil)

	mockService.On("UnpublishStream", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(nil)

	mockService.On("UpdateStreamStatus", mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("models.StreamStatus")).
		Return(nil)

	mockService.On("StartStreamUpload", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(&service.UploadInfo{
			UploadURL: "https://storage.example.com/upload/" + testStreamID.String(),
			StreamID:  testStreamID,
		}, nil)

	mockService.On("CompleteStreamUpload", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(nil)

	mockService.On("CanUserAccessStream", mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID")).
		Return(true, nil)

	mockService.On("ListStreams", mock.Anything, mock.AnythingOfType("repository.StreamFilter")).
		Return([]*models.Stream{
			createTestStreamData(uuid.New(), "Stream 1", testUserID),
			createTestStreamData(uuid.New(), "Stream 2", testUserID),
		}, nil)

	mockService.On("ListUserStreams", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return([]*models.Stream{
			createTestStreamData(uuid.New(), "User Stream 1", testUserID),
			createTestStreamData(uuid.New(), "User Stream 2", testUserID),
		}, nil)
}
