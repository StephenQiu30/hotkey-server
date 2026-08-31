package application

import "context"

// TransactionRunner owns an atomic Source use-case boundary.
type TransactionRunner interface {
	RunInTransaction(context.Context, func(context.Context) error) error
}

// TransactionLocker serializes bounded Source configuration identities.
type TransactionLocker interface {
	LockTransaction(context.Context, string) error
}

// WideTransactionLocker serializes high-cardinality collection identities.
type WideTransactionLocker interface {
	LockTransactionWide(context.Context, string) error
}

type configurationTransactions interface {
	TransactionRunner
	TransactionLocker
}

type collectionTransactions interface {
	TransactionRunner
	WideTransactionLocker
}
