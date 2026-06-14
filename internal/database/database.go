package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Open connects to PostgreSQL using the given DATABASE_URL.
// It creates the database and applies db/schema.sql when they are missing.
func Open(databaseURL string) (*sql.DB, error) {
	if err := EnsureReady(context.Background(), databaseURL); err != nil {
		return nil, fmt.Errorf("ensure database ready: %w", err)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}
