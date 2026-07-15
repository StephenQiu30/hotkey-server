// Package database owns PostgreSQL connectivity and compatibility checks. It
// deliberately exposes one pgx pool and derives every SQL consumer from it.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// Runtime is the sole owner of a network pool. SQL is a database/sql facade
// derived from Pool, and GORM is configured to use that facade; neither opens
// a separate connection pool or parses a second DSN.
type Runtime struct {
	Pool *pgxpool.Pool
	SQL  *sql.DB
	GORM *gorm.DB
}

// Open establishes the shared pool, derives its database/sql facade, and
// creates GORM over that facade.
func Open(ctx context.Context, dsn string) (*Runtime, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("database URL is required")
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pgx pool: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	gormDB, err := openGORM(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		pool.Close()
		return nil, err
	}

	return &Runtime{Pool: pool, SQL: sqlDB, GORM: gormDB}, nil
}

// NewRuntime is the Fx constructor used when an application process has a
// database URL. Tests can construct an in-memory lifecycle without a database
// by not supplying the runtime option.
func NewRuntime(cfg config.Config) (*Runtime, error) {
	return Open(context.Background(), cfg.DatabaseURL)
}

// Close releases the SQL facade before its owning pgx pool.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	if r.SQL != nil {
		closeErr = r.SQL.Close()
	}
	if r.Pool != nil {
		r.Pool.Close()
	}
	return closeErr
}

// Ping is the cheap readiness probe. The deeper catalog check is performed at
// lifecycle start and by the explicit db verify command.
func (r *Runtime) Ping(ctx context.Context) error {
	if r == nil || r.Pool == nil {
		return errors.New("database runtime is not initialized")
	}
	if err := r.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

// RegisterLifecycle verifies an existing schema on start and owns pool
// shutdown. It never calls schema initialization or automatic GORM DDL.
func RegisterLifecycle(lifecycle fx.Lifecycle, runtime *Runtime) {
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if _, err := Verify(ctx, runtime.Pool); err != nil {
				return fmt.Errorf("verify database compatibility: %w", err)
			}
			return nil
		},
		OnStop: func(context.Context) error {
			return runtime.Close()
		},
	})
}
