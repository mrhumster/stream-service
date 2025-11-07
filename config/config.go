package config

import (
	"fmt"
	"os"
)

type Config struct {
	Server
	Database
	JWT
}

type Server struct {
	ServerAddr string
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

func LoadConfig() (*Config, error) {
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
			ServerAddr: os.Getenv("SERVER_ADDR"),
		},
		JWT: JWT{
			AccessPublicKeyUrl: os.Getenv("JWT_ACCESS_PUBLIC_KEY_URL"),
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
