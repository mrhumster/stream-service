package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.Default()
}

func TestStreamHandler_CreateStream(t *testing.T) {
	mockService := &mocks.MockStreamService{}
	handler := NewStreamHandler(mockService)

	router := setupTestRouter()

	router.Use(func(c *gin.Context) {
		c.Set("userID", "00000000-0000-0000-0000-000000000000")
		c.Next()
	})

	router.POST("/streams", handler.CreateStream)

	t.Run("successfull creation", func(t *testing.T) {
		expecetedStream := &models.Stream{
			Title:   "Test Stream",
			OwnerID: uuid.New(),
			Status:  models.StatusDraft,
		}
		streamID := uuid.New()
		expecetedStream.ID = streamID
		mockService.On("CreateStream", mock.Anything, mock.Anything).Return(expecetedStream, nil)
		reqBody := `{"Title": "Test Stream", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var resp response.StreamResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, expecetedStream.ID, resp.ID)
		assert.Equal(t, expecetedStream.Title, resp.Title)
		mockService.AssertExpectations(t)
	})

	t.Run("validation error", func(t *testing.T) {

		reqBody := `{"Title": "", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})
}
