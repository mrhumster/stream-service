package database

import (
	"errors"
	"time"

	"github.com/mrhumster/stream-service/config"
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
	return db, nil
}
