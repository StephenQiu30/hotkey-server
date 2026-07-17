package repository

import (
	"context"
	"testing"
)

type testRecord struct{ ID int64 }

func TestRepositoryContractsAreTypeSafe(t *testing.T) {
	var _ CRUDRepository[testRecord, int64] = (*testCRUDRepository)(nil)
	var _ HistoryRepository[testRecord, int64] = (*testHistoryRepository)(nil)
}

type testCRUDRepository struct{}

func (*testCRUDRepository) Create(ctx context.Context, v *testRecord) error { return nil }
func (*testCRUDRepository) GetByID(ctx context.Context, id int64) (*testRecord, error) {
	return nil, nil
}
func (*testCRUDRepository) List(ctx context.Context, q PageQuery) (PageResult[testRecord], error) {
	return PageResult[testRecord]{}, nil
}
func (*testCRUDRepository) Update(ctx context.Context, v *testRecord) error { return nil }
func (*testCRUDRepository) Delete(ctx context.Context, id int64) error      { return nil }

type testHistoryRepository struct{}

func (*testHistoryRepository) Create(ctx context.Context, v *testRecord) error { return nil }
func (*testHistoryRepository) GetByID(ctx context.Context, id int64) (*testRecord, error) {
	return nil, nil
}
func (*testHistoryRepository) List(ctx context.Context, q PageQuery) (PageResult[testRecord], error) {
	return PageResult[testRecord]{}, nil
}
