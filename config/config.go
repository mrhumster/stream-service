// config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server   Server
	Database Database
	JWT      JWT
	MinIO    MinIO
	Redis    Redis
}

type Redis struct {
	Addr     string
	Password string
	// EventsQueueDB is the Redis DB serving the events-service asynq queue
	// (event:activity). Defaults to 3.
	EventsQueueDB int
}

type ServerMode string

const (
	Debug   ServerMode = "debug"
	Release ServerMode = "release"
	Test    ServerMode = "test"
)

type Server struct {
	ServerAddr        string
	AuthServiceAddr   string
	KeepOriginalFile  bool
	Mode              ServerMode
	GRPCTLSCertFile   string
	GRPCTLSKeyFile    string
	GRPCTLSCAFile     string
	GRPCTLSAllowedOUs []string
	GRPCTLSEnabled    bool
	AllowedOrigins    []string
}

type Database struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SslMode  string
	TimeZone string
}

type JWT struct {
	AccessPublicKeyURL string
}

type MinIO struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	UseSSL          bool
	Region          string
}

func (m ServerMode) isValid() bool {
	switch m {
	case Test, Debug, Release:
		return true
	}
	return false
}

func LoadConfig() (*Config, error) {
	useSSL, _ := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	keepOriginalFile, _ := strconv.ParseBool(getEnv("KEEP_ORIGINAL_FILE", "true"))

	mode := ServerMode(getEnv("MODE", "debug"))
	if !mode.isValid() {
		mode = Release
	}

	return &Config{
		Database: Database{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASS", ""),
			Name:     getEnv("DB_NAME", "postgres"),
			SslMode:  "disable",
			TimeZone: "UTC",
		},
		Server: Server{
			ServerAddr:        os.Getenv("SERVER_ADDR"),
			AuthServiceAddr:   os.Getenv("AUTH_SERVICE_ADDRESS"),
			KeepOriginalFile:  keepOriginalFile,
			Mode:              mode,
			GRPCTLSCertFile:   os.Getenv("GRPC_TLS_CERT"),
			GRPCTLSKeyFile:    os.Getenv("GRPC_TLS_KEY"),
			GRPCTLSCAFile:     os.Getenv("GRPC_TLS_CA"),
			GRPCTLSAllowedOUs: commaSplit(getEnv("GRPC_TLS_ALLOWED_OUS", "")),
			GRPCTLSEnabled:    getBool("GRPC_TLS_ENABLED"),
			AllowedOrigins:    commaSplit(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173,https://example.com,https://api.example.com")),
		},
		JWT: JWT{
			AccessPublicKeyURL: os.Getenv("JWT_ACCESS_PUBLIC_KEY_URL"),
		},
		MinIO: MinIO{
			Endpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKeyID:     getEnv("MINIO_ACCESS_KEY", "admin"),
			SecretAccessKey: getEnv("MINIO_SECRET_KEY", "minio123"),
			BucketName:      getEnv("MINIO_BUCKET_NAME", "stream-service-test"),
			UseSSL:          useSSL,
			Region:          getEnv("MINIO_REGION", "ru-east-1"),
		},
		Redis: Redis{
			Addr:           getEnv("REDIS_ADDR", "localhost"),
			Password:       getEnv("redis-password", ""),
			EventsQueueDB:  getQueueDB("EVENTS_QUEUE_DB", 3),
		},
	}, nil
}

func TestConfig() (*Config, error) {
	return &Config{
		Database: Database{
			Host:     os.Getenv("DB_HOST"),
			Port:     os.Getenv("DB_PORT"),
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASS"),
			Name:     "testdatabase1",
			SslMode:  "disable",
			TimeZone: "UTC",
		},
		Server: Server{
			ServerAddr:       os.Getenv("SERVER_ADDR"),
			KeepOriginalFile: false,
			Mode:             Test,
			AllowedOrigins:   commaSplit(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173,https://example.com,https://api.example.com")),
		},
		JWT: JWT{
			AccessPublicKeyURL: os.Getenv("JWT_ACCESS_PUBLIC_KEY_URL"),
		},
	}, nil
}

func (config *Config) GetDsn() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		config.Database.Host,
		config.Database.Port,
		config.Database.User,
		config.Database.Password,
		config.Database.Name,
		config.Database.SslMode,
		config.Database.TimeZone)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}

func getBool(key string) bool {
	v, _ := strconv.ParseBool(os.Getenv(key))
	return v
}

// getQueueDB parses an asynq queue DB index; on any parse failure it falls
// back to the provided default.
func getQueueDB(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}

func commaSplit(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
