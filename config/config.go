// config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
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
}

type ServerMode string

const (
	Debug   ServerMode = "debug"
	Release ServerMode = "release"
	Test    ServerMode = "test"
)

type Server struct {
	ServerAddr       string
	AuthServiceAddr  string
	KeepOriginalFile bool
	Mode             ServerMode
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
			ServerAddr:       os.Getenv("SERVER_ADDR"),
			AuthServiceAddr:  os.Getenv("AUTH_SERVICE_ADDRESS"),
			KeepOriginalFile: keepOriginalFile,
			Mode:             mode,
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
			Addr:     getEnv("REDIS_ADDR", "localhost"),
			Password: getEnv("redis-password", ""),
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
