package database

import (
	"errors"
	"log"
	"time"

	"github.com/mrhumster/stream-service/config"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func SetupDatabase(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.GetDsn()), &gorm.Config{})
	if err != nil {
		return nil, errors.New("⚠️ GORM not open DB")
	}
	sqlDb, err := db.DB()
	if err != nil {
		return nil, errors.New("⚠️ GORM not open DB")
	}
	sqlDb.SetMaxOpenConns(100)
	sqlDb.SetMaxIdleConns(10)
	sqlDb.SetConnMaxLifetime(time.Hour)
	sqlDb.SetConnMaxIdleTime(30 * time.Minute)
	log.Printf("🔌  Creating uuid-ossp extension...")
	db.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")
	db.AutoMigrate(&models.Stream{})
	return db, nil
}
