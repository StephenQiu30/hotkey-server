package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	ErrNotFound     = errors.New("repository record not found")
	ErrConflict     = errors.New("repository conflict")
	ErrInvalidInput = errors.New("repository invalid input")
	ErrConstraint   = errors.New("repository constraint violation")
	ErrUnavailable  = errors.New("repository temporarily unavailable")
	ErrImmutable    = errors.New("repository record is immutable")
	ErrUnsupported  = errors.New("repository operation is unsupported")
)

// MapError keeps database implementation details from leaking through the
// repository boundary while preserving errors.Is classification for callers.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "40001":
			return fmt.Errorf("%w: %v", ErrConflict, err)
		case "23503", "23514":
			return fmt.Errorf("%w: %v", ErrConstraint, err)
		case "57014":
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return err
}
