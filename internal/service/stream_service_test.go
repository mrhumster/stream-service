package service_test

import (
	"testing"

	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/service/mocks"
)

func TestStreamServiceInterface(t *testing.T) {
	var _ service.StreamService = (*mocks.MockStreamService)(nil)
}

func TestStreamService_GetStream(t *testing.T) {
	t.Skip("implementation pendig")
}

func TestStreamService_PublishStream(t *testing.T) {
	// ctx := context.Background()

	t.Run("publish ready stream", func(t *testing.T) {
		t.Skip("implementation pendig")
	})

	t.Run("cannot publish draft stream", func(t *testing.T) {
		t.Skip("implementation pendig")
	})
}
