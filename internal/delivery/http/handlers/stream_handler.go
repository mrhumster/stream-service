package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/request"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
)

type StreamHandler struct {
	service service.StreamService
}

func NewStreamHandler(service service.StreamService) *StreamHandler {
	return &StreamHandler{service: service}
}

func (h *StreamHandler) ListStreamPublic(c *gin.Context) {
	pub := models.VisibilityPublic
	limit := c.Query("limit")
	offset := c.Query("offset")
	filter := repository.StreamFilter{
		Visibility: &pub,
		Offset:     0,
		Limit:      10,
	}
	if limit != "" {
		limitInt, err := strconv.Atoi(limit)
		if err != nil {
			c.JSON(http.StatusBadRequest, response.ErrorResponse("not valid limit query"))
			return
		}
		filter.Limit = limitInt
	}

	if offset != "" {
		offsetInt, err := strconv.Atoi(offset)
		if err != nil {
			c.JSON(http.StatusBadRequest, response.ErrorResponse("not valid offset query"))
			return
		}
		filter.Offset = offsetInt
	}

	streams, total, err := h.service.ListStreams(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("error getting stream list from service"))
		return
	}

	streamRespList := make([]response.StreamResponse, 0, len(streams))

	for _, v := range streams {
		streamRespList = append(streamRespList, response.FromDomainModel(v))
	}

	streamsList := response.ListReponse[response.StreamResponse]{
		Items:  streamRespList,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	c.JSON(http.StatusOK, streamsList)
}

func (h *StreamHandler) CreateStream(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, response.ErrorResponse("user not authenticated"))
		return
	}

	userIDStr, ok := userID.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid user ID in context"))
		return
	}

	var req request.CreateStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	serviceReq, err := req.ToServiceRequest(userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	stream, err := h.service.CreateStream(c.Request.Context(), serviceReq)
	if err != nil {
		// TODO: Реализовать разделение ошибок (validation, not found, internal)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	resp := response.FromDomainModel(stream)
	c.JSON(http.StatusCreated, resp)
}

func (h *StreamHandler) GetStream(c *gin.Context) {
	streamID := c.Param("id")

	streamIDuuid, err := uuid.Parse(streamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid stream ID in params"))
		return
	}

	stream, err := h.service.GetStream(c.Request.Context(), streamIDuuid)

	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	if stream.Visibility != models.VisibilityPublic {
		userID, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, response.ErrorResponse("user not authenticated"))
			return
		}

		_, ok := userID.(string)

		if !ok {
			c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid user ID in context"))
			return
		}

	}

	resp := response.FromDomainModel(stream)
	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) UpdateStream(c *gin.Context) {
	userID, exists := c.Get("userID")

	if !exists {
		c.JSON(http.StatusUnauthorized, response.ErrorResponse("user not authenticated"))
		return
	}

	_, ok := userID.(string)

	if !ok {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid user ID in context"))
		return
	}

	paramID := c.Param("id")

	streamID, err := uuid.Parse(paramID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid stream ID in params"))
		return
	}

	var req request.UpdateStreamRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	updateRequest, err := req.ToServiceRequest()

	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	updatedStream, err := h.service.UpdateStream(c.Request.Context(), streamID, updateRequest)

	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	resp := response.FromDomainModel(updatedStream)

	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) DeleteStream(c *gin.Context) {
	userID, exists := c.Get("userID")

	if !exists {
		c.JSON(http.StatusUnauthorized, response.ErrorResponse("user not authenticated"))
		return
	}

	_, ok := userID.(string)

	if !ok {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid user ID in context"))
		return
	}

	param := c.Param("id")

	streamID, err := uuid.Parse(param)

	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid stream ID in params"))
		return
	}

	h.service.DeleteStream(c.Request.Context(), streamID)
	c.JSON(http.StatusOK, nil)
}

func (h *StreamHandler) UploadVideo(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, response.ErrorResponse("user not authenticated"))
		return
	}
	userIDStr, ok := userID.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid user ID"))
		return
	}
	streamID := c.Param("id")
	streamUUID, err := uuid.Parse(streamID)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream id"))
		return
	}
	file, fileHeader, err := c.Request.FormFile("video")
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("video file required"))
		return
	}
	defer file.Close()

	uploadReq := &request.VideoUploadRequest{
		StreamID:   streamUUID,
		UserID:     userIDStr,
		File:       file,
		FileHeader: fileHeader,
	}
	serviceReq, err := uploadReq.ToServiceRequest()
	if err != nil {
		if _, ok := err.(*request.ValidationError); ok {
			c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		} else {
			c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		}
		return
	}
	err = h.service.UploadVideo(c.Request.Context(), *serviceReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "video uploaded successfully",
	})
}

func (h *StreamHandler) DownloadStream(c *gin.Context) {
	streamID := c.Param("id")
	streamUUID, err := uuid.Parse(streamID)

	if err != nil {
		h.handleDownloadError(c, err)
		return
	}

	serviceResp, err := h.service.GenerateDownloadURL(c, streamUUID)
	if err != nil {
		h.handleDownloadError(c, err)
		return
	}

	resp, err := response.NewDownloadResponse(serviceResp)

	if err != nil {
		h.handleDownloadError(c, err)
		return
	}

	directDownload := c.Query("direct") == "true"
	if directDownload {
		c.Redirect(http.StatusTemporaryRedirect, resp.URL)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) handleDownloadError(c *gin.Context, err error) {
	errorMsg := err.Error()

	switch {
	case strings.Contains(errorMsg, "not found"):
		c.JSON(http.StatusNotFound, response.ErrorResponse("stream not found"))
	case strings.Contains(errorMsg, "not ready"):
		c.JSON(http.StatusBadRequest, response.ErrorResponse("stream not available for download"))
	default:
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("failed to generate download link"))
	}

}
