package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/request"
	"github.com/mrhumster/stream-service/internal/delivery/http/dto/response"
	"github.com/mrhumster/stream-service/internal/service"
)

type StreamHandler struct {
	service service.StreamService
}

func NewStreamHandler(service service.StreamService) *StreamHandler {
	return &StreamHandler{service: service}
}

func (h *StreamHandler) GetContent(c *gin.Context) {
	claims, exists := c.Get("claims")
	if !exists {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "claims not exists in context"})
	}
	c.JSON(http.StatusOK, gin.H{"response": fmt.Sprintf("Claims %#v", claims)})
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
