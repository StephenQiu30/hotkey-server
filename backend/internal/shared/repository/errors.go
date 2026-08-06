package repository

import "errors"

var (
	ErrNotFound     = errors.New("repository record not found")
	ErrConflict     = errors.New("repository conflict")
	ErrInvalidInput = errors.New("repository invalid input")
	ErrConstraint   = errors.New("repository constraint violation")
	ErrUnavailable  = errors.New("repository temporarily unavailable")
	ErrImmutable    = errors.New("repository record is immutable")
	ErrUnsupported  = errors.New("repository operation is unsupported")
)
