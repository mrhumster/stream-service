package routes

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/delivery/http/handlers"
	"github.com/mrhumster/stream-service/internal/delivery/http/middleware"
	"github.com/mrhumster/stream-service/internal/service"
	"gorm.io/gorm"
)

const (
	ModeTest    = "TEST"
	ModeDebug   = "DEBUG"
	ModeRelease = "RELEASE"
)

func SetupRoutes(db *gorm.DB, mode string) (*gin.Engine, error) {
	var (
		cfg *config.Config
		err error
	)

	r := gin.Default()
	switch mode {
	case ModeTest:
		gin.SetMode(gin.TestMode)
		cfg, err = config.TestConfig()
	case ModeDebug:
		gin.SetMode(gin.DebugMode)
		cfg, err = config.TestConfig()
	case ModeRelease:
		gin.SetMode(gin.ReleaseMode)
		cfg, err = config.LoadConfig()
	default:
		gin.SetMode(gin.DebugMode)
		cfg, err = config.LoadConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("⚠️ Error setup routes: %w", err)
	}

	tokenService, err := service.NewTokenService(&cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("⚠️ Error setup routes: %w", err)
	}

	streamService, err := service.NewStreamService("my stream service")
	streamHandler := handlers.NewStreamHandler(streamService)

	r.GET("/stream/health", func(c *gin.Context) {
		if _, err := db.DB(); err != nil {
			log.Println("⚠️ PG ERROR: ", err.Error())
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "up"})
	})

	auth := r.Group("/stream")
	auth.Use(middleware.AuthMiddleware(tokenService))
	auth.Use(middleware.Authorize("stream", "read"))
	{
		auth.GET("/api/content", streamHandler.GetContent)
	}
	return r, nil
}
