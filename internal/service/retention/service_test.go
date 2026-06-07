package retention

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/objectstorage"
)

type mockStore struct {
	objects map[string]*objectstorage.Object
	deleteErr error
	listExpiredErr error
	listByPrefixErr error
	deletedKeys []string
}

func newMockStore() *mockStore {
	return &mockStore{
		objects: make(map[string]*objectstorage.Object),
	}
}

func (m *mockStore) Delete(_ context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.objects[key]; !ok {
		return objectstorage.ErrNotFound
	}
	delete(m.objects, key)
	m.deletedKeys = append(m.deletedKeys, key)
	return nil
}

func (m *mockStore) ListExpired(_ context.Context, _ string, before time.Time) ([]objectstorage.Object, error) {
	if m.listExpiredErr != nil {
		return nil, m.listExpiredErr
	}
	var result []objectstorage.Object
	for _, obj := range m.objects {
		if obj.Metadata.ExpiresAt != nil && obj.Metadata.ExpiresAt.Before(before) {
			result = append(result, *obj)
		}
	}
	return result, nil
}

func (m *mockStore) ListByPrefix(_ context.Context, prefix string) ([]objectstorage.Object, error) {
	if m.listByPrefixErr != nil {
		return nil, m.listByPrefixErr
	}
	var result []objectstorage.Object
	for key, obj := range m.objects {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, *obj)
		}
	}
	return result, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func TestCleanupExpired(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	t.Run("deletes expired objects", func(t *testing.T) {
		store := newMockStore()
		store.objects["expired-1"] = &objectstorage.Object{
			Key:    "expired-1",
			Bucket: "test",
			Metadata: objectstorage.Metadata{
				ExpiresAt: &past,
			},
		}
		store.objects["not-expired"] = &objectstorage.Object{
			Key:    "not-expired",
			Bucket: "test",
			Metadata: objectstorage.Metadata{
				ExpiresAt: &future,
			},
		}
		store.objects["no-expiry"] = &objectstorage.Object{
			Key:    "no-expiry",
			Bucket: "test",
			Metadata: objectstorage.Metadata{
				ExpiresAt: nil,
			},
		}

		svc := NewService(store, testLogger())
		svc.now = func() time.Time { return now }

		deleted, err := svc.CleanupExpired(context.Background(), "test")
		if err != nil {
			t.Fatalf("CleanupExpired() error = %v", err)
		}
		if deleted != 1 {
			t.Errorf("CleanupExpired() deleted = %d, want 1", deleted)
		}
		if _, exists := store.objects["expired-1"]; exists {
			t.Error("expired-1 should have been deleted")
		}
		if _, exists := store.objects["not-expired"]; !exists {
			t.Error("not-expired should not have been deleted")
		}
		if _, exists := store.objects["no-expiry"]; !exists {
			t.Error("no-expiry should not have been deleted")
		}
	})

	t.Run("returns 0 when no expired objects", func(t *testing.T) {
		store := newMockStore()
		store.objects["future"] = &objectstorage.Object{
			Key:    "future",
			Bucket: "test",
			Metadata: objectstorage.Metadata{
				ExpiresAt: &future,
			},
		}

		svc := NewService(store, testLogger())
		svc.now = func() time.Time { return now }

		deleted, err := svc.CleanupExpired(context.Background(), "test")
		if err != nil {
			t.Fatalf("CleanupExpired() error = %v", err)
		}
		if deleted != 0 {
			t.Errorf("CleanupExpired() deleted = %d, want 0", deleted)
		}
	})

	t.Run("returns error when ListExpired fails", func(t *testing.T) {
		store := newMockStore()
		store.listExpiredErr = errors.New("storage unavailable")

		svc := NewService(store, testLogger())
		svc.now = func() time.Time { return now }

		_, err := svc.CleanupExpired(context.Background(), "test")
		if err == nil {
			t.Fatal("CleanupExpired() expected error")
		}
	})
}

func TestDeleteUserObjects(t *testing.T) {
	t.Run("deletes all objects for a user", func(t *testing.T) {
		store := newMockStore()
		store.objects["user-1/2026/06/01/item-1"] = &objectstorage.Object{
			Key: "user-1/2026/06/01/item-1",
			Metadata: objectstorage.Metadata{
				UserID: "user-1",
			},
		}
		store.objects["user-1/2026/06/02/item-2"] = &objectstorage.Object{
			Key: "user-1/2026/06/02/item-2",
			Metadata: objectstorage.Metadata{
				UserID: "user-1",
			},
		}
		store.objects["user-2/2026/06/01/item-3"] = &objectstorage.Object{
			Key: "user-2/2026/06/01/item-3",
			Metadata: objectstorage.Metadata{
				UserID: "user-2",
			},
		}

		svc := NewService(store, testLogger())
		deleted, err := svc.DeleteUserObjects(context.Background(), "user-1")
		if err != nil {
			t.Fatalf("DeleteUserObjects() error = %v", err)
		}
		if deleted != 2 {
			t.Errorf("DeleteUserObjects() deleted = %d, want 2", deleted)
		}
		if _, exists := store.objects["user-1/2026/06/01/item-1"]; exists {
			t.Error("user-1 item-1 should have been deleted")
		}
		if _, exists := store.objects["user-1/2026/06/02/item-2"]; exists {
			t.Error("user-1 item-2 should have been deleted")
		}
		if _, exists := store.objects["user-2/2026/06/01/item-3"]; !exists {
			t.Error("user-2 item-3 should not have been deleted")
		}
	})

	t.Run("returns error for empty user ID", func(t *testing.T) {
		store := newMockStore()
		svc := NewService(store, testLogger())

		_, err := svc.DeleteUserObjects(context.Background(), "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("DeleteUserObjects() error = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("returns 0 when user has no objects", func(t *testing.T) {
		store := newMockStore()
		svc := NewService(store, testLogger())

		deleted, err := svc.DeleteUserObjects(context.Background(), "user-nonexistent")
		if err != nil {
			t.Fatalf("DeleteUserObjects() error = %v", err)
		}
		if deleted != 0 {
			t.Errorf("DeleteUserObjects() deleted = %d, want 0", deleted)
		}
	})

	t.Run("returns error when ListByPrefix fails", func(t *testing.T) {
		store := newMockStore()
		store.listByPrefixErr = errors.New("storage unavailable")

		svc := NewService(store, testLogger())
		_, err := svc.DeleteUserObjects(context.Background(), "user-1")
		if err == nil {
			t.Fatal("DeleteUserObjects() expected error")
		}
	})
}
