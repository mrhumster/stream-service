package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/domain/models"
	servicemock "github.com/mrhumster/stream-service/internal/service/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.Default()
}

func TestStreamHandler_ReadStream(t *testing.T) {
	router := setupTestRouter()

	router.Use(func(c *gin.Context) {
		c.Set("userID", "00000000-0000-0000-0000-000000000000")
		c.Next()
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockService := servicemock.NewMockStreamService(ctrl)
	handler := NewStreamHandler(mockService)
	router.GET("/streams/:id", handler.GetStream)
	t.Run("successfull read stream", func(t *testing.T) {
		expectedStream := &models.Stream{
			Title:   "Test Stream",
			OwnerID: uuid.New(),
			Status:  models.StatusDraft,
		}
		streamID := uuid.New()
		expectedStream.ID = streamID

		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(expectedStream, nil)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", streamID.String()), nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp response.StreamResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, expectedStream.ID, resp.ID)
		assert.Equal(t, expectedStream.Title, resp.Title)
	})

	t.Run("invalid stream id return error", func(t *testing.T) {
		invalidID := "undefined"
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", invalidID), nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestStreamHandler_CreateStream(t *testing.T) {

	t.Run("successfull creation", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "12345678-1234-5678-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.POST("/streams", handler.CreateStream)

		expecetedStream := &models.Stream{
			Title:   "Test Stream",
			OwnerID: uuid.New(),
			Status:  models.StatusDraft,
		}
		streamID := uuid.New()
		expecetedStream.ID = streamID

		mockService.EXPECT().CreateStream(gomock.Any(), gomock.Any()).Return(expecetedStream, nil)

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
	})

	t.Run("validation error", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.POST("/streams", handler.CreateStream)

		reqBody := `{"Title": "", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
