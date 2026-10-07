package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/mrhumster/identity-service/pkg/dto"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/request"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	servicemock "github.com/mrhumster/stream-service/internal/service/mock"
	"github.com/mrhumster/stream-service/internal/wss/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.Default()
}

func TestStreamHandler_GetStreamStatus(t *testing.T) {
	t.Run("returns status and visibility", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)

		streamID := uuid.New()
		mockService.EXPECT().GetStreamStatus(gomock.Any(), streamID).Return(&models.Stream{
			Status:     models.StatusPublished,
			Visibility: models.VisibilityPublic,
			OwnerID:    streamID,
			Title:      "Sunset",
		}, nil)

		router := setupTestRouter()
		router.GET("/stream/:id/status", handler.GetStreamStatus)
		req := httptest.NewRequest("GET", fmt.Sprintf("/stream/%s/status", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "published", resp["status"])
		assert.Equal(t, "public", resp["visibility"])
		assert.Equal(t, streamID.String(), resp["owner_id"])
		assert.Equal(t, "Sunset", resp["title"])
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)

		streamID := uuid.New()
		mockService.EXPECT().GetStreamStatus(gomock.Any(), streamID).Return(nil, service.ErrStreamNotFound)

		router := setupTestRouter()
		router.GET("/stream/:id/status", handler.GetStreamStatus)
		req := httptest.NewRequest("GET", fmt.Sprintf("/stream/%s/status", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid stream id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)

		router := setupTestRouter()
		router.GET("/stream/:id/status", handler.GetStreamStatus)
		req := httptest.NewRequest("GET", "/stream/not-a-uuid/status", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("propagates internal error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)

		streamID := uuid.New()
		mockService.EXPECT().GetStreamStatus(gomock.Any(), streamID).Return(nil, errors.New("boom"))

		router := setupTestRouter()
		router.GET("/stream/:id/status", handler.GetStreamStatus)
		req := httptest.NewRequest("GET", fmt.Sprintf("/stream/%s/status", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestStreamHandler_GetStream(t *testing.T) {
	router := setupTestRouter()

	router.Use(func(c *gin.Context) {
		userUUID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")
		c.Set("user", userUUID)
		c.Next()
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockService := servicemock.NewMockStreamService(ctrl)
	handler := NewStreamHandler(mockService, nil)
	router.GET("/streams/:id", handler.GetStream)

	t.Run("invalid user ID type", func(t *testing.T) {
		r1 := setupTestRouter()
		r1.Use(func(c *gin.Context) {
			c.Set("user", 12345)
			c.Next()
		})
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		mockService.EXPECT().GetStream(gomock.Any(), gomock.Any()).Return(&models.Stream{
			Visibility: models.VisibilityPrivate,
		}, nil)
		handler1 := NewStreamHandler(mockService, nil)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", uuid.New()), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("request without userID", func(t *testing.T) {
		r1 := setupTestRouter()
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		mockService.EXPECT().GetStream(gomock.Any(), gomock.Any()).Return(&models.Stream{
			Visibility: models.VisibilityPrivate,
		}, nil)
		handler1 := NewStreamHandler(mockService, nil)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", uuid.New()), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("propagation service error", func(t *testing.T) {
		r1 := setupTestRouter()
		r1.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})

		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		mockService.EXPECT().GetStream(gomock.Any(), gomock.Any()).Return(nil, errors.New("expected error"))
		handler1 := NewStreamHandler(mockService, nil)
		r1.GET("/streams/:id", handler1.GetStream)
		streamID := uuid.New()
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", streamID), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, resp["error"], "internal server error")
	})

	t.Run("successfull read stream", func(t *testing.T) {
		expectedStream := &models.Stream{
			Title:      "Test Stream",
			OwnerID:    uuid.New(),
			Status:     models.StatusDraft,
			Visibility: models.VisibilityPublic,
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

	t.Run("invalid user id return errror", func(t *testing.T) {
	})

	t.Run("unlisted stream available to non-owner by link", func(t *testing.T) {
		r1 := setupTestRouter()
		owner := uuid.New()
		otherUser := uuid.New()
		r1.Use(func(c *gin.Context) {
			c.Set("user", otherUser)
			c.Next()
		})
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Visibility: models.VisibilityUnlisted,
			OwnerID:    owner,
			Status:     models.StatusPublished,
			Title:      "hidden gem",
		}, nil)
		handler1 := NewStreamHandler(mockService, nil)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", streamID), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp response.StreamResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "hidden gem", resp.Title)
	})

	t.Run("private stream forbidden for non-owner", func(t *testing.T) {
		r1 := setupTestRouter()
		owner := uuid.New()
		otherUser := uuid.New()
		r1.Use(func(c *gin.Context) {
			c.Set("user", otherUser)
			c.Next()
		})
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Visibility: models.VisibilityPrivate,
			OwnerID:    owner,
			Status:     models.StatusPublished,
		}, nil)
		handler1 := NewStreamHandler(mockService, nil)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", streamID), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("private stream visible to owner", func(t *testing.T) {
		r1 := setupTestRouter()
		owner := uuid.New()
		r1.Use(func(c *gin.Context) {
			c.Set("user", owner)
			c.Next()
		})
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Visibility: models.VisibilityPrivate,
			OwnerID:    owner,
			Status:     models.StatusPublished,
			Title:      "my private",
		}, nil)
		handler1 := NewStreamHandler(mockService, nil)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", streamID), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("anonymous sees public stream", func(t *testing.T) {
		r1 := setupTestRouter()
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Visibility: models.VisibilityPublic,
			OwnerID:    uuid.New(),
			Status:     models.StatusPublished,
			Title:      "public",
		}, nil)
		handler1 := NewStreamHandler(mockService, nil)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", streamID), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestStreamHandler_CreateStream(t *testing.T) {
	t.Run("propagation validation error", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", "non-valid-uuid")
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)

		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": "Title", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("propagation validation error to service req", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.Nil)
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)

		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": "Title", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("propagation service error", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().CreateStream(gomock.Any(), gomock.Any()).Return(nil, errors.New("title cannot be empty"))

		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": "Title", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, resp["error"], "internal server error")
	})

	t.Run("error bind json", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": Test Stream, "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("error bind json", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": Test Stream, "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("creation only auth users", func(t *testing.T) {
		router := setupTestRouter()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": "Test Stream", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("bad user ID type", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", 12345)
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": "Test Stream", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("successfull creation", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
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
			c.Set("user", uuid.New())
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)

		reqBody := `{"Title": "", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unverified member is forbidden", func(t *testing.T) {
		router := setupTestRouter()
		userID := uuid.New()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Set("claims", &dto.AccessClaims{UserID: userID.String(), Role: "member", EmailVerified: false})
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)

		reqBody := `{"Title": "Test Stream", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "email not verified", resp["error"])
	})

	t.Run("verified member can create", func(t *testing.T) {
		router := setupTestRouter()
		userID := uuid.New()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Set("claims", &dto.AccessClaims{UserID: userID.String(), Role: "member", EmailVerified: true})
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		expecetedStream := &models.Stream{Title: "Test Stream", OwnerID: userID, Status: models.StatusDraft}
		expecetedStream.ID = uuid.New()
		mockService.EXPECT().CreateStream(gomock.Any(), gomock.Any()).Return(expecetedStream, nil)

		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)

		reqBody := `{"Title": "Test Stream", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("unverified admin can create (admin auto-verified at bootstrap)", func(t *testing.T) {
		router := setupTestRouter()
		userID := uuid.New()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Set("claims", &dto.AccessClaims{UserID: userID.String(), Role: "admin", EmailVerified: false})
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		expecetedStream := &models.Stream{Title: "Test Stream", OwnerID: userID, Status: models.StatusDraft}
		expecetedStream.ID = uuid.New()
		mockService.EXPECT().CreateStream(gomock.Any(), gomock.Any()).Return(expecetedStream, nil)

		handler := NewStreamHandler(mockService, nil)
		router.POST("/streams", handler.CreateStream)

		reqBody := `{"Title": "Test Stream", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestStreamHandler_UpdateStream(t *testing.T) {
	t.Run("Update stream successfull", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.PATCH("/streams/:id", handler.UpdateStream)
		existiongStream := uuid.New()
		updatedStream := &models.Stream{
			Title:       "Updated title",
			Visibility:  "public",
			Description: "Updated description",
		}
		updatedStream.ID = existiongStream

		mockService.EXPECT().UpdateStream(gomock.Any(), existiongStream, gomock.Any()).Return(updatedStream, nil)

		reqBody := `{"Title": "Updated title", "Visibility": "public", "Description": "Updated description"}`
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.StreamResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Updated title", resp.Title)
	})

	t.Run("Validation error", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.PATCH("/streams/:id", handler.UpdateStream)
		existiongStream := uuid.New()
		reqBody := `{"Title": "", "Visibility": "public", "Description": "Updated description"}`
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad stream ID type", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.PATCH("/streams/:id", handler.UpdateStream)
		reqBody := `{"Title": "", "Visibility": "public", "Description": "Updated description"}`
		req := httptest.NewRequest("PATCH", "/streams/bad-stream-id", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("bad json", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.PATCH("/streams/:id", handler.UpdateStream)
		existiongStream := uuid.New()
		reqBody := `{"Title": "", "Visibility": public, "Description": "Updated description"}`
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service returns validation error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", "test-user-uuid")
			c.Next()
		})
		router.PATCH("/streams/:id", handler.UpdateStream)

		streamID := uuid.New()

		mockService.EXPECT().
			UpdateStream(gomock.Any(), streamID, gomock.Any()).
			Return(nil, errors.New("title cannot be empty"))

		reqBody := `{"title": "Valid Title"}`
		req := httptest.NewRequest("PATCH", "/streams/"+streamID.String(),
			bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, resp["error"], "internal server error")
	})
}

func TestStreamHandler_DeleteStream(t *testing.T) {
	t.Run("delete succesfull", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		router := setupTestRouter()
		handlers := NewStreamHandler(mockService, nil)
		router.DELETE("/streams/:id", handlers.DeleteStream)
		existingStreamID := uuid.New()
		mockService.EXPECT().DeleteStream(gomock.Any(), existingStreamID).Return(nil)
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/streams/%s", existingStreamID.String()), nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid stream ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		router := setupTestRouter()
		handlers := NewStreamHandler(mockService, nil)
		router.DELETE("/streams/:id", handlers.DeleteStream)
		existingStreamID := "invalid-uuid-struct"
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/streams/%s", existingStreamID), nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["error"], "invalid stream ID in param")
	})
}

