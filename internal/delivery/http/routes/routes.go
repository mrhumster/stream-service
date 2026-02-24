package routes

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/delivery/http/handlers"
	"github.com/mrhumster/stream-service/internal/delivery/http/middleware"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/storage"
	"github.com/mrhumster/web-server-gin/pkg/auth"
	"gorm.io/gorm"
)

const (
	ModeTest    = "TEST"
	ModeDebug   = "DEBUG"
	ModeRelease = "RELEASE"
)

func SetupRoutes(db *gorm.DB, mode string, permissionClient auth.PermissionClient, storage storage.FileStorage) (*gin.Engine, error) {
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

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "https://example.com", "https://api.example.com"},
		AllowMethods:     []string{"GET", "PATCH", "POST", "OPTIONS", "PUT", "DELETE"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	if err != nil {
		return nil, fmt.Errorf("⚠️ Error setup routes: %w", err)
	}

	tokenService, err := service.NewTokenService(&cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("⚠️ Error setup routes: %w", err)
	}

	database := repository.NewGormStreamRepository(db)
	streamService := service.NewStreamServiceImpl(database, permissionClient, storage)
	streamHandler := handlers.NewStreamHandler(streamService)

	r.GET("/stream/health", func(c *gin.Context) {
		if _, err := db.DB(); err != nil {
			log.Println("⚠️ PG ERROR: ", err.Error())
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "up"})
	})

	r.GET("/stream", streamHandler.ListStreamPublic)
	r.GET("/stream/:id", streamHandler.GetStream)
	r.GET("/stream/:id/download", streamHandler.DownloadStream)

	auth := r.Group("/stream")
	auth.Use(middleware.AuthMiddleware(tokenService))
	auth.Use(middleware.Authorize("stream", "read", permissionClient))
	{
		auth.GET("/own", streamHandler.ListStreamOwner)
		auth.POST("/", middleware.Authorize("stream", "write", permissionClient), streamHandler.CreateStream)
		auth.PATCH("/:id", middleware.Authorize("stream", "write", permissionClient), streamHandler.UpdateStream)
		auth.DELETE("/:id", middleware.Authorize("stream", "delete", permissionClient), streamHandler.DeleteStream)
		auth.POST("/:id/upload", middleware.Authorize("stream", "write", permissionClient), streamHandler.UploadVideo)
	}
	return r, nil
}
