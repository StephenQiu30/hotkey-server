// Package repository defines infrastructure-neutral persistence contracts.
package repository

import "context"

type PageQuery struct {
	Cursor            string
	Limit             int
	Sort              string
	Descending        bool
	FilterFingerprint string
}

type PageResult[T any] struct {
	Items      []T
	NextCursor string
}

type CRUDRepository[T any, ID comparable] interface {
	Create(context.Context, *T) error
	GetByID(context.Context, ID) (*T, error)
	List(context.Context, PageQuery) (PageResult[T], error)
	Update(context.Context, *T) error
	Delete(context.Context, ID) error
}

// HistoryRepository intentionally omits Update and Delete for immutable run,
// revision, snapshot and audit facts.
type HistoryRepository[T any, ID comparable] interface {
	Create(context.Context, *T) error
	GetByID(context.Context, ID) (*T, error)
	List(context.Context, PageQuery) (PageResult[T], error)
}
