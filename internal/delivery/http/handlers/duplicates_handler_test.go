package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service"
	servicemock "github.com/mrhumster/stream-service/internal/service/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStreamHandler_ListDuplicateStreams(t *testing.T) {
	userID := uuid.New()

	t.Run("returns grouped duplicates", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)

		keep := &models.Stream{BaseModel: models.BaseModel{ID: uuid.New()}, Title: "clip", Status: models.StatusReady}
		deletable := &models.Stream{BaseModel: models.BaseModel{ID: uuid.New()}, Title: "clip", Status: models.StatusDraft}
		protected := &models.Stream{BaseModel: models.BaseModel{ID: uuid.New()}, Title: "clip", Status: models.StatusPublished}

		mockService.EXPECT().FindDuplicateStreams(gomock.Any(), userID).Return([]service.StreamDuplicateGroup{
			{Title: "clip", Keep: keep, Deletable: []*models.Stream{deletable}, Protected: []*models.Stream{protected}},
		}, nil)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.GET("/stream/own/duplicates", handler.ListDuplicateStreams)

		req := httptest.NewRequest("GET", "/stream/own/duplicates", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.DuplicatesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.TotalGroups)
		assert.Equal(t, 1, resp.TotalDeletable)
		assert.Equal(t, 1, resp.TotalProtected)
		require.Len(t, resp.Groups, 1)
		assert.Equal(t, 3, resp.Groups[0].Count)
		assert.Equal(t, keep.ID, resp.Groups[0].Keep.ID)
		require.Len(t, resp.Groups[0].Deletable, 1)
		require.Len(t, resp.Groups[0].Protected, 1)
	})

	t.Run("propagates internal error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)

		mockService.EXPECT().FindDuplicateStreams(gomock.Any(), userID).Return(nil, errors.New("boom"))

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.GET("/stream/own/duplicates", handler.ListDuplicateStreams)

		req := httptest.NewRequest("GET", "/stream/own/duplicates", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
