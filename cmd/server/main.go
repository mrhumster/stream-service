package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrhumster/identity-service/pkg/auth"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/database"
	"github.com/mrhumster/stream-service/internal/delivery/http/routes"
	"github.com/mrhumster/stream-service/internal/storage"
)

func main() {
	opts := &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))

	slog.SetDefault(logger)
	slog.Info("Start Stream service", "version", "v0.1.0")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌  Error load config: %v", err)
	}

	fileMinIOStorage, err := storage.NewMinIOStorageFromConfig(cfg.MinIO)
	if err != nil {
		log.Fatalf("❌  Error create file storage: %v", err)
	}

	db, err := database.SetupDatabase(cfg)
	if err != nil {
		log.Fatalf("❌ Error open database: %v", err)
	}
	mode := os.Getenv("MODE")

	permissionClient, err := auth.NewPermissionClient(cfg.Server.AuthServiceAddr)
	if err != nil {
		log.Fatalf("❌ Permission gRPC client: %v", err)
	}

	r, err := routes.SetupRoutes(db, mode, permissionClient, fileMinIOStorage)
	if err != nil {
		log.Fatalf("❌ Error gin route: %v", err)
	}

	httpErr := make(chan error, 1)

	defer func() {
		log.Println("🟡 Closing database pool...")
		sqlDB, err := db.DB()
		if err != nil {
			log.Printf("failed to get sql.DB: %s", err.Error())
		}
		if err := sqlDB.Close(); err != nil {
			log.Println("🟢 Database pool closed")
		}
	}()

	srv := &http.Server{
		Addr:         cfg.Server.ServerAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		log.Printf("🚀 Server starting on %s\n", cfg.Server.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("🔴 Server error: ", err)
			httpErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🟡 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("🔴 Server shutdown error: ", err)
	}

	log.Println("🟢 Server stoped")
}