func TestStreamHandler_PublishStream(t *testing.T) {
	t.Run("publish non-ready stream returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		userID := uuid.New()
		streamID := uuid.New()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			OwnerID:    userID,
			Status:     models.StatusDraft,
			Visibility: models.VisibilityPublic,
		}, nil)
		mockService.EXPECT().PublishStream(gomock.Any(), streamID).Return(service.ErrCannotPublish)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/publish", handlers.PublishStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/publish", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["error"], "stream must be ready before publishing")
	})

	t.Run("publish ready stream succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		userID := uuid.New()
		streamID := uuid.New()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			OwnerID:    userID,
			Status:     models.StatusReady,
			Visibility: models.VisibilityPublic,
		}, nil)
		mockService.EXPECT().PublishStream(gomock.Any(), streamID).Return(nil)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/publish", handlers.PublishStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/publish", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("publish by non-owner returns 403", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ownerID := uuid.New()
		otherUserID := uuid.New()
		streamID := uuid.New()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			OwnerID:    ownerID,
			Status:     models.StatusReady,
			Visibility: models.VisibilityPublic,
		}, nil)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", otherUserID)
			c.Set("claims", &dto.AccessClaims{Role: "member"})
			c.Next()
		})
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/publish", handlers.PublishStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/publish", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["error"], "only owner")
	})

	t.Run("publish by admin bypasses owner check", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ownerID := uuid.New()
		adminID := uuid.New()
		streamID := uuid.New()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			OwnerID:    ownerID,
			Status:     models.StatusReady,
			Visibility: models.VisibilityPublic,
		}, nil)
		mockService.EXPECT().PublishStream(gomock.Any(), streamID).Return(nil)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", adminID)
			c.Set("claims", &dto.AccessClaims{Role: "admin"})
			c.Next()
		})
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/publish", handlers.PublishStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/publish", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestStreamHandler_UnpublishStream(t *testing.T) {
	t.Run("unpublish non-published stream returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		userID := uuid.New()
		streamID := uuid.New()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			OwnerID:    userID,
			Status:     models.StatusReady,
			Visibility: models.VisibilityPublic,
		}, nil)
		mockService.EXPECT().UnpublishStream(gomock.Any(), streamID).Return(service.ErrCannotUnpublish)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/unpublish", handlers.UnpublishStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/unpublish", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["error"], "only published streams can be unpublished")
	})

	t.Run("unpublish published stream succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		userID := uuid.New()
		streamID := uuid.New()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			OwnerID:    userID,
			Status:     models.StatusPublished,
			Visibility: models.VisibilityPublic,
		}, nil)
		mockService.EXPECT().UnpublishStream(gomock.Any(), streamID).Return(nil)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/unpublish", handlers.UnpublishStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/unpublish", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("unpublish by admin bypasses owner check", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ownerID := uuid.New()
		adminID := uuid.New()
		streamID := uuid.New()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			OwnerID:    ownerID,
			Status:     models.StatusPublished,
			Visibility: models.VisibilityPublic,
		}, nil)
		mockService.EXPECT().UnpublishStream(gomock.Any(), streamID).Return(nil)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", adminID)
			c.Set("claims", &dto.AccessClaims{Role: "admin"})
			c.Next()
		})
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/unpublish", handlers.UnpublishStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/unpublish", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestStreamHandler_StartStreamUpload(t *testing.T) {
	t.Run("success case", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handlers := NewStreamHandler(mockService, nil)
		userID := uuid.New()
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.POST("/stream/:id/upload", handlers.UploadVideo)
		streamID := uuid.New()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("video", "test.mp4")
		require.NoError(t, err)
		_, err = part.Write([]byte("fake video data"))
		require.NoError(t, err)
		writer.Close()
		mockService.EXPECT().
			UploadVideo(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx any, req any) error {
				uploadReq := req.(service.UploadVideoRequest)
				assert.Equal(t, streamID, uploadReq.StreamID)
				assert.NotNil(t, uploadReq.File)
				assert.Equal(t, "test.mp4", uploadReq.FileName)
				return nil
			})

		req := httptest.NewRequest("POST", "/stream/"+streamID.String()+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "success")
	})

	t.Run("validation error - invalid file type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})
		router.POST("/stream/:id/upload", handler.UploadVideo)
		streamID := uuid.New()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("video", "test.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("text content"))
		require.NoError(t, err)
		writer.Close()
		req := httptest.NewRequest("POST", "/stream/"+streamID.String()+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid file extension")
	})

	t.Run("service return error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handlers := NewStreamHandler(mockService, nil)
		userID := uuid.New()
		streamID := uuid.New()
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.POST("/stream/:id/upload", handlers.UploadVideo)
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("video", "test.mp4")
		require.NoError(t, err)
		part.Write([]byte("video data"))
		writer.Close()
		expectedError := errors.New("stream not found")
		mockService.EXPECT().
			UploadVideo(gomock.Any(), gomock.Any()).
			Return(expectedError)

		req := httptest.NewRequest("POST", "/stream/"+streamID.String()+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "internal server error")
	})

	t.Run("without loginID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handlers := NewStreamHandler(mockService, nil)
		streamID := uuid.New()
		router := setupTestRouter()
		router.POST("/stream/:id/upload", handlers.UploadVideo)
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("video", "test.mp4")
		require.NoError(t, err)
		part.Write([]byte("video data"))
		writer.Close()
		req := httptest.NewRequest("POST", "/stream/"+streamID.String()+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("user id bad type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handlers := NewStreamHandler(mockService, nil)
		streamID := uuid.New()
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", 1234)
			c.Next()
		})
		router.POST("/stream/:id/upload", handlers.UploadVideo)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("video", "test.mp4")
		require.NoError(t, err)
		part.Write([]byte("video data"))
		writer.Close()
		req := httptest.NewRequest("POST", "/stream/"+streamID.String()+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("stream id bad type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handlers := NewStreamHandler(mockService, nil)
		streamID := "123455"
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})
		router.POST("/stream/:id/upload", handlers.UploadVideo)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("video", "test.mp4")
		require.NoError(t, err)
		part.Write([]byte("video data"))
		writer.Close()
		req := httptest.NewRequest("POST", "/stream/"+streamID+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid stream")
	})
	t.Run("video file required", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handlers := NewStreamHandler(mockService, nil)
		streamID := uuid.New()
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", uuid.New())
			c.Next()
		})
		router.POST("/stream/:id/upload", handlers.UploadVideo)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("not-video", "test.mp4")
		require.NoError(t, err)
		part.Write([]byte("video data"))
		writer.Close()
		req := httptest.NewRequest("POST", "/stream/"+streamID.String()+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "video file required")
	})
}

