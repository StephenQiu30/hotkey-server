package objectstorage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMemoryStore_PutAndGet(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	obj := Object{
		Key:         "user1/src1/2026/06/07/item1",
		ContentType: "text/plain",
		Size:        5,
		Metadata: Metadata{
			UserID:       "user1",
			SourceID:     "src1",
			SourceItemID: "item1",
			Retention:    RetentionRawSnapshot,
		},
		CreatedAt: time.Now(),
	}

	if err := store.Put(ctx, obj, strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, reader, err := store.Get(ctx, obj.Key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()

	if got.Key != obj.Key {
		t.Errorf("Get key = %q, want %q", got.Key, obj.Key)
	}
	if got.Metadata.UserID != "user1" {
		t.Errorf("Get UserID = %q, want %q", got.Metadata.UserID, "user1")
	}
}

func TestMemoryStore_PutDuplicate(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	obj := Object{Key: "k1", Metadata: Metadata{UserID: "u1"}}
	if err := store.Put(ctx, obj, strings.NewReader("a")); err != nil {
		t.Fatalf("first Put: %v", err)
	}

	err := store.Put(ctx, obj, strings.NewReader("b"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate Put error = %v, want ErrAlreadyExists", err)
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	_, _, err := store.Get(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get nonexistent error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	obj := Object{Key: "k1", Metadata: Metadata{UserID: "u1"}}
	if err := store.Put(ctx, obj, strings.NewReader("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, _, err := store.Get(ctx, "k1")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_DeleteNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete nonexistent error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_Head(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	obj := Object{Key: "k1", Metadata: Metadata{UserID: "u1"}}
	if err := store.Put(ctx, obj, strings.NewReader("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Head(ctx, "k1")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if got.Key != "k1" {
		t.Errorf("Head key = %q, want %q", got.Key, "k1")
	}
}

func TestMemoryStore_ListExpired(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	store.Put(ctx, Object{Key: "expired", Metadata: Metadata{UserID: "u1", ExpiresAt: &past}}, bytes.NewReader(nil))
	store.Put(ctx, Object{Key: "valid", Metadata: Metadata{UserID: "u1", ExpiresAt: &future}}, bytes.NewReader(nil))
	store.Put(ctx, Object{Key: "noexpiry", Metadata: Metadata{UserID: "u1"}}, bytes.NewReader(nil))

	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	expired, err := store.ListExpired(ctx, "", now)
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}

	if len(expired) != 1 {
		t.Fatalf("ListExpired returned %d objects, want 1", len(expired))
	}
	if expired[0].Key != "expired" {
		t.Errorf("ListExpired key = %q, want %q", expired[0].Key, "expired")
	}
}

func TestMemoryStore_ListByPrefix(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	store.Put(ctx, Object{Key: "user1/src1/2026/01/01/a", Metadata: Metadata{UserID: "user1"}}, bytes.NewReader(nil))
	store.Put(ctx, Object{Key: "user1/src2/2026/01/01/b", Metadata: Metadata{UserID: "user1"}}, bytes.NewReader(nil))
	store.Put(ctx, Object{Key: "user2/src1/2026/01/01/c", Metadata: Metadata{UserID: "user2"}}, bytes.NewReader(nil))

	matched, err := store.ListByPrefix(ctx, "user1/")
	if err != nil {
		t.Fatalf("ListByPrefix: %v", err)
	}

	if len(matched) != 2 {
		t.Fatalf("ListByPrefix(user1/) returned %d objects, want 2", len(matched))
	}
}

func TestMemoryStore_ListByPrefix_NoMatch(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	store.Put(ctx, Object{Key: "user1/src1/a", Metadata: Metadata{UserID: "user1"}}, bytes.NewReader(nil))

	matched, err := store.ListByPrefix(ctx, "user999/")
	if err != nil {
		t.Fatalf("ListByPrefix: %v", err)
	}
	if len(matched) != 0 {
		t.Errorf("ListByPrefix(user999/) returned %d objects, want 0", len(matched))
	}
}

func TestMemoryStore_PutReadError(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	errReader := &errReader{err: errors.New("read failed")}
	obj := Object{Key: "k1", Metadata: Metadata{UserID: "u1"}}

	err := store.Put(ctx, obj, errReader)
	if err == nil {
		t.Error("Put with error reader should fail")
	}
}

type errReader struct {
	err error
}

func (r *errReader) Read([]byte) (int, error) {
	return 0, r.err
}

func (r *errReader) Close() error {
	return nil
}

var _ io.ReadCloser = (*errReader)(nil)
