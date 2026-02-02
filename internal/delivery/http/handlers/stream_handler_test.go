package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.Default()
}

func TestStreamHandler_GetStream(t *testing.T) {
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

	t.Run("getting stream only auth users", func(t *testing.T) {
		r1 := setupTestRouter()
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		handler1 := NewStreamHandler(mockService)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", uuid.New()), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid user ID type", func(t *testing.T) {
		r1 := setupTestRouter()
		r1.Use(func(c *gin.Context) {
			c.Set("userID", 12345)
			c.Next()
		})
		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		mockService := servicemock.NewMockStreamService(ctrl1)
		handler1 := NewStreamHandler(mockService)
		r1.GET("/streams/:id", handler1.GetStream)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", uuid.New()), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("propagation service error", func(t *testing.T) {
		r1 := setupTestRouter()
		r1.Use(func(c *gin.Context) {
			c.Set("userID", uuid.New().String())
			c.Next()
		})

		ctrl1 := gomock.NewController(t)
		defer ctrl1.Finish()
		expectedError := "expected error"
		mockService := servicemock.NewMockStreamService(ctrl1)
		mockService.EXPECT().GetStream(gomock.Any(), gomock.Any()).Return(nil, errors.New(expectedError))
		handler1 := NewStreamHandler(mockService)
		r1.GET("/streams/:id", handler1.GetStream)
		streamID := uuid.New()
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s", streamID), nil)
		w := httptest.NewRecorder()
		r1.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Contains(t, resp["error"], expectedError)

	})

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

	t.Run("propagation validation error", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "non-valid-uuid")
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		expectedError := "UUID"

		handler := NewStreamHandler(mockService)
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

		assert.Contains(t, resp["error"], expectedError)

	})
	t.Run("propagation service error", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "12345678-1234-5678-0000-000000000000")
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		expectedError := "title cannot be empty"
		mockService.EXPECT().CreateStream(gomock.Any(), gomock.Any()).Return(nil, errors.New(expectedError))

		handler := NewStreamHandler(mockService)
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

		assert.Contains(t, resp["error"], expectedError)

	})

	t.Run("error bind json", func(t *testing.T) {
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
			c.Set("userID", "12345678-1234-5678-0000-000000000000")
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
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
		handler := NewStreamHandler(mockService)
		router.POST("/streams", handler.CreateStream)
		reqBody := `{"Title": "Test Stream", "Visibility": "public"}`
		req := httptest.NewRequest("POST", "/streams", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("bad user ID type", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", 12345)
			c.Next()
		})
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
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

func TestStreamHandler_UpdateStream(t *testing.T) {
	t.Run("Update stream successfull", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
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

	t.Run("Update stream only auth user", func(t *testing.T) {
		router := setupTestRouter()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.PATCH("/streams/:id", handler.UpdateStream)
		reqBody := `{"Title": "Updated title", "Visibility": "public", "Description": "Updated description"}`
		existiongStream := uuid.New()
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Validation error", func(t *testing.T) {

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.PATCH("/streams/:id", handler.UpdateStream)
		existiongStream := uuid.New()
		reqBody := `{"Title": "", "Visibility": "public", "Description": "Updated description"}`
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad user ID type", func(t *testing.T) {

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", 123456)
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.PATCH("/streams/:id", handler.UpdateStream)
		existiongStream := uuid.New()
		reqBody := `{"Title": "", "Visibility": "public", "Description": "Updated description"}`
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("bad stream ID type", func(t *testing.T) {

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
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
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.PATCH("/streams/:id", handler.UpdateStream)
		existiongStream := uuid.New()
		reqBody := `{"Title": "", "Visibility": public, "Description": "Updated description"}`
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("service returns validation error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)

		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "test-user-uuid")
			c.Next()
		})
		router.PATCH("/streams/:id", handler.UpdateStream)

		streamID := uuid.New()

		expectedError := "title cannot be empty"
		mockService.EXPECT().
			UpdateStream(gomock.Any(), streamID, gomock.Any()).
			Return(nil, errors.New(expectedError))

		reqBody := `{"title": "Valid Title"}`
		req := httptest.NewRequest("PATCH", "/streams/"+streamID.String(),
			bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Contains(t, resp["error"], expectedError)
	})
}

func TestStreamHandler_DeleteStream(t *testing.T) {
	t.Run("delete succesfull", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})
		handlers := NewStreamHandler(mockService)
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
		router.Use(func(c *gin.Context) {
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})
		handlers := NewStreamHandler(mockService)
		router.DELETE("/streams/:id", handlers.DeleteStream)
		existingStreamID := "invalid-uuid-struct"
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/streams/%s", existingStreamID), nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["error"], "invalid stream ID in param")
	})

	t.Run("Only authenticated users can use deletion", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		r := setupTestRouter()
		h := NewStreamHandler(mockService)
		r.DELETE("/streams/:id", h.DeleteStream)
		streamID := uuid.New()
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/streams/%s", streamID), nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Contains(t, resp["error"], "user not auth")
	})
	t.Run("Bad user ID type", func(t *testing.T) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", 123456)
			c.Next()
		})

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.DELETE("/streams/:id", handler.DeleteStream)
		existiongStream := uuid.New()
		reqBody := `{"Title": "", "Visibility": "public", "Description": "Updated description"}`
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/streams/%s", existiongStream.String()), bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestStreamHandler_StartStreamUpload(t *testing.T) {
	t.Run("success case", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockService := servicemock.NewMockStreamService(ctrl)
		handlers := NewStreamHandler(mockService)
		userID := uuid.New().String()
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", userID)
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
		handler := NewStreamHandler(mockService)
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", uuid.New().String())
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
		handlers := NewStreamHandler(mockService)
		userID := uuid.New()
		streamID := uuid.New()
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", userID.String())
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
		assert.Contains(t, w.Body.String(), "stream not found")
	})
}

func TestStreamHandler_DownloadStream(t *testing.T) {
	setupTest := func() (*gin.Engine, *servicemock.MockStreamService, *StreamHandler) {
		router := setupTestRouter()
		router.Use(func(c *gin.Context) {
			c.Set("userID", "00000000-0000-0000-0000-000000000000")
			c.Next()
		})
		ctrl := gomock.NewController(t)
		mockService := servicemock.NewMockStreamService(ctrl)
		handler := NewStreamHandler(mockService)
		router.GET("/streams/:id/download", handler.DownloadStream)
		return router, mockService, handler
	}
	t.Run("successfull download request", func(t *testing.T) {
		router, mockService, _ := setupTest()
		streamID := uuid.New()
		expectedURL := "https://storage.example.com/streams/...?signature=..."
		expiresAt := time.Now().Add(1 * time.Hour)
		mockService.EXPECT().GenerateDownloadURL(gomock.Any(), streamID).Return(expectedURL, expiresAt, nil)
		req := httptest.NewRequest("GET", fmt.Sprintf("/streams/%s/download", streamID), nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp response.DownloadResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, expectedURL, resp.URL)
		assert.Equal(t, expiresAt.Format(time.RFC3339), resp.ExpiresAt.Format(time.RFC3339))
	})
}
