package postgres

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Options struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func NewPool(url string, opts Options) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}

	if opts.MaxOpenConns == 0 {
		opts.MaxOpenConns = 25
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = 25
	}
	if opts.ConnMaxLifetime == 0 {
		opts.ConnMaxLifetime = 5 * time.Minute
	}

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxLifetime(opts.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type TransactionalDB struct {
	db *sql.DB
}

func NewTransactionalDB(db *sql.DB) *TransactionalDB {
	return &TransactionalDB{db: db}
}

func (t *TransactionalDB) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

type txKey struct{}

func GetTx(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(*sql.Tx)
	return tx, ok
}

func GetQueryer(ctx context.Context, db *sql.DB) Queryer {
	if tx, ok := GetTx(ctx); ok {
		return tx
	}
	return db
}
