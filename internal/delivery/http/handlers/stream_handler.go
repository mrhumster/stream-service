package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mrhumster/stream-service/internal/service"
)

type StreamHandler struct {
	service *service.StreamService
}

func NewStreamHandler(service *service.StreamService) *StreamHandler {
	return &StreamHandler{
		service: service,
	}
}

func (h *StreamHandler) GetContent(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"response": "ok"})
}
