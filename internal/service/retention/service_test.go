package retention

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/objectstorage"
)

func TestCleanupExpired_DeletesExpiredObjects(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/expired",
		Metadata: objectstorage.Metadata{UserID: "user1", ExpiresAt: &past},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put expired fixture: %v", err)
	}
	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/valid",
		Metadata: objectstorage.Metadata{UserID: "user1", ExpiresAt: &future},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put valid fixture: %v", err)
	}

	svc := NewService(store, slog.Default())
	svc.now = func() time.Time { return time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC) }

	deleted, err := svc.CleanupExpired(ctx, "")
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	// Verify expired object is gone
	_, _, getErr := store.Get(ctx, "user1/src1/2026/01/01/expired")
	if !errors.Is(getErr, objectstorage.ErrNotFound) {
		t.Errorf("expired object still exists after cleanup")
	}
}

func TestCleanupExpired_EmptyBucketAllowed(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	svc := NewService(store, slog.Default())
	svc.now = func() time.Time { return time.Now() }

	// Empty bucket should not error
	_, err := svc.CleanupExpired(ctx, "")
	if err != nil {
		t.Errorf("CleanupExpired with empty bucket: %v", err)
	}
}

func TestDeleteUserObjects_ByUserPrefix(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/a",
		Metadata: objectstorage.Metadata{UserID: "user1"},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}
	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user1/src2/2026/01/02/b",
		Metadata: objectstorage.Metadata{UserID: "user1"},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}
	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user2/src1/2026/01/01/c",
		Metadata: objectstorage.Metadata{UserID: "user2"},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}

	svc := NewService(store, slog.Default())

	deleted, err := svc.DeleteUserObjects(ctx, "user1")
	if err != nil {
		t.Fatalf("DeleteUserObjects: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	// user2's objects should be untouched
	_, _, getErr := store.Get(ctx, "user2/src1/2026/01/01/c")
	if getErr != nil {
		t.Errorf("user2 object should still exist: %v", getErr)
	}
}

func TestDeleteUserObjects_EmptyUserID(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	svc := NewService(store, slog.Default())

	_, err := svc.DeleteUserObjects(ctx, "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("DeleteUserObjects(empty) error = %v, want ErrInvalidInput", err)
	}
}

func TestDeleteUserObjects_MetadataMismatchSkips(t *testing.T) {
	// Object key says user1 but metadata says user2 — should NOT be deleted
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/orphan",
		Metadata: objectstorage.Metadata{UserID: "user2"}, // mismatch
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}

	svc := NewService(store, slog.Default())

	deleted, err := svc.DeleteUserObjects(ctx, "user1")
	if err != nil {
		t.Fatalf("DeleteUserObjects: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (metadata mismatch should skip)", deleted)
	}
}

func TestDeleteUserObjects_DeleteError(t *testing.T) {
	store := &deleteErrorStore{
		MemoryStore: objectstorage.NewMemoryStore(),
		deleteErr:   errors.New("storage unavailable"),
	}
	ctx := context.Background()

	if err := store.MemoryStore.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/a",
		Metadata: objectstorage.Metadata{UserID: "user1"},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}

	svc := NewService(store, slog.Default())

	_, err := svc.DeleteUserObjects(ctx, "user1")
	if err == nil {
		t.Error("DeleteUserObjects should return error when all deletes fail")
	}
}

func TestCleanupExpired_ListExpiredError(t *testing.T) {
	store := &listErrorStore{listErr: errors.New("connection refused")}
	ctx := context.Background()

	svc := NewService(store, slog.Default())
	svc.now = func() time.Time { return time.Now() }

	_, err := svc.CleanupExpired(ctx, "")
	if err == nil {
		t.Error("CleanupExpired should return error when ListExpired fails")
	}
}

// deleteErrorStore wraps MemoryStore but returns errors on Delete.
type deleteErrorStore struct {
	*objectstorage.MemoryStore
	deleteErr error
}

func (s *deleteErrorStore) Delete(ctx context.Context, key string) error {
	return s.deleteErr
}

// listErrorStore always returns an error from ListExpired.
type listErrorStore struct {
	listErr error
}

func (s *listErrorStore) Delete(ctx context.Context, key string) error {
	return nil
}

func (s *listErrorStore) ListExpired(ctx context.Context, bucket string, before time.Time) ([]objectstorage.Object, error) {
	return nil, s.listErr
}

func (s *listErrorStore) ListByPrefix(ctx context.Context, prefix string) ([]objectstorage.Object, error) {
	return nil, s.listErr
}
