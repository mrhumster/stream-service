package handlers

import (
	"fmt"
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

func (h *StreamHandler) ListStreamOwner(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)

	var filter repository.StreamFilter
	streams, total, err := h.service.ListUserStreams(c.Request.Context(), userUUID)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}
	var resp response.ListReponse[response.StreamResponse]
	for _, s := range streams {
		resp.Items = append(resp.Items, response.FromDomainModel(s))
	}
	resp.Total = total
	resp.Limit = filter.Limit
	resp.Offset = filter.Offset

	c.JSON(http.StatusOK, resp)
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
	userUUID := c.MustGet("user").(uuid.UUID)

	var req request.CreateStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	serviceReq, err := req.ToServiceRequest(userUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	stream, err := h.service.CreateStream(c.Request.Context(), *serviceReq)
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
		_, ok := c.MustGet("user").(uuid.UUID)

		if !ok {
			c.JSON(http.StatusInternalServerError, response.ErrorResponse("invalid user ID in context"))
			return
		}

	}

	resp := response.FromDomainModel(stream)
	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) UpdateStream(c *gin.Context) {
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
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

	updatedStream, err := h.service.UpdateStream(c.Request.Context(), streamUUID, *updateRequest)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	resp := response.FromDomainModel(updatedStream)

	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) DeleteStream(c *gin.Context) {
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
	userUUID := c.MustGet("user").(uuid.UUID)
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
		UserID:     userUUID,
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

func (h *StreamHandler) InitUpload(c *gin.Context) {
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	var req request.StartUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		h.handleDownloadError(c, err)
		return
	}

	userUUID := c.MustGet("user").(uuid.UUID)

	serviceReq := req.ToService(streamUUID, userUUID)

	uploadInfo, err := h.service.StartStreamUpload(
		c.Request.Context(),
		*serviceReq,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	resp := response.StartUploadResponse{
		StreamID: uploadInfo.StreamID.String(),
		UploadID: uploadInfo.UploadID,
	}
	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) PartUpload(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}
	var req request.UploadPartRequest
	if err = c.ShouldBind(&req); err != nil {
		fmt.Printf("Binding error: %v\n", err)
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}
	reqService, err := req.ToService(streamUUID, userUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	part, err := h.service.UploadPart(c.Request.Context(), *reqService)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}
	resp := response.PartUploadResponse{
		PartNumber: part.PartNumber,
		ETag:       part.ETag,
	}
	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) CompleteUpload(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	var req request.CompleteUploadRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}

	reqService, err := req.ToService(streamUUID, userUUID)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}
	if err := h.service.CompleteStreamUpload(c.Request.Context(), *reqService); err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *StreamHandler) GetHLS(c *gin.Context) {
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse(err.Error()))
		return
	}
	fileName := c.Param("file")
	if fileName == "" || fileName == "/" {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("filename cannot be empty"))
		return
	}
	req := &service.GetFileByKeyRequest{
		StreamUUID: streamUUID,
		FileName:   fileName,
	}
	res, err := h.service.GetFileByKey(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse(err.Error()))
		return
	}

	if res != nil {
		defer res.Content.Close()
	}
	c.DataFromReader(http.StatusOK, res.Size, res.ContentType, res.Content, nil)
}
