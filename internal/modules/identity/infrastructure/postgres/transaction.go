package postgres

import (
	"context"
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func useTransaction(ctx context.Context, runtime *database.Runtime, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return runtime.WithinTransaction(ctx, fn)
}

func transactionSQL(ctx context.Context, runtime *database.Runtime) interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
} {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return runtime.SQL
}
