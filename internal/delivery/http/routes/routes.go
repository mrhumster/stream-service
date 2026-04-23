package routes

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/mrhumster/identity-service/pkg/auth"
	"github.com/mrhumster/identity-service/pkg/middleware"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/delivery/http/handlers"
	"github.com/mrhumster/stream-service/internal/queue"
	"github.com/mrhumster/stream-service/internal/repository"
	"github.com/mrhumster/stream-service/internal/service"
	"github.com/mrhumster/stream-service/internal/storage"
	"github.com/mrhumster/stream-service/internal/wss"
	"gorm.io/gorm"
)

func SetupRoutes(db *gorm.DB, mode config.ServerMode, permissionClient auth.PermissionClient, storage storage.FileStorage) (*gin.Engine, service.StreamService, error) {
	var (
		cfg *config.Config
		err error
	)

	r := gin.New()
	r.Use(middleware.StructuredLog())
	r.Use(gin.Recovery())

	switch mode {
	case config.Test:
		gin.SetMode(gin.TestMode)
		cfg, err = config.TestConfig()
	case config.Debug:
		gin.SetMode(gin.DebugMode)
		cfg, err = config.TestConfig()
	case config.Release:
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
		return nil, nil, fmt.Errorf("⚠️ Error setup routes: %w", err)
	}

	tokenService, err := service.NewTokenService(&cfg.JWT)
	if err != nil {
		return nil, nil, fmt.Errorf("⚠️ Error setup routes: %w", err)
	}

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       2,
	}

	hub := wss.NewWssHub()
	database := repository.NewGormStreamRepository(db)
	asyncDistributor := queue.NewAsyncDistributor(redisOpt)
	streamService := service.NewStreamServiceImpl(database, permissionClient, storage, asyncDistributor, hub, &cfg.Server)
	streamHandler := handlers.NewStreamHandler(streamService, hub)

	r.GET("/stream/health", func(c *gin.Context) {
		if _, err := db.DB(); err != nil {
			log.Println("⚠️ PG ERROR: ", err.Error())
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "up"})
	})

	r.GET("/stream", streamHandler.ListStreamPublic)
	r.GET("/stream/:id", middleware.OptionalAuthMiddleware(tokenService), streamHandler.GetStream)
	r.GET("/stream/:id/download", middleware.OptionalAuthMiddleware(tokenService), streamHandler.DownloadStream)
	r.GET("/stream/:id/hls/*file", middleware.OptionalAuthMiddleware(tokenService), streamHandler.GetHLS)
	r.GET("/stream/ws/updates", middleware.AuthMiddleware(tokenService), streamHandler.HandleWS)

	authGroup := r.Group("/stream")
	authGroup.Use(middleware.AuthMiddleware(tokenService))
	authGroup.Use(middleware.Authorize(permissionClient, "stream", "read"))
	{
		authGroup.GET("/own", streamHandler.ListStreamOwner)
		authGroup.POST("/", streamHandler.CreateStream)
		authGroup.PATCH("/:id", middleware.Authorize(permissionClient, "stream", "write"), streamHandler.UpdateStream)
		authGroup.DELETE("/:id", middleware.Authorize(permissionClient, "stream", "delete"), streamHandler.DeleteStream)
		authGroup.POST("/:id/publish", middleware.Authorize(permissionClient, "stream", "write"), streamHandler.PublishStream)
		authGroup.POST("/:id/unpublish", middleware.Authorize(permissionClient, "stream", "write"), streamHandler.UnpublishStream)
		authGroup.POST("/:id/upload", middleware.Authorize(permissionClient, "stream", "write"), streamHandler.UploadVideo)
		authGroup.POST("/:id/upload/init", middleware.Authorize(permissionClient, "stream", "write"), streamHandler.InitUpload)
		authGroup.PUT("/:id/upload/part", middleware.Authorize(permissionClient, "stream", "write"), streamHandler.PartUpload)
		authGroup.POST("/:id/upload/complete", middleware.Authorize(permissionClient, "stream", "write"), streamHandler.CompleteUpload)
	}
	return r, streamService, nil
}
