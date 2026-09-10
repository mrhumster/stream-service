package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/request"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	streammetrics "github.com/mrhumster/stream-service/internal/metrics"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/wss"
)

type StreamHandler struct {
	service      service.StreamService
	hub          wss.Hub
	allowedOrigs map[string]bool
}

func NewStreamHandler(service service.StreamService, hub wss.Hub) *StreamHandler {
	return NewStreamHandlerWithOrigins(service, hub, nil)
}

func NewStreamHandlerWithOrigins(service service.StreamService, hub wss.Hub, origins []string) *StreamHandler {
	if len(origins) == 0 {
		origins = []string{"http://localhost:5173", "https://example.com", "https://api.example.com"}
	}
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return &StreamHandler{
		service:      service,
		hub:          hub,
		allowedOrigs: allowed,
	}
}

func (h *StreamHandler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return h.allowedOrigs[origin]
}

func (h *StreamHandler) HandleWS(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)
	slog.Debug("Auth HANDLE WS", "user", userUUID)
	upgrader := websocket.Upgrader{
		CheckOrigin: h.checkOrigin,
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, http.Header{
		"Sec-Websocket-Protocol": {c.GetHeader("Sec-Websocket-Protocol")},
	})
	if err != nil {
		slog.Error("Upgrade connection", "error", err)
		return
	}
	if h.hub != nil {
		h.hub.Register(userUUID, conn)
		defer func() {
			h.hub.Unregister(userUUID, conn)
			conn.Close()
		}()

	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *StreamHandler) ListStreamOwner(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)

	var filter repository.StreamFilter
	streams, total, err := h.service.ListUserStreams(c.Request.Context(), userUUID)
	if err != nil {
		slog.Error("list user streams failed", "error", err)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
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
	published := models.StatusPublished
	limit := c.Query("limit")
	offset := c.Query("offset")
	filter := repository.StreamFilter{
		Visibility: &pub,
		Offset:     0,
		Limit:      10,
		Status:     &published,
	}
	if limit != "" {
		limitInt, err := strconv.Atoi(limit)
		if err != nil || limitInt < 0 {
			c.JSON(http.StatusBadRequest, response.ErrorResponse("not valid limit query"))
			return
		}
		if limitInt > 100 {
			c.JSON(http.StatusBadRequest, response.ErrorResponse("limit must be 100 or less"))
			return
		}
		filter.Limit = limitInt
	}

	if offset != "" {
		offsetInt, err := strconv.Atoi(offset)
		if err != nil || offsetInt < 0 {
			c.JSON(http.StatusBadRequest, response.ErrorResponse("not valid offset query"))
			return
		}
		if offsetInt > 1000 {
			c.JSON(http.StatusBadRequest, response.ErrorResponse("offset must be 1000 or less"))
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
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid request body"))
		return
	}

	serviceReq, err := req.ToServiceRequest(userUUID)
	if err != nil {
		slog.Error("create stream request conversion failed", "error", err)
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream request"))
		return
	}

	stream, err := h.service.CreateStream(c.Request.Context(), *serviceReq)
	if err != nil {
		slog.Error("create stream failed", "error", err)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		return
	}

	streammetrics.Lifecycle.WithLabelValues("created").Inc()
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
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, response.ErrorResponse("stream not found"))
		} else {
			slog.Error("get stream failed", "error", err)
			c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		}
		return
	}

	if stream.Visibility != models.VisibilityPublic {
		user, ok := c.MustGet("user").(uuid.UUID)

		if !ok || user == uuid.Nil || user != stream.OwnerID {
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
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid request body"))
		return
	}

	updateRequest, err := req.ToServiceRequest()
	if err != nil {
		slog.Error("update stream request conversion failed", "error", err)
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream request"))
		return
	}

	updatedStream, err := h.service.UpdateStream(c.Request.Context(), streamUUID, *updateRequest)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, response.ErrorResponse("stream not found"))
		} else {
			slog.Error("update stream failed", "error", err)
			c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		}
		return
	}

	resp := response.FromDomainModel(updatedStream)

	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) DeleteStream(c *gin.Context) {
	param := c.Param("id")
	streamID, err := uuid.Parse(param)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream ID in params"))
		return
	}

	if err := h.service.DeleteStream(c.Request.Context(), streamID); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "stream not found"):
			c.JSON(http.StatusNotFound, response.ErrorResponse(msg))
		case strings.Contains(msg, "cannot delete published stream"):
			c.JSON(http.StatusBadRequest, response.ErrorResponse(msg))
		default:
			slog.Error("delete stream", "error", msg)
			c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		}
		return
	}

	streammetrics.Lifecycle.WithLabelValues("deleted").Inc()
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
			slog.Error("upload request conversion failed", "error", err)
			c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		}
		return
	}
	err = h.service.UploadVideo(c.Request.Context(), *serviceReq)
	if err != nil {
		slog.Error("upload video failed", "error", err)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		return
	}
	streammetrics.Uploads.WithLabelValues("simple").Inc()
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "video uploaded successfully",
	})
}

