package application

import "context"

// TransactionRunner owns the atomic boundary required by delivery use cases.
type TransactionRunner interface {
	RunInTransaction(context.Context, func(context.Context) error) error
}
