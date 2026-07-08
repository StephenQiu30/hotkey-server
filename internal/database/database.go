package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open connects to PostgreSQL and initializes the database if needed.
func Open(databaseURL string) (*gorm.DB, error) {
	if err := EnsureReady(context.Background(), databaseURL); err != nil {
		return nil, fmt.Errorf("ensure database ready: %w", err)
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: &ZapGormLogger{SlowThreshold: 200 * time.Millisecond},
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm db: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}
