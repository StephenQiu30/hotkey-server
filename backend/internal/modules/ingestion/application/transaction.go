package application

import "context"

// TransactionRunner owns the atomic boundary required by Ingestion use cases.
type TransactionRunner interface {
	RunInTransaction(context.Context, func(context.Context) error) error
}

// WideTransactionLocker serializes high-cardinality evidence identities.
type WideTransactionLocker interface {
	LockTransactionWide(context.Context, string) error
}

type evidenceTransactions interface {
	TransactionRunner
	WideTransactionLocker
}
