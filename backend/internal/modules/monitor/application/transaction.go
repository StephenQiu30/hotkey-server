package application

import "context"

// TransactionRunner owns the atomic boundary required by Monitor use cases.
type TransactionRunner interface {
	RunInTransaction(context.Context, func(context.Context) error) error
}

// TransactionLocker serializes configuration commands inside that boundary.
type TransactionLocker interface {
	LockTransaction(context.Context, string) error
}

type configurationTransactions interface {
	TransactionRunner
	TransactionLocker
}
