package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

var ErrNestedTransaction = errors.New("nested transactions are not supported")

type transactionContextKey struct{}

// Transaction is the single handle passed to application transaction closures.
// Raw SQL and GORM both use the same *sql.Tx.
type Transaction struct {
	SQL  *sql.Tx
	GORM *gorm.DB
}

// TransactionFromContext returns the current Runtime transaction when a
// repository is called from a WithinTransaction callback. It lets adapters
// reuse the caller's SQL/GORM handle instead of silently opening a nested
// transaction or escaping the caller's atomic boundary.
func TransactionFromContext(ctx context.Context) (Transaction, bool) {
	if ctx == nil {
		return Transaction{}, false
	}
	transaction, ok := ctx.Value(transactionContextKey{}).(Transaction)
	return transaction, ok && transaction.SQL != nil && transaction.GORM != nil
}

// WithinTransaction executes fn exactly once in a transaction. Re-entering
// with the callback context is rejected instead of creating a savepoint; a
// separately supplied parent context deliberately starts an independent
// top-level transaction. Panics roll back and are re-thrown; context
// cancellation is delegated to the standard library transaction so the
// connection is returned to the pool.
func (r *Runtime) WithinTransaction(ctx context.Context, fn func(context.Context, Transaction) error) (err error) {
	if r == nil || r.SQL == nil || r.GORM == nil {
		return fmt.Errorf("database runtime is not initialized")
	}
	if fn == nil {
		return fmt.Errorf("transaction callback is required")
	}
	if ctx.Value(transactionContextKey{}) != nil {
		return ErrNestedTransaction
	}
	tx, err := r.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		}
	}()

	transaction := Transaction{SQL: tx}
	transactionCtx := context.WithValue(ctx, transactionContextKey{}, transaction)
	gormTx := r.GORM.Session(&gorm.Session{Context: transactionCtx, NewDB: true})
	gormTx.Statement.ConnPool = tx
	transaction.GORM = gormTx
	transactionCtx = context.WithValue(ctx, transactionContextKey{}, transaction)
	if err := fn(transactionCtx, transaction); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return fmt.Errorf("transaction failed: %w (rollback: %w)", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// RunInTransaction exposes the context-carried unit-of-work boundary without
// leaking SQL or GORM transaction handles into Application packages.
func (r *Runtime) RunInTransaction(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("transaction callback is required")
	}
	return r.WithinTransaction(ctx, func(transactionCtx context.Context, _ Transaction) error {
		return fn(transactionCtx)
	})
}

// LockTransaction serializes work inside the current transaction using a
// stable text key. Callers own the business key; this adapter owns PostgreSQL.
func (r *Runtime) LockTransaction(ctx context.Context, key string) error {
	return r.lockTransaction(ctx, key, false)
}

// LockTransactionWide is the 64-bit-key variant used by high-cardinality
// source and evidence identities.
func (r *Runtime) LockTransactionWide(ctx context.Context, key string) error {
	return r.lockTransaction(ctx, key, true)
}

func (r *Runtime) lockTransaction(ctx context.Context, key string, wide bool) error {
	if r == nil || strings.TrimSpace(key) == "" {
		return fmt.Errorf("transaction lock key is required")
	}
	transaction, found := TransactionFromContext(ctx)
	if !found {
		return fmt.Errorf("transaction lock requires an active transaction")
	}
	statement := `SELECT pg_advisory_xact_lock(hashtext($1))`
	if wide {
		statement = `SELECT pg_advisory_xact_lock(hashtextextended($1, 0::bigint))`
	}
	if _, err := transaction.SQL.ExecContext(ctx, statement, strings.TrimSpace(key)); err != nil {
		return fmt.Errorf("acquire transaction lock: %w", err)
	}
	return nil
}