func TestStreamHandler_DownloadStream(t *testing.T) {
	setupTest := func() (*gin.Engine, *servicemock.MockStreamService) {
		router := setupTestRouter()
		user := uuid.New()
		router.Use(func(c *gin.Context) {
			c.Set("user", user)
			c.Next()
		})
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.GET("/streams/:id/download", handler.DownloadStream)
		return router, mockService
	}

	t.Run("streams bytes with a download filename", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		payload := "fake-mp4-bytes"
		mockService.EXPECT().
			OpenStreamDownload(gomock.Any(), streamID, gomock.Any()).
			Return(&service.DownloadStreamInfo{
				Content:     io.NopCloser(strings.NewReader(payload)),
				ContentType: "video/mp4",
				FileName:    "Отпуск на море.mp4",
				Size:        int64(len(payload)),
			}, nil)

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, payload, w.Body.String())
		assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
		assert.Equal(t, strconv.Itoa(len(payload)), w.Header().Get("Content-Length"))
		// A Cyrillic title must still produce a valid header: the quoted
		// parameter is downgraded to ASCII so old clients cannot choke on it,
		// and filename* carries the real name.
		disposition := w.Header().Get("Content-Disposition")
		const marker = `filename="`
		from := strings.Index(disposition, marker) + len(marker)
		quoted := disposition[from : from+strings.Index(disposition[from:], `"`)]
		for _, r := range quoted {
			assert.LessOrEqual(t, int(r), 127, "quoted filename must stay ASCII: %q", quoted)
		}
		assert.Contains(t, disposition, "filename*=UTF-8''"+url.PathEscape("Отпуск на море.mp4"))
		assert.Equal(t, ".mp4", filepath.Ext(quoted), "fallback keeps the extension so the OS opens it")
	})

	t.Run("wrong uuid", func(t *testing.T) {
		router, _ := setupTest()
		req := httptest.NewRequest("GET", "/streams/bad-uuid-format/download", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("stream not found", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().OpenStreamDownload(gomock.Any(), streamID, gomock.Any()).Return(nil, service.ErrStreamNotFound)

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		var resp response.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp.Error, "stream not found")
	})

	t.Run("export not requested yet", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().OpenStreamDownload(gomock.Any(), streamID, gomock.Any()).Return(nil, service.ErrExportNotFound)

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("export still being prepared", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().OpenStreamDownload(gomock.Any(), streamID, gomock.Any()).Return(nil, service.ErrExportPending)

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusConflict, w.Code)
		var resp response.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp.Error, "still being prepared")
	})

	t.Run("export failed", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().OpenStreamDownload(gomock.Any(), streamID, gomock.Any()).
			Return(nil, fmt.Errorf("export failed: %s", "ffmpeg exited 1"))

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusConflict, w.Code)
		var resp response.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// The worker error must not leak into the response body.
		assert.NotContains(t, resp.Error, "ffmpeg")
	})

	t.Run("not the owner", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().OpenStreamDownload(gomock.Any(), streamID, gomock.Any()).Return(nil, service.ErrStreamForbidden)

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("unexpected error", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().OpenStreamDownload(gomock.Any(), streamID, gomock.Any()).Return(nil, errors.New("minio down"))

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		var resp response.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp.Error, "failed to prepare download")
	})
}

