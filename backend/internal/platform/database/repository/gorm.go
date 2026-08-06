package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/model"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"gorm.io/gorm"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// GORMCRUD 是通用 CRUD 契约的窄实现。
// 它只允许使用 model.PersistenceFor 注册的表，禁止请求输入覆盖表名、版本、删除和游标规则。
type GORMCRUD[T any] struct {
	db   *gorm.DB
	meta model.Persistence
}

// NewGORMCRUD 为一个已注册的权威表映射创建仓储。
func NewGORMCRUD[T any](db *gorm.DB, table string) (*GORMCRUD[T], error) {
	if db == nil {
		return nil, fmt.Errorf("%w: GORM database is required", sharedrepository.ErrInvalidInput)
	}
	meta, found := model.PersistenceFor(table)
	if !found {
		return nil, fmt.Errorf("%w: unregistered table %q", sharedrepository.ErrInvalidInput, table)
	}
	return &GORMCRUD[T]{db: db, meta: meta}, nil
}

func (r *GORMCRUD[T]) Create(ctx context.Context, value *T) error {
	if value == nil {
		return fmt.Errorf("%w: record is required", sharedrepository.ErrInvalidInput)
	}
	if versioned, found := versionedRecord(value); found && versioned.GetVersion() == 0 {
		versioned.SetVersion(1)
	}
	if err := r.db.WithContext(ctx).Table(r.meta.Table).Create(value).Error; err != nil {
		return MapError(err)
	}
	return nil
}

func (r *GORMCRUD[T]) GetByID(ctx context.Context, id int64) (*T, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: positive id is required", sharedrepository.ErrInvalidInput)
	}
	var value T
	query := r.activeScope(r.db.WithContext(ctx).Table(r.meta.Table)).Where(r.meta.KeyColumn+" = ?", id)
	if err := query.Take(&value).Error; err != nil {
		return nil, MapError(err)
	}
	return &value, nil
}

func (r *GORMCRUD[T]) List(ctx context.Context, query sharedrepository.PageQuery) (sharedrepository.PageResult[T], error) {
	limit, err := pageLimit(query.Limit)
	if err != nil {
		return sharedrepository.PageResult[T]{}, err
	}
	sort := query.Sort
	if sort == "" {
		sort = r.meta.KeyColumn
	}
	if !slices.Contains(r.meta.AllowedSort, sort) || sort != r.meta.KeyColumn {
		return sharedrepository.PageResult[T]{}, fmt.Errorf("%w: sort %q is not allowed", sharedrepository.ErrInvalidInput, sort)
	}
	cursor, err := pagination.Decode(query.Cursor, sort, query.Descending, query.FilterFingerprint)
	if err != nil {
		return sharedrepository.PageResult[T]{}, mapCursorError(err)
	}

	db := r.activeScope(r.db.WithContext(ctx).Table(r.meta.Table))
	if cursor.ID != 0 {
		operator := ">"
		if query.Descending {
			operator = "<"
		}
		db = db.Where(r.meta.KeyColumn+" "+operator+" ?", cursor.ID)
	}
	direction := "ASC"
	if query.Descending {
		direction = "DESC"
	}
	var values []T
	if err := db.Order(r.meta.KeyColumn + " " + direction).Limit(limit + 1).Find(&values).Error; err != nil {
		return sharedrepository.PageResult[T]{}, MapError(err)
	}

	result := sharedrepository.PageResult[T]{Items: values}
	if len(result.Items) <= limit {
		return result, nil
	}
	result.Items = result.Items[:limit]
	last := &result.Items[len(result.Items)-1]
	identified, found := identifiedRecord(last)
	if !found || identified.GetID() <= 0 {
		return sharedrepository.PageResult[T]{}, fmt.Errorf("%w: cursor model must expose an id", sharedrepository.ErrInvalidInput)
	}
	next, err := pagination.Encode(sort, query.Descending, query.FilterFingerprint, identified.GetID())
	if err != nil {
		return sharedrepository.PageResult[T]{}, fmt.Errorf("encode next cursor: %w", err)
	}
	result.NextCursor = next
	return result, nil
}

