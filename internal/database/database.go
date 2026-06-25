package database

import (
	"context"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open connects to PostgreSQL using the given DATABASE_URL.
// It creates the database and applies db/schema.sql when they are missing.
func Open(databaseURL string) (*gorm.DB, error) {
	if err := EnsureReady(context.Background(), databaseURL); err != nil {
		return nil, fmt.Errorf("ensure database ready: %w", err)
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
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
