package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
	transactionCtx := context.WithValue(ctx, transactionContextKey{}, struct{}{})

	tx, err := r.SQL.BeginTx(transactionCtx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		}
	}()

	gormTx := r.GORM.Session(&gorm.Session{Context: transactionCtx, NewDB: true})
	gormTx.Statement.ConnPool = tx
	if err := fn(transactionCtx, Transaction{SQL: tx, GORM: gormTx}); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
			return fmt.Errorf("transaction failed: %w (rollback: %v)", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