func (r *GORMCRUD[T]) Update(ctx context.Context, value *T) error {
	if value == nil {
		return fmt.Errorf("%w: record is required", sharedrepository.ErrInvalidInput)
	}
	if r.meta.VersionColumn == "" {
		return fmt.Errorf("%w: %s has no optimistic-lock version", sharedrepository.ErrUnsupported, r.meta.Table)
	}
	versioned, found := versionedRecord(value)
	if !found || versioned.GetID() <= 0 || versioned.GetVersion() <= 0 {
		return fmt.Errorf("%w: versioned record with positive id and version is required", sharedrepository.ErrInvalidInput)
	}
	previousVersion := versioned.GetVersion()
	versioned.SetVersion(previousVersion + 1)
	result := r.activeScope(r.db.WithContext(ctx).Table(r.meta.Table)).
		Where(r.meta.KeyColumn+" = ? AND "+r.meta.VersionColumn+" = ?", versioned.GetID(), previousVersion).
		Omit(r.meta.KeyColumn, "created_at", "updated_at", "deleted_at").
		Updates(value)
	if result.Error != nil {
		versioned.SetVersion(previousVersion)
		return MapError(result.Error)
	}
	if result.RowsAffected == 0 {
		versioned.SetVersion(previousVersion)
		return fmt.Errorf("%w: record was changed or removed", sharedrepository.ErrConflict)
	}
	return nil
}

func (r *GORMCRUD[T]) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: positive id is required", sharedrepository.ErrInvalidInput)
	}
	var result *gorm.DB
	switch r.meta.Deletion {
	case model.DeletionSoft:
		result = r.db.WithContext(ctx).Table(r.meta.Table).
			Where(r.meta.KeyColumn+" = ? AND deleted_at IS NULL", id).
			Updates(map[string]any{"deleted_at": gorm.Expr("now()")})
	case model.DeletionHard:
		result = r.db.WithContext(ctx).Exec("DELETE FROM "+r.meta.Table+" WHERE "+r.meta.KeyColumn+" = ?", id)
	case model.DeletionRetained:
		return fmt.Errorf("%w: %s", sharedrepository.ErrImmutable, r.meta.Table)
	default:
		return fmt.Errorf("%w: unknown deletion policy for %s", sharedrepository.ErrUnsupported, r.meta.Table)
	}
	if result.Error != nil {
		return MapError(result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %d", sharedrepository.ErrNotFound, id)
	}
	return nil
}

// GORMHistory 为运行、快照和审计数据提供不可变操作子集。
// 它刻意不实现完整 CRUDRepository，避免历史事实被更新或删除。
type GORMHistory[T any] struct {
	crud *GORMCRUD[T]
}

func NewGORMHistory[T any](db *gorm.DB, table string) (*GORMHistory[T], error) {
	crud, err := NewGORMCRUD[T](db, table)
	if err != nil {
		return nil, err
	}
	if crud.meta.Deletion != model.DeletionRetained {
		return nil, fmt.Errorf("%w: %s is not an immutable history table", sharedrepository.ErrInvalidInput, table)
	}
	return &GORMHistory[T]{crud: crud}, nil
}

func (r *GORMHistory[T]) Create(ctx context.Context, value *T) error {
	return r.crud.Create(ctx, value)
}
func (r *GORMHistory[T]) GetByID(ctx context.Context, id int64) (*T, error) {
	return r.crud.GetByID(ctx, id)
}
func (r *GORMHistory[T]) List(ctx context.Context, query sharedrepository.PageQuery) (sharedrepository.PageResult[T], error) {
	return r.crud.List(ctx, query)
}

type versioned interface {
	GetID() int64
	GetVersion() int64
	SetVersion(int64)
}

type identified interface {
	GetID() int64
}

func versionedRecord(value any) (versioned, bool) {
	record, found := value.(versioned)
	return record, found
}

func identifiedRecord(value any) (identified, bool) {
	record, found := value.(identified)
	return record, found
}

func (r *GORMCRUD[T]) activeScope(db *gorm.DB) *gorm.DB {
	if r.meta.Deletion == model.DeletionSoft {
		return db.Where("deleted_at IS NULL")
	}
	return db
}

func pageLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultPageLimit, nil
	}
	if limit < 0 || limit > maxPageLimit {
		return 0, fmt.Errorf("%w: limit must be between 1 and %d", sharedrepository.ErrInvalidInput, maxPageLimit)
	}
	return limit, nil
}

func mapCursorError(err error) error {
	if errors.Is(err, pagination.ErrInvalidCursor) || errors.Is(err, pagination.ErrStaleCursor) {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return err
}