func (h *StreamHandler) DownloadStream(c *gin.Context) {
	streamID := c.Param("id")
	streamUUID, err := uuid.Parse(streamID)
	userUUID := c.MustGet("user").(uuid.UUID)
	if err != nil {
		h.handleDownloadError(c, err)
		return
	}

	serviceResp, err := h.service.GenerateDownloadURL(c, streamUUID, userUUID)
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
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream id"))
		return
	}

	var req request.StartUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid request body"))
		return
	}

	userUUID := c.MustGet("user").(uuid.UUID)

	serviceReq := req.ToService(streamUUID, userUUID)

	uploadInfo, err := h.service.StartStreamUpload(
		c.Request.Context(),
		*serviceReq,
	)
	if err != nil {
		slog.Error("start stream upload failed", "error", err)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		return
	}

	resp := response.StartUploadResponse{
		StreamID: uploadInfo.StreamID.String(),
		UploadID: uploadInfo.UploadID,
	}
	streammetrics.Uploads.WithLabelValues("init").Inc()
	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) PartUpload(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream id"))
		return
	}
	var req request.UploadPartRequest
	if err = c.ShouldBind(&req); err != nil {
		slog.Error("bind part upload request failed", "error", err)
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid request body"))
		return
	}
	reqService, err := req.ToService(streamUUID, userUUID)
	if err != nil {
		slog.Error("part upload request conversion failed", "error", err)
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid part upload request"))
		return
	}

	part, err := h.service.UploadPart(c.Request.Context(), *reqService)
	if err != nil {
		slog.Error("upload part failed", "error", err)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		return
	}
	resp := response.PartUploadResponse{
		PartNumber: part.PartNumber,
		ETag:       part.ETag,
	}
	streammetrics.Uploads.WithLabelValues("part").Inc()
	c.JSON(http.StatusOK, resp)
}

func (h *StreamHandler) CompleteUpload(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream id"))
		return
	}

	var req request.CompleteUploadRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid request body"))
		return
	}

	reqService, err := req.ToService(streamUUID, userUUID)
	if err != nil {
		slog.Error("complete upload request conversion failed", "error", err)
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid complete upload request"))
		return
	}
	if err := h.service.CompleteStreamUpload(c.Request.Context(), *reqService); err != nil {
		slog.Error("complete stream upload failed", "error", err)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		return
	}

	streammetrics.Uploads.WithLabelValues("complete").Inc()
	c.Status(http.StatusNoContent)
}

func (h *StreamHandler) GetHLS(c *gin.Context) {
	val := c.Param("id")
	streamUUID, err := uuid.Parse(val)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream id"))
		return
	}

	stream, err := h.service.GetStream(c.Request.Context(), streamUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, response.ErrorResponse("stream not found"))
		return
	}

	if stream.Status != models.StatusPublished {
		user, exist := c.Get("user")
		if !exist || user.(uuid.UUID) != stream.OwnerID {
			streammetrics.HLSRequests.WithLabelValues("403").Inc()
			c.JSON(http.StatusForbidden, response.ErrorResponse("this stream is private or not ready "))
			return
		}
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
		switch {
		case strings.Contains(err.Error(), "invalid file name"):
			c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid file name"))
		case strings.Contains(err.Error(), "can't watch"):
			c.JSON(http.StatusForbidden, response.ErrorResponse("stream is not available for watching"))
		default:
			slog.Error("get file by key failed", "error", err)
			c.JSON(http.StatusInternalServerError, response.ErrorResponse("internal server error"))
		}
		return
	}

	if res != nil {
		defer res.Content.Close()
	}
	streammetrics.HLSRequests.WithLabelValues("200").Inc()
	c.DataFromReader(http.StatusOK, res.Size, res.ContentType, res.Content, nil)
}

func (h *StreamHandler) PublishStream(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)
	streamID := c.Param("id")
	streamUUID, err := uuid.Parse(streamID)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream id"))
		return
	}
	stream, err := h.service.GetStream(c.Request.Context(), streamUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, response.ErrorResponse("stream not found"))
		return
	}
	if stream.OwnerID != userUUID {
		c.JSON(http.StatusForbidden, response.ErrorResponse("only owner can published that stream"))
		return
	}
	if err := h.service.PublishStream(c.Request.Context(), streamUUID); err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("service error"))
		return
	}
	streammetrics.Lifecycle.WithLabelValues("published").Inc()
	c.Status(http.StatusCreated)
}

func (h *StreamHandler) UnpublishStream(c *gin.Context) {
	userUUID := c.MustGet("user").(uuid.UUID)
	streamID := c.Param("id")
	streamUUID, err := uuid.Parse(streamID)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.ErrorResponse("invalid stream id"))
		return
	}
	stream, err := h.service.GetStream(c.Request.Context(), streamUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, response.ErrorResponse("stream not found"))
		return
	}
	if stream.OwnerID != userUUID {
		c.JSON(http.StatusForbidden, response.ErrorResponse("only owner can unpublished that stream"))
		return
	}
	if err := h.service.UnpublishStream(c.Request.Context(), streamUUID); err != nil {
		c.JSON(http.StatusInternalServerError, response.ErrorResponse("service error"))
		return
	}
	streammetrics.Lifecycle.WithLabelValues("unpublished").Inc()
	c.Status(http.StatusCreated)
}
