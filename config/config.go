// config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Server
	Database
	JWT
	MinIO
}

type Server struct {
	ServerAddr      string
	AuthServiceAddr string
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
	AccessPublicKeyUrl string
}

type MinIO struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	UseSSL          bool
	Region          string
}

func LoadConfig() (*Config, error) {
	useSSL, _ := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))

	return &Config{
		Database: Database{
			Host:     os.Getenv("DB_HOST"),
			Port:     os.Getenv("DB_PORT"),
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASS"),
			Name:     os.Getenv("DB_NAME"),
			SslMode:  "disable",
			TimeZone: "UTC",
		},
		Server: Server{
			ServerAddr:      os.Getenv("SERVER_ADDR"),
			AuthServiceAddr: os.Getenv("AUTH_SERVICE_ADDRESS"),
		},
		JWT: JWT{
			AccessPublicKeyUrl: os.Getenv("JWT_ACCESS_PUBLIC_KEY_URL"),
		},
		MinIO: MinIO{
			Endpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKeyID:     getEnv("MINIO_ACCESS_KEY_ID", "admin"),
			SecretAccessKey: getEnv("MINIO_SECRET_ACCESS_KEY", "minio123"),
			BucketName:      getEnv("MINIO_BUCKET_NAME", "stream-service-test"),
			UseSSL:          useSSL,
			Region:          getEnv("MINIO_REGION", "ru-east-1"),
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
			ServerAddr: os.Getenv("SERVER_ADDR"),
		},
		JWT: JWT{
			AccessPublicKeyUrl: os.Getenv("JWT_ACCESS_PUBLIC_KEY_URL"),
		},
	}, nil
}

func (config *Config) GetDsn() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		config.Host,
		config.User,
		config.Password,
		config.Name,
		config.Port,
		config.SslMode,
		config.TimeZone)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}
