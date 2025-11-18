package handlers

import (
	"fmt"
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
	claims, exists := c.Get("claims")
	if !exists {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "claims not exists in context"})
	}
	c.JSON(http.StatusOK, gin.H{"response": fmt.Sprintf("Claims %#v", claims)})
}
