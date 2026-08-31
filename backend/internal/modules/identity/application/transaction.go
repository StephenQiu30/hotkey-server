package application

import "context"

// TransactionRunner owns the atomic boundary required by identity use cases.
type TransactionRunner interface {
	RunInTransaction(context.Context, func(context.Context) error) error
}
