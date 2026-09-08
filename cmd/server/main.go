package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrhumster/identity-service/pkg/auth"
	"github.com/mrhumster/identity-service/pkg/grpctls"
	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/gen/go/stream"
	"github.com/mrhumster/stream-service/internal/database"
	grpcHandle "github.com/mrhumster/stream-service/internal/delivery/grpc"
	"github.com/mrhumster/stream-service/internal/delivery/http/routes"
	"github.com/mrhumster/stream-service/internal/storage"
	"google.golang.org/grpc"
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

	if !cfg.Server.KeepOriginalFile {
		log.Printf("⚠️ ORIGINAL FILE AFTER TRANSCODING WILL BE DELETED")
	}

	fileMinIOStorage, err := storage.NewMinIOStorageFromConfig(cfg.MinIO)
	if err != nil {
		log.Fatalf("❌  Error create file storage: %v", err)
	}

	db, err := database.SetupDatabase(cfg)
	if err != nil {
		log.Fatalf("❌ Error open database: %v", err)
	}

	permissionClient, err := newPermissionClient(cfg)
	if err != nil {
		log.Fatalf("❌ Permission gRPC client: %v", err)
	}

	r, svc, err := routes.SetupRoutes(db, cfg.Server.Mode, permissionClient, fileMinIOStorage)
	if err != nil {
		log.Fatalf("❌ Error gin route: %v", err)
	}

	httpErr := make(chan error, 1)
	grpcErr := make(chan error, 1)

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

	httpServer := &http.Server{
		Addr:         cfg.Server.ServerAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	grpcHandle := grpcHandle.NewStreamGRPCServer(svc)

	grpcOpts := []grpc.ServerOption{}
	if cfg.Server.GRPCTLSEnabled {
		serverCreds, cerr := grpctls.ServerTLSCreds(cfg.Server.GRPCTLSCertFile, cfg.Server.GRPCTLSKeyFile, cfg.Server.GRPCTLSCAFile)
		if cerr != nil {
			log.Fatalf("🔴 Failed to load gRPC server TLS: %v", cerr)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(serverCreds))
		if len(cfg.Server.GRPCTLSAllowedOUs) > 0 {
			grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(grpctls.AllowOUsInterceptor(cfg.Server.GRPCTLSAllowedOUs...)))
		}
	}

	grpcServer := grpc.NewServer(grpcOpts...)
	stream.RegisterStreamServiceServer(grpcServer, grpcHandle)

	go func() {
		// HTTP serve
		log.Printf("🚀 Server starting on %s\n", cfg.Server.ServerAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("🔴 Server error: ", err)
			httpErr <- err
		}
	}()

	go func() {
		// gRPC serve
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			log.Fatalf("🔴 Failed to listen: %v", err)
		}

		log.Printf("🛰️ gRPC server listened at %v", lis.Addr())

		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("🔴 Failed to serve: %v", err)
			grpcErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🟡 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	httpServer.Shutdown(ctx)
	grpcServer.GracefulStop()
}

// newPermissionClient builds the PermissionService gRPC client, using mTLS
// credentials against identity-service when TLS is enabled, insecure otherwise.
func newPermissionClient(cfg *config.Config) (auth.PermissionClient, error) {
	if !cfg.Server.GRPCTLSEnabled {
		return auth.NewPermissionClient(cfg.Server.AuthServiceAddr)
	}
	creds, err := grpctls.ClientTLSCreds(cfg.Server.GRPCTLSCertFile, cfg.Server.GRPCTLSKeyFile, cfg.Server.GRPCTLSCAFile, "identity-service")
	if err != nil {
		return nil, err
	}
	return auth.NewPermissionClientWithTLS(cfg.Server.AuthServiceAddr, creds)
}