func TestStreamHandler_Export(t *testing.T) {
	setupTest := func() (*gin.Engine, *servicemock.MockStreamService) {
		router := setupTestRouter()
		user := uuid.New()
		router.Use(func(c *gin.Context) {
			c.Set("user", user)
			c.Set("claims", &dto.AccessClaims{Email: "owner@example.com", EmailVerified: true})
			c.Next()
		})
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.GET("/streams/:id/export", handler.GetExport)
		router.POST("/streams/:id/export", handler.RequestExport)
		return router, mockService
	}

	t.Run("request queues the export and returns 202", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		// The email is taken from the signed token, not from a body field, so a
		// caller cannot redirect the notification to someone else.
		mockService.EXPECT().
			RequestStreamExport(gomock.Any(), streamID, gomock.Any(), "owner@example.com").
			Return(&service.StreamExportInfo{StreamID: streamID, Status: models.ExportStatusPending}, nil)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/export", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)
		var resp response.ExportResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, models.ExportStatusPending, resp.Status)
		assert.Equal(t, streamID.String(), resp.StreamID)
	})

	t.Run("already ready is reported as ready", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().RequestStreamExport(gomock.Any(), streamID, gomock.Any(), "owner@example.com").
			Return(&service.StreamExportInfo{StreamID: streamID, Status: models.ExportStatusReady, Size: 2048, FileName: "Summer in Kaliningrad.mp4"}, nil)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/export", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)
		var resp response.ExportResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, models.ExportStatusReady, resp.Status)
		assert.Equal(t, int64(2048), resp.Size)
		// The client cannot ask for the name later: its save dialog is opened
		// from the click, before the file is fetched. It has to arrive here.
		assert.Equal(t, "Summer in Kaliningrad.mp4", resp.FileName)
		assert.Contains(t, w.Body.String(), `"file_name":"Summer in Kaliningrad.mp4"`)
	})

	t.Run("request on a stream that is not ready", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().RequestStreamExport(gomock.Any(), streamID, gomock.Any(), gomock.Any()).
			Return(nil, service.ErrStreamNotReady)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/export", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("status read", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().GetStreamExport(gomock.Any(), streamID, gomock.Any()).
			Return(&service.StreamExportInfo{StreamID: streamID, Status: models.ExportStatusFailed, Error: "ffmpeg exited 1"}, nil)

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/export", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.ExportResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, models.ExportStatusFailed, resp.Status)
		assert.Equal(t, "ffmpeg exited 1", resp.Error)
	})

	t.Run("status read by a non-owner", func(t *testing.T) {
		router, mockService := setupTest()
		streamID := uuid.New()
		mockService.EXPECT().GetStreamExport(gomock.Any(), streamID, gomock.Any()).Return(nil, service.ErrStreamForbidden)

		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/export", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestStreamHandler_ListStreamPublic(t *testing.T) {
	setupTest := func() (*gin.Engine, *servicemock.MockStreamService, *StreamHandler) {
		router := setupTestRouter()
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.GET("/streams", handler.ListStreamPublic)
		return router, mockService, handler
	}

	t.Run("success stream list", func(t *testing.T) {
		router, mockService, _ := setupTest()
		ownerID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")
		streamList := []*models.Stream{
			{
				BaseModel:  models.BaseModel{ID: uuid.New()},
				Title:      "Stream 1",
				OwnerID:    ownerID,
				Visibility: models.VisibilityPublic,
			}, {
				BaseModel:  models.BaseModel{ID: uuid.New()},
				Title:      "Stream 2",
				OwnerID:    ownerID,
				Visibility: models.VisibilityPublic,
			}, {
				BaseModel:  models.BaseModel{ID: uuid.New()},
				Title:      "Not my stream",
				OwnerID:    uuid.New(),
				Visibility: models.VisibilityPublic,
			},
		}
		mockService.EXPECT().ListStreams(gomock.Any(), gomock.Any()).Return(streamList, int64(3), nil)
		req := httptest.NewRequest("GET", "/streams?limit=10&offset=1", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusOK)
		var resp response.ListReponse[*models.Stream]
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Len(t, resp.Items, 3)
	})

	t.Run("validation limit error", func(t *testing.T) {
		router, _, _ := setupTest()
		req := httptest.NewRequest("GET", "/streams?limit=bad", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("validation offset error", func(t *testing.T) {
		router, _, _ := setupTest()
		req := httptest.NewRequest("GET", "/streams?offset=bad", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("service error propagate", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().
			ListStreams(gomock.Any(), gomock.Any()).
			Return(nil, int64(0), errors.New("srevice error"))
		req := httptest.NewRequest("GET", "/streams", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusInternalServerError)
	})

	t.Run("propagates sort and order", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().ListStreams(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
				require.Equal(t, "recorded_at", filter.SortBy)
				require.Equal(t, "asc", filter.SortOrder)
				return []*models.Stream{}, int64(0), nil
			},
		)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?sort=recorded_at&order=asc", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusOK)
	})

	t.Run("rejects invalid sort", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?sort=owner_id", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("rejects invalid order", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?order=sideways", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})
}

func TestStreamHandler_ListStreamOwner(t *testing.T) {
	userID := uuid.New()

	setupTest := func() (*gin.Engine, *servicemock.MockStreamService, *StreamHandler) {
		router := setupTestRouter()
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.GET("/streams", handler.ListStreamOwner)
		return router, mockService, handler
	}

	t.Run("seccesful downalod of stream list with owner", func(t *testing.T) {
		router, mockService, _ := setupTest()
		streams := []*models.Stream{
			{
				Title:   "Stream 1",
				OwnerID: userID,
				Status:  models.StatusPublished,
			},
			{
				Title:   "Stream 2",
				OwnerID: userID,
				Status:  models.StatusDraft,
			},
			{
				Title:   "Stream 3",
				OwnerID: userID,
				Status:  models.StatusPublished,
			},
			{
				Title:   "Stream 4",
				OwnerID: userID,
				Status:  models.StatusProcessing,
			},
		}
		ctx := context.Background()
		mockService.EXPECT().ListUserStreams(ctx, userID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ uuid.UUID, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
				return streams, int64(4), nil
			},
		)
		req := httptest.NewRequest("GET", "/streams", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp response.ListReponse[response.StreamResponse]
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, resp.Total, int64(4))
		assert.Len(t, resp.Items, 4)
	})

	t.Run("only auth user can make request", func(t *testing.T) {
		routerWithoutUserID := setupTestRouter()
		controller := gomock.NewController(t)
		defer controller.Finish()
		mockService := servicemock.NewMockStreamService(controller)
		handler := NewStreamHandler(mockService, nil)
		routerWithoutUserID.GET("/streams", handler.ListStreamOwner)
		req := httptest.NewRequest("GET", "/streams", nil)
		w := httptest.NewRecorder()
		routerWithoutUserID.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusInternalServerError)
	})

	t.Run("error parse user uuid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", "1234asdasw")
			c.Next()
		})
		router.GET("/streams", handler.ListStreamOwner)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusInternalServerError)
	})

	t.Run("propagation service error", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().ListUserStreams(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, int64(0), errors.New("service error"))
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusInternalServerError)
		assert.Contains(t, w.Body.String(), "internal server error")
	})

	t.Run("propagates limit and offset query params", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().ListUserStreams(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ uuid.UUID, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
				require.Equal(t, 25, filter.Limit)
				require.Equal(t, 50, filter.Offset)
				return []*models.Stream{}, int64(0), nil
			},
		)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?limit=25&offset=50", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusOK)
	})

	t.Run("rejects limit above 100", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?limit=101", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("rejects invalid offset", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?offset=-1", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("propagates status filter", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().ListUserStreams(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ uuid.UUID, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
				require.NotNil(t, filter.Status)
				require.Equal(t, models.StatusReady, *filter.Status)
				return []*models.Stream{}, int64(0), nil
			},
		)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?status=ready", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusOK)
	})

	t.Run("rejects invalid status", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?status=bogus", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("propagates faces_detected filter", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().ListUserStreams(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ uuid.UUID, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
				require.NotNil(t, filter.FacesDetected)
				require.True(t, *filter.FacesDetected)
				return []*models.Stream{}, int64(0), nil
			},
		)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?faces_detected=true", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusOK)
	})

	t.Run("rejects invalid faces_detected", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?faces_detected=maybe", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("propagates sort and order", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().ListUserStreams(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ uuid.UUID, filter repository.StreamFilter) ([]*models.Stream, int64, error) {
				require.Equal(t, "title", filter.SortBy)
				require.Equal(t, "asc", filter.SortOrder)
				return []*models.Stream{}, int64(0), nil
			},
		)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?sort=title&order=asc", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusOK)
	})

	t.Run("rejects invalid sort", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?sort=owner_id", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	t.Run("rejects invalid order", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?order=sideways", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, w.Code, http.StatusBadRequest)
	})

	// An empty result must be an empty JSON array, never null: the web client
	// maps over items inside an RTK Query reducer, and a null slice made the
	// fulfilled update throw, leaving the request pending forever.
	t.Run("empty result returns items as empty array not null", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().ListUserStreams(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, int64(0), nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/streams?status=draft", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"items":[]`)
		assert.NotContains(t, w.Body.String(), `"items":null`)

		var resp struct {
			Items  []response.StreamResponse `json:"items"`
			Total  int64                     `json:"total"`
			Offset int                       `json:"offset"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotNil(t, resp.Items)
		assert.Len(t, resp.Items, 0)
		assert.Equal(t, int64(0), resp.Total)
	})
}

func TestStreamHandler_StreamUpload(t *testing.T) {
	userID := uuid.New()
	ctx := context.Background()
	setupTest := func() (*gin.Engine, *servicemock.MockStreamService, *StreamHandler) {
		router := setupTestRouter()
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.POST("/streams/:id/upload/complete", handler.CompleteUpload)
		router.POST("/streams/:id/upload/init", handler.InitUpload)
		router.PUT("/streams/:id/upload/part", handler.PartUpload)
		return router, mockService, handler
	}
	t.Run("successful upload partition", func(t *testing.T) {
		router, mockService, _ := setupTest()
		w := httptest.NewRecorder()
		streamID := uuid.New()
		body := request.StartUploadRequest{
			FileName:    "video.mp4",
			TotalSize:   int64(100),
			ContentType: "video/mp4",
		}
		uploadInfo := &service.UploadInfo{
			UploadID: "UploadID-123",
			StreamID: streamID,
		}

		serviceReq, _ := body.ToService(streamID, userID)

		mockService.EXPECT().
			StartStreamUpload(
				ctx,
				*serviceReq).
			Return(uploadInfo, nil)
		jsonBody, _ := json.Marshal(body)
		url := fmt.Sprintf("/streams/%s/upload/init", streamID.String())
		req := httptest.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
		router.ServeHTTP(w, req)
		assert.Equal(t, w.Code, http.StatusOK)
		assert.Contains(t, w.Body.String(), "upload_id")
	})

	t.Run("init upload bad stream uuid", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		streamID := "1234"

		body := request.StartUploadRequest{
			FileName:    "video.mp4",
			TotalSize:   int64(100),
			ContentType: "video/mp4",
		}
		jsonBody, _ := json.Marshal(body)
		url := fmt.Sprintf("/streams/%s/upload/init", streamID)
		req := httptest.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
		router.ServeHTTP(w, req)
		assert.Equal(t, w.Code, http.StatusBadRequest)
		assert.Contains(t, w.Body.String(), "invalid stream id")
	})

	t.Run("init upload error bind json", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		streamID := uuid.New()
		url := fmt.Sprintf("/streams/%s/upload/init", streamID)
		req := httptest.NewRequest("POST", url, bytes.NewBuffer([]byte{'s'}))
		router.ServeHTTP(w, req)
		assert.Equal(t, w.Code, http.StatusBadRequest)
		assert.Contains(t, w.Body.String(), "invalid request body")
	})

	t.Run("init upload service error", func(t *testing.T) {
		router, mockService, _ := setupTest()
		w := httptest.NewRecorder()
		streamID := uuid.New()
		body := request.StartUploadRequest{
			FileName:    "video.mp4",
			TotalSize:   int64(100),
			ContentType: "video/mp4",
		}

		serviceReq, _ := body.ToService(streamID, userID)

		mockService.EXPECT().
			StartStreamUpload(
				ctx,
				*serviceReq).
			Return(nil, fmt.Errorf("service error"))
		jsonBody, _ := json.Marshal(body)
		url := fmt.Sprintf("/streams/%s/upload/init", streamID.String())
		req := httptest.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
		router.ServeHTTP(w, req)
		assert.Equal(t, w.Code, http.StatusInternalServerError)
		assert.Contains(t, w.Body.String(), "internal server error")
	})

	t.Run("part upload successfull", func(t *testing.T) {
		router, mockService, _ := setupTest()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("uploadID", "UploadID-123")
		_ = writer.WriteField("partNumber", "1")
		part, _ := writer.CreateFormFile("video", "chunk.mp4")
		part.Write([]byte("fake-video-data"))
		writer.Close()
		mockService.EXPECT().UploadPart(ctx, gomock.Any()).Return(&models.MultipartPart{
			PartNumber: 1, ETag: "qwerty",
		}, nil)
		url := fmt.Sprintf("/streams/%s/upload/part", uuid.New())
		req := httptest.NewRequest("PUT", url, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("part upload bad id", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		url := fmt.Sprintf("/streams/%s/upload/part", "badUUID")
		req := httptest.NewRequest("PUT", url, bytes.NewBuffer([]byte{'s'}))
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid stream id")
	})
	t.Run("part upload bad bind form", func(t *testing.T) {
		router, _, _ := setupTest()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("uploadID", "UploadID-123")
		writer.Close()
		url := fmt.Sprintf("/streams/%s/upload/part", uuid.New())
		req := httptest.NewRequest("PUT", url, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid request body")
	})
	t.Run("part upload toService request err", func(t *testing.T) {
		router, _, _ := setupTest()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("uploadID", "UploadID-123")
		_ = writer.WriteField("partNumber", "1")
		part, _ := writer.CreateFormFile("video", "chunk.mp4")
		part.Write([]byte("fake-video-data"))
		writer.Close()
		url := fmt.Sprintf("/streams/%s/upload/part", uuid.Nil)
		req := httptest.NewRequest("PUT", url, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid part upload request")
	})

	t.Run("part upload service error propagate", func(t *testing.T) {
		router, mockService, _ := setupTest()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("uploadID", "UploadID-123")
		_ = writer.WriteField("partNumber", "1")
		part, _ := writer.CreateFormFile("video", "chunk.mp4")
		part.Write([]byte("fake-video-data"))
		writer.Close()
		mockService.EXPECT().UploadPart(ctx, gomock.Any()).Return(nil, fmt.Errorf("service error"))
		url := fmt.Sprintf("/streams/%s/upload/part", uuid.New())
		req := httptest.NewRequest("PUT", url, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "internal server error")
	})
	t.Run("complete upload successfull", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().CompleteStreamUpload(ctx, gomock.Any()).Return(nil)
		w := httptest.NewRecorder()
		streamID := uuid.New()
		url := fmt.Sprintf("/streams/%s/upload/complete", streamID.String())
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "tag1",
			},
			{
				PartNumber: 2,
				ETag:       "tag2",
			},
		}
		body := request.CompleteUploadRequest{
			Parts: parts,
		}
		bodyJSON, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNoContent, w.Code)
	})
	t.Run("complete upload bad stream uuid", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		url := fmt.Sprintf("/streams/%s/upload/complete", "111")
		req := httptest.NewRequest("POST", url, bytes.NewBuffer([]byte(`s`)))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid stream id")
	})
	t.Run("complete upload bad bind json", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		url := fmt.Sprintf("/streams/%s/upload/complete", uuid.New())
		req := httptest.NewRequest("POST", url, bytes.NewBuffer([]byte(`s`)))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid request body")
	})
	t.Run("complete upload validation error", func(t *testing.T) {
		router, _, _ := setupTest()
		w := httptest.NewRecorder()
		url := fmt.Sprintf("/streams/%s/upload/complete", uuid.Nil)
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "tag1",
			},
			{
				PartNumber: 2,
				ETag:       "tag2",
			},
		}
		body := request.CompleteUploadRequest{
			Parts: parts,
		}
		bodyJSON, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid complete upload request")
	})
	t.Run("complete upload service error CompleteStreamUpload", func(t *testing.T) {
		router, mockService, _ := setupTest()
		mockService.EXPECT().CompleteStreamUpload(ctx, gomock.Any()).Return(fmt.Errorf("service error"))
		w := httptest.NewRecorder()
		streamID := uuid.New()
		url := fmt.Sprintf("/streams/%s/upload/complete", streamID.String())
		parts := []models.MultipartPart{
			{
				PartNumber: 1,
				ETag:       "tag1",
			},
			{
				PartNumber: 2,
				ETag:       "tag2",
			},
		}
		body := request.CompleteUploadRequest{
			Parts: parts,
		}
		bodyJSON, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "internal server error")
	})
}

func TestStreamHandler_GetHLS(t *testing.T) {
	userID := uuid.New()
	setupTest := func() (*gin.Engine, *servicemock.MockStreamService, *StreamHandler) {
		router := setupTestRouter()
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService, nil)
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.GET("/streams/:id/hls/*file", handler.GetHLS)
		return router, mockService, handler
	}
	t.Run("success get hls", func(t *testing.T) {
		router, mockService, _ := setupTest()
		streamUUID := uuid.New()
		file := "0x00, 0x01, 0x02, 0x03"
		content := io.NopCloser(strings.NewReader(file))
		filename := "/index.m3u8"

		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   filename,
		}
		svcRes := &service.GetFileByKeyResponse{
			Content:     content,
			ContentType: "application/x-mpegURL",
			Size:        int64(len(file)),
		}
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamUUID},
			Status:    models.StatusPublished,
		}
		mockService.EXPECT().GetStream(gomock.Any(), streamUUID).Return(expectedStream, nil)
		mockService.EXPECT().
			GetFileByKey(gomock.Any(), svcReq).
			Return(svcRes, nil)
		url := fmt.Sprintf("/streams/%s/hls/index.m3u8", streamUUID)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", url, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-mpegURL", w.Header().Get("Content-Type"))
		assert.Equal(t, []byte(file), w.Body.Bytes())
	})

	t.Run("stream uuid parse error", func(t *testing.T) {
		router, _, _ := setupTest()
		streamUUID := "bad-uuid-format"
		url := fmt.Sprintf("/streams/%s/hls/index.m3u8", streamUUID)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", url, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid stream id")
	})

	t.Run("file cannot be empty error", func(t *testing.T) {
		router, mockService, _ := setupTest()
		streamUUID := uuid.New()
		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamUUID},
			Status:    models.StatusPublished,
		}
		mockService.EXPECT().GetStream(gomock.Any(), streamUUID).Return(expectedStream, nil)
		url := fmt.Sprintf("/streams/%s/hls/", streamUUID)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", url, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "filename cannot be empty")
	})
	t.Run("service error propagation", func(t *testing.T) {
		router, mockService, _ := setupTest()
		streamUUID := uuid.New()
		filename := "/index.m3u8"

		expectedStream := &models.Stream{
			BaseModel: models.BaseModel{ID: streamUUID},
			Status:    models.StatusPublished,
		}
		svcReq := &service.GetFileByKeyRequest{
			StreamUUID: streamUUID,
			FileName:   filename,
		}
		mockService.EXPECT().GetStream(gomock.Any(), streamUUID).Return(expectedStream, nil)
		mockService.EXPECT().
			GetFileByKey(gomock.Any(), svcReq).
			Return(nil, fmt.Errorf("file not found"))
		url := fmt.Sprintf("/streams/%s/hls/index.m3u8", streamUUID)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", url, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "internal server error")
	})
}

func TestStreamHandler_GetHLS_Visibility(t *testing.T) {
	ownerID := uuid.New()

	newAnonRouter := func(t *testing.T, stream *models.Stream, mockService *servicemock.MockStreamService) *gin.Engine {
		handler := NewStreamHandler(mockService, nil)
		router := setupTestRouter()
		router.GET("/streams/:id/hls/*file", handler.GetHLS)
		return router
	}

	serve := func(t *testing.T, router *gin.Engine, streamID uuid.UUID) *httptest.ResponseRecorder {
		url := fmt.Sprintf("/streams/%s/hls/index.m3u8", streamID)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", url, nil)
		router.ServeHTTP(w, req)
		return w
	}

	t.Run("published private stream forbidden for anonymous", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Status:     models.StatusPublished,
			Visibility: models.VisibilityPrivate,
			OwnerID:    ownerID,
		}, nil)

		router := newAnonRouter(t, nil, mockService)
		w := serve(t, router, streamID)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("published private stream visible to owner", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Status:     models.StatusPublished,
			Visibility: models.VisibilityPrivate,
			OwnerID:    ownerID,
		}, nil)

		content := io.NopCloser(strings.NewReader("#EXTM3U"))
		mockService.EXPECT().GetFileByKey(gomock.Any(), &service.GetFileByKeyRequest{
			StreamUUID: streamID,
			FileName:   "/index.m3u8",
		}).Return(&service.GetFileByKeyResponse{
			Content:     content,
			ContentType: "application/x-mpegURL",
			Size:        int64(len("#EXTM3U")),
		}, nil)

		handler := NewStreamHandler(mockService, nil)
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("user", ownerID)
			c.Next()
		})
		router.GET("/streams/:id/hls/*file", handler.GetHLS)

		w := serve(t, router, streamID)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("published unlisted stream open by link for anonymous", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Status:     models.StatusPublished,
			Visibility: models.VisibilityUnlisted,
			OwnerID:    ownerID,
		}, nil)

		content := io.NopCloser(strings.NewReader("#EXTM3U"))
		mockService.EXPECT().GetFileByKey(gomock.Any(), &service.GetFileByKeyRequest{
			StreamUUID: streamID,
			FileName:   "/index.m3u8",
		}).Return(&service.GetFileByKeyResponse{
			Content:     content,
			ContentType: "application/x-mpegURL",
			Size:        int64(len("#EXTM3U")),
		}, nil)

		router := newAnonRouter(t, nil, mockService)
		w := serve(t, router, streamID)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("published public stream open for anonymous", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Status:     models.StatusPublished,
			Visibility: models.VisibilityPublic,
			OwnerID:    ownerID,
		}, nil)

		content := io.NopCloser(strings.NewReader("#EXTM3U"))
		mockService.EXPECT().GetFileByKey(gomock.Any(), &service.GetFileByKeyRequest{
			StreamUUID: streamID,
			FileName:   "/index.m3u8",
		}).Return(&service.GetFileByKeyResponse{
			Content:     content,
			ContentType: "application/x-mpegURL",
			Size:        int64(len("#EXTM3U")),
		}, nil)

		router := newAnonRouter(t, nil, mockService)
		w := serve(t, router, streamID)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("unpublished stream forbidden for non-owner", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		streamID := uuid.New()
		mockService.EXPECT().GetStream(gomock.Any(), streamID).Return(&models.Stream{
			BaseModel:  models.BaseModel{ID: streamID},
			Status:     models.StatusReady,
			Visibility: models.VisibilityPublic,
			OwnerID:    ownerID,
		}, nil)

		router := newAnonRouter(t, nil, mockService)
		w := serve(t, router, streamID)
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestStreamHandler_HandleWS(t *testing.T) {
	userID := uuid.New()
	setupTest := func() (*gin.Engine, *servicemock.MockStreamService, *StreamHandler, *mock.MockHub) {
		router := setupTestRouter()
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		mockHub := mock.NewMockHub(ctrl)
		handler := NewStreamHandler(mockService, mockHub)
		router.Use(func(c *gin.Context) {
			c.Set("user", userID)
			c.Next()
		})
		router.GET("/streams/ws/upgrade", handler.HandleWS)
		return router, mockService, handler, mockHub
	}

	t.Run("success", func(t *testing.T) {
		router, _, _, mockHub := setupTest()
		unregistered := make(chan struct{})
		mockHub.EXPECT().Register(userID, gomock.Any()).Return()
		mockHub.EXPECT().Unregister(userID, gomock.Any()).DoAndReturn(func(uuid.UUID, *websocket.Conn) {
			close(unregistered)
		})
		ts := httptest.NewServer(router)
		defer ts.Close()
		u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/streams/ws/upgrade"

		conn, _, err := websocket.DefaultDialer.Dial(u, nil)
		if err != nil {
			t.Fatalf("failed to dial websocket: %v", err)
		}

		// Unregister runs in the handler's goroutine once the read loop ends,
		// so closing the socket is not enough: without waiting for the call
		// itself, gomock's cleanup can finish first and report the expectation
		// as unmet. That race is what made this test red on a few runs out of
		// a dozen.
		require.NoError(t, conn.Close())
		select {
		case <-unregistered:
		case <-time.After(5 * time.Second):
			t.Fatal("the handler never unregistered the closed connection")
		}
	})
}

func TestStreamHandler_ProcessFacesStream(t *testing.T) {
	streamID := uuid.New()

	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ProcessFacesStream(gomock.Any(), streamID).Return(nil)

		router := setupTestRouter()
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/faces", handlers.ProcessFacesStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/faces", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("stream not found returns 404", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ProcessFacesStream(gomock.Any(), streamID).Return(service.ErrStreamNotFound)

		router := setupTestRouter()
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/faces", handlers.ProcessFacesStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/faces", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["error"], "stream not found")
	})

	t.Run("source file missing returns 409", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ProcessFacesStream(gomock.Any(), streamID).Return(service.ErrSourceFileMissing)

		router := setupTestRouter()
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/faces", handlers.ProcessFacesStream)

		req := httptest.NewRequest("POST", fmt.Sprintf("/streams/%s/faces", streamID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("invalid stream id returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		router := setupTestRouter()
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/faces", handlers.ProcessFacesStream)

		req := httptest.NewRequest("POST", "/streams/not-a-uuid/faces", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestStreamHandler_ProcessFacesBatch(t *testing.T) {
	userID := uuid.New()
	streamID := uuid.New()

	setUser := func(claims *dto.AccessClaims) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("user", userID)
			if claims != nil {
				c.Set("claims", claims)
			}
			c.Next()
		}
	}

	t.Run("success returns processed and failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ProcessFacesBatch(gomock.Any(), userID, false, gomock.Any()).
			Return(&service.FacesBatchResult{
				Processed: []uuid.UUID{streamID},
				Failed: []service.FacesBatchFailure{
					{StreamID: uuid.New(), Reason: service.FacesBatchReasonForbidden},
				},
			}, nil)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/faces/batch", handlers.ProcessFacesBatch)

		req := httptest.NewRequest("POST", "/streams/faces/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.FacesBatchResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Len(t, resp.Processed, 1)
		assert.Len(t, resp.Failed, 1)
		assert.Equal(t, "forbidden", resp.Failed[0].Reason)
	})

	t.Run("missing user in context returns 500", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		router := setupTestRouter()
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/faces/batch", handlers.ProcessFacesBatch)

		req := httptest.NewRequest("POST", "/streams/faces/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("invalid body returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/faces/batch", handlers.ProcessFacesBatch)

		req := httptest.NewRequest("POST", "/streams/faces/batch", strings.NewReader(`{"ids":[]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("too many ids returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/faces/batch", handlers.ProcessFacesBatch)

		ids := make([]string, 0, request.MaxFacesBatchSize+1)
		for i := 0; i <= request.MaxFacesBatchSize; i++ {
			ids = append(ids, uuid.New().String())
		}
		body, _ := json.Marshal(map[string][]string{"ids": ids})

		req := httptest.NewRequest("POST", "/streams/faces/batch", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ProcessFacesBatch(gomock.Any(), userID, false, gomock.Any()).
			Return(nil, errors.New("boom"))

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/faces/batch", handlers.ProcessFacesBatch)

		req := httptest.NewRequest("POST", "/streams/faces/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestStreamHandler_ForceStreamError(t *testing.T) {
	userID := uuid.New()
	streamID := uuid.New()

	setUser := func(claims *dto.AccessClaims) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("user", userID)
			if claims != nil {
				c.Set("claims", claims)
			}
			c.Next()
		}
	}

	t.Run("success returns 200", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamError(gomock.Any(), streamID, userID, false).Return(nil)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/force-error", handlers.ForceStreamError)

		req := httptest.NewRequest("POST", "/streams/"+streamID.String()+"/force-error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("admin flag is passed through", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamError(gomock.Any(), streamID, userID, true).Return(nil)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "admin"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/force-error", handlers.ForceStreamError)

		req := httptest.NewRequest("POST", "/streams/"+streamID.String()+"/force-error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/force-error", handlers.ForceStreamError)

		req := httptest.NewRequest("POST", "/streams/not-a-uuid/force-error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not processing returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamError(gomock.Any(), streamID, userID, false).
			Return(service.ErrStreamNotProcessing)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/force-error", handlers.ForceStreamError)

		req := httptest.NewRequest("POST", "/streams/"+streamID.String()+"/force-error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamError(gomock.Any(), streamID, userID, false).
			Return(service.ErrStreamForbidden)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/force-error", handlers.ForceStreamError)

		req := httptest.NewRequest("POST", "/streams/"+streamID.String()+"/force-error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamError(gomock.Any(), streamID, userID, false).
			Return(service.ErrStreamNotFound)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/force-error", handlers.ForceStreamError)

		req := httptest.NewRequest("POST", "/streams/"+streamID.String()+"/force-error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unexpected error returns 500", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamError(gomock.Any(), streamID, userID, false).
			Return(errors.New("boom"))

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/:id/force-error", handlers.ForceStreamError)

		req := httptest.NewRequest("POST", "/streams/"+streamID.String()+"/force-error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestStreamHandler_ForceStreamErrorBatch(t *testing.T) {
	userID := uuid.New()
	streamID := uuid.New()

	setUser := func(claims *dto.AccessClaims) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("user", userID)
			if claims != nil {
				c.Set("claims", claims)
			}
			c.Next()
		}
	}

	t.Run("success returns processed and failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamErrorBatch(gomock.Any(), userID, false, gomock.Any()).
			Return(&service.StreamBatchResult{
				Processed: []uuid.UUID{streamID},
				Failed: []service.StreamBatchFailure{
					{StreamID: uuid.New(), Reason: service.StreamBatchReasonNotProcessing},
					{StreamID: uuid.New(), Reason: service.StreamBatchReasonInternal},
				},
			}, nil)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/force-error/batch", handlers.ForceStreamErrorBatch)

		req := httptest.NewRequest("POST", "/streams/force-error/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.ForceErrorBatchResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{streamID.String()}, resp.Processed)
		require.Len(t, resp.Failed, 2)
		assert.Equal(t, "not processing", resp.Failed[0].Reason)
		assert.Equal(t, "internal error", resp.Failed[1].Reason)
	})

	t.Run("invalid body returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/force-error/batch", handlers.ForceStreamErrorBatch)

		req := httptest.NewRequest("POST", "/streams/force-error/batch", strings.NewReader(`{"ids":[]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("duplicate ids return 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		id := streamID.String()
		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/force-error/batch", handlers.ForceStreamErrorBatch)

		req := httptest.NewRequest("POST", "/streams/force-error/batch", strings.NewReader(`{"ids":["`+id+`","`+id+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("too many ids return 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		ids := make([]string, 0, request.MaxForceErrorBatchSize+1)
		for i := 0; i <= request.MaxForceErrorBatchSize; i++ {
			ids = append(ids, uuid.New().String())
		}
		body, _ := json.Marshal(map[string][]string{"ids": ids})

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/force-error/batch", handlers.ForceStreamErrorBatch)

		req := httptest.NewRequest("POST", "/streams/force-error/batch", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ForceStreamErrorBatch(gomock.Any(), userID, false, gomock.Any()).
			Return(nil, errors.New("boom"))

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/force-error/batch", handlers.ForceStreamErrorBatch)

		req := httptest.NewRequest("POST", "/streams/force-error/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestStreamHandler_ReprocessStreamBatch(t *testing.T) {
	userID := uuid.New()
	streamID := uuid.New()

	setUser := func(claims *dto.AccessClaims) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("user", userID)
			if claims != nil {
				c.Set("claims", claims)
			}
			c.Next()
		}
	}

	t.Run("success returns processed and failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ReprocessStreamBatch(gomock.Any(), userID, false, gomock.Any()).
			Return(&service.StreamBatchResult{
				Processed: []uuid.UUID{streamID},
				Failed: []service.StreamBatchFailure{
					{StreamID: uuid.New(), Reason: service.StreamBatchReasonNotFailed},
					{StreamID: uuid.New(), Reason: service.StreamBatchReasonSourceMissing},
				},
			}, nil)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/reprocess/batch", handlers.ReprocessStreamBatch)

		req := httptest.NewRequest("POST", "/streams/reprocess/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp response.ReprocessBatchResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{streamID.String()}, resp.Processed)
		require.Len(t, resp.Failed, 2)
		assert.Equal(t, "not failed", resp.Failed[0].Reason)
		assert.Equal(t, "source removed", resp.Failed[1].Reason)
	})

	t.Run("admin flag reaches the service", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ReprocessStreamBatch(gomock.Any(), userID, true, gomock.Any()).
			Return(&service.StreamBatchResult{Processed: []uuid.UUID{streamID}}, nil)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "admin"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/reprocess/batch", handlers.ReprocessStreamBatch)

		req := httptest.NewRequest("POST", "/streams/reprocess/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("empty ids returns 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/reprocess/batch", handlers.ReprocessStreamBatch)

		req := httptest.NewRequest("POST", "/streams/reprocess/batch", strings.NewReader(`{"ids":[]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("duplicate ids return 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		id := streamID.String()
		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/reprocess/batch", handlers.ReprocessStreamBatch)

		req := httptest.NewRequest("POST", "/streams/reprocess/batch", strings.NewReader(`{"ids":["`+id+`","`+id+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("too many ids return 400", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)

		ids := make([]string, 0, request.MaxReprocessBatchSize+1)
		for i := 0; i <= request.MaxReprocessBatchSize; i++ {
			ids = append(ids, uuid.New().String())
		}
		body, err := json.Marshal(map[string][]string{"ids": ids})
		require.NoError(t, err)

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/reprocess/batch", handlers.ReprocessStreamBatch)

		req := httptest.NewRequest("POST", "/streams/reprocess/batch", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		mockService.EXPECT().ReprocessStreamBatch(gomock.Any(), userID, false, gomock.Any()).
			Return(nil, errors.New("boom"))

		router := setupTestRouter()
		router.Use(setUser(&dto.AccessClaims{Role: "member"}))
		handlers := NewStreamHandler(mockService, nil)
		router.POST("/streams/reprocess/batch", handlers.ReprocessStreamBatch)

		req := httptest.NewRequest("POST", "/streams/reprocess/batch", strings.NewReader(`{"ids":["`+streamID.String()+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
