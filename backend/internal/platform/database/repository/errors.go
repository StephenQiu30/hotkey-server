// Package repository 提供 internal/shared/repository 中基础设施中立契约的数据库实现。
package repository

import (
	"context"
	"errors"
	"fmt"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// MapError 将数据库实现错误映射为 shared 中稳定的仓储错误分类。
func MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %v", sharedrepository.ErrNotFound, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", sharedrepository.ErrUnavailable, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "40001":
			return fmt.Errorf("%w: %v", sharedrepository.ErrConflict, err)
		case "23503", "23514":
			return fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		case "57014":
			return fmt.Errorf("%w: %v", sharedrepository.ErrUnavailable, err)
		}
	}
	return err
}
