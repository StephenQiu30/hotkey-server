package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/objectstorage"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestStoreSnapshotHandler_Success(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	contenter := SnapshotContentFunc(func(title, snippet, url string) []byte {
		return []byte("Title: " + title + "\n" + snippet)
	})

	handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)
	handler.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	payload := queue.StoreSnapshotPayload{
		SourceItemID: "item-1",
		SourceID:     "src-1",
		UserID:       "user-1",
		Platform:     "twitter",
		Title:        "Test Title",
		Snippet:      "Test snippet",
		OriginalURL:  "https://example.com/1",
	}
	payloadBytes, _ := json.Marshal(payload)

	job := queue.Job{ID: "j1", Type: queue.JobTypeStoreSnapshot, Payload: payloadBytes}
	err := handler.Handle(context.Background(), job)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Verify key starts with userID
	key := objectstorage.BuildKey("user-1", "src-1", "item-1", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))
	obj, reader, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("Get stored object: %v", err)
	}
	defer reader.Close()

	if obj.Metadata.UserID != "user-1" {
		t.Errorf("stored UserID = %q, want %q", obj.Metadata.UserID, "user-1")
	}
	if obj.Metadata.Platform != "twitter" {
		t.Errorf("stored Platform = %q, want %q", obj.Metadata.Platform, "twitter")
	}
	if obj.Metadata.ExpiresAt == nil {
		t.Error("stored ExpiresAt is nil, want non-nil for raw_snapshot")
	}
}

func TestStoreSnapshotHandler_MissingPayload(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	contenter := SnapshotContentFunc(func(title, snippet, url string) []byte {
		return []byte(title)
	})

	handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)

	job := queue.Job{ID: "j1", Type: queue.JobTypeStoreSnapshot, Payload: json.RawMessage(`{"source_item_id":"","source_id":""}`)}
	err := handler.Handle(context.Background(), job)
	if err == nil {
		t.Error("Handle with empty payload should fail")
	}
}

func TestStoreSnapshotHandler_InvalidJSON(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	contenter := SnapshotContentFunc(func(title, snippet, url string) []byte { return nil })

	handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)

	job := queue.Job{ID: "j1", Type: queue.JobTypeStoreSnapshot, Payload: json.RawMessage(`invalid`)}
	err := handler.Handle(context.Background(), job)
	if err == nil {
		t.Error("Handle with invalid JSON should fail")
	}
}

func TestCleanupExpiredObjectsHandler_Success(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/exp",
		Metadata: objectstorage.Metadata{UserID: "user1", ExpiresAt: &past},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}

	handler := NewCleanupExpiredObjectsHandler(store, "default-bucket")
	handler.now = func() time.Time { return time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC) }

	payload := queue.CleanupExpiredObjectsPayload{Bucket: ""}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeCleanupExpiredObjects, Payload: payloadBytes}

	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestCleanupExpiredObjectsHandler_Success_VerifiesDeletion(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Put(ctx, objectstorage.Object{
		Key:      "expired-obj",
		Metadata: objectstorage.Metadata{UserID: "user1", ExpiresAt: &past},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put expired fixture: %v", err)
	}
	if err := store.Put(ctx, objectstorage.Object{
		Key:      "valid-obj",
		Metadata: objectstorage.Metadata{UserID: "user2", ExpiresAt: &future},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put valid fixture: %v", err)
	}

	handler := NewCleanupExpiredObjectsHandler(store, "bucket")
	handler.now = func() time.Time { return time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC) }

	payload := queue.CleanupExpiredObjectsPayload{Bucket: "bucket"}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeCleanupExpiredObjects, Payload: payloadBytes}

	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Expired object must be deleted
	_, _, err := store.Get(ctx, "expired-obj")
	if !errors.Is(err, objectstorage.ErrNotFound) {
		t.Error("expired object should be deleted after cleanup")
	}

	// Valid object must survive
	_, _, err = store.Get(ctx, "valid-obj")
	if err != nil {
		t.Error("non-expired object should still exist after cleanup")
	}
}

func TestCleanupExpiredObjectsHandler_EmptyBucketUsesDefault(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	handler := NewCleanupExpiredObjectsHandler(store, "my-bucket")
	handler.now = func() time.Time { return time.Now() }

	payload := queue.CleanupExpiredObjectsPayload{Bucket: ""}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeCleanupExpiredObjects, Payload: payloadBytes}

	// Should not error — empty bucket falls back to handler default
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Errorf("Handle with empty bucket: %v", err)
	}
}

func TestCleanupExpiredObjectsHandler_ListExpiredRetryable(t *testing.T) {
	store := &listFailStore{err: errors.New("connection refused")}
	handler := NewCleanupExpiredObjectsHandler(store, "bucket")

	payload := queue.CleanupExpiredObjectsPayload{Bucket: "bucket"}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeCleanupExpiredObjects, Payload: payloadBytes}

	err := handler.Handle(context.Background(), job)
	if err == nil {
		t.Fatal("expected error")
	}
	var retryable queue.RetryableError
	if !errors.As(err, &retryable) {
		t.Errorf("error should be RetryableError, got %T", err)
	}
}

func TestDeleteUserObjectsHandler_Success(t *testing.T) {
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

	handler := NewDeleteUserObjectsHandler(store)

	payload := queue.DeleteUserObjectsPayload{UserID: "user1"}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeDeleteUserObjects, Payload: payloadBytes}

	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// user1 objects should be gone
	_, _, err := store.Get(ctx, "user1/src1/2026/01/01/a")
	if !errors.Is(err, objectstorage.ErrNotFound) {
		t.Error("user1 object should be deleted")
	}

	// user2 object should exist
	_, _, err = store.Get(ctx, "user2/src1/2026/01/01/c")
	if err != nil {
		t.Error("user2 object should still exist")
	}
}

func TestDeleteUserObjectsHandler_EmptyUserID(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	handler := NewDeleteUserObjectsHandler(store)

	payload := queue.DeleteUserObjectsPayload{UserID: ""}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeDeleteUserObjects, Payload: payloadBytes}

	err := handler.Handle(context.Background(), job)
	if err == nil {
		t.Error("Handle with empty UserID should fail")
	}
}

func TestDeleteUserObjectsHandler_ListByPrefixRetryable(t *testing.T) {
	store := &listFailStore{err: errors.New("timeout")}
	handler := NewDeleteUserObjectsHandler(store)

	payload := queue.DeleteUserObjectsPayload{UserID: "user1"}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeDeleteUserObjects, Payload: payloadBytes}

	err := handler.Handle(context.Background(), job)
	if err == nil {
		t.Fatal("expected error")
	}
	var retryable queue.RetryableError
	if !errors.As(err, &retryable) {
		t.Errorf("error should be RetryableError, got %T", err)
	}
}

func TestDeleteUserObjectsHandler_DeleteErrorRetryable(t *testing.T) {
	store := &deleteFailStore{
		MemoryStore: objectstorage.NewMemoryStore(),
		deleteErr:   errors.New("disk full"),
	}
	ctx := context.Background()

	if err := store.MemoryStore.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/a",
		Metadata: objectstorage.Metadata{UserID: "user1"},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}

	handler := NewDeleteUserObjectsHandler(store)

	payload := queue.DeleteUserObjectsPayload{UserID: "user1"}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeDeleteUserObjects, Payload: payloadBytes}

	err := handler.Handle(ctx, job)
	if err == nil {
		t.Fatal("expected error")
	}
	var retryable queue.RetryableError
	if !errors.As(err, &retryable) {
		t.Errorf("error should be RetryableError, got %T", err)
	}
}

func TestDeleteUserObjectsHandler_MetadataMismatchSkips(t *testing.T) {
	store := objectstorage.NewMemoryStore()
	ctx := context.Background()

	// Key says user1 but metadata says user2
	if err := store.Put(ctx, objectstorage.Object{
		Key:      "user1/src1/2026/01/01/orphan",
		Metadata: objectstorage.Metadata{UserID: "user2"},
	}, bytes.NewReader(nil)); err != nil {
		t.Fatalf("Put fixture: %v", err)
	}

	handler := NewDeleteUserObjectsHandler(store)

	payload := queue.DeleteUserObjectsPayload{UserID: "user1"}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeDeleteUserObjects, Payload: payloadBytes}

	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Object should still exist (metadata mismatch = skip)
	_, _, err := store.Get(ctx, "user1/src1/2026/01/01/orphan")
	if err != nil {
		t.Error("object with mismatched metadata should not be deleted")
	}
}

func TestStoreSnapshotHandler_PutRetryable(t *testing.T) {
	store := &putFailStore{err: errors.New("connection refused")}
	contenter := SnapshotContentFunc(func(title, snippet, url string) []byte {
		return []byte("data")
	})

	handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)
	handler.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	payload := queue.StoreSnapshotPayload{
		SourceItemID: "item-1",
		SourceID:     "src-1",
		UserID:       "user-1",
		Platform:     "twitter",
		Title:        "Test",
		Snippet:      "snippet",
		OriginalURL:  "https://example.com/1",
	}
	payloadBytes, _ := json.Marshal(payload)
	job := queue.Job{ID: "j1", Type: queue.JobTypeStoreSnapshot, Payload: payloadBytes}

	err := handler.Handle(context.Background(), job)
	if err == nil {
		t.Fatal("expected error from Put failure")
	}
	var retryable queue.RetryableError
	if !errors.As(err, &retryable) {
		t.Errorf("Put error should be RetryableError, got %T: %v", err, err)
	}
}

func TestStoreSnapshotHandler_NilStore(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewStoreSnapshotHandler with nil store should panic")
		}
	}()
	NewStoreSnapshotHandler(nil, SnapshotContentFunc(func(string, string, string) []byte { return nil }), objectstorage.RetentionRawSnapshot)
}

func TestDeleteUserObjectsHandler_NilStore(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewDeleteUserObjectsHandler with nil store should panic")
		}
	}()
	NewDeleteUserObjectsHandler(nil)
}

// listFailStore always returns an error from ListExpired/ListByPrefix.
type listFailStore struct {
	err error
}

func (s *listFailStore) Put(context.Context, objectstorage.Object, io.Reader) error {
	return nil
}
func (s *listFailStore) Get(context.Context, string) (objectstorage.Object, io.ReadCloser, error) {
	return objectstorage.Object{}, nil, nil
}
func (s *listFailStore) Delete(context.Context, string) error {
	return nil
}
func (s *listFailStore) Head(context.Context, string) (objectstorage.Object, error) {
	return objectstorage.Object{}, nil
}
func (s *listFailStore) ListExpired(context.Context, string, time.Time) ([]objectstorage.Object, error) {
	return nil, s.err
}
func (s *listFailStore) ListByPrefix(context.Context, string) ([]objectstorage.Object, error) {
	return nil, s.err
}

// putFailStore always returns an error from Put.
type putFailStore struct {
	err error
}

func (s *putFailStore) Put(context.Context, objectstorage.Object, io.Reader) error {
	return s.err
}
func (s *putFailStore) Get(context.Context, string) (objectstorage.Object, io.ReadCloser, error) {
	return objectstorage.Object{}, nil, nil
}
func (s *putFailStore) Delete(context.Context, string) error {
	return nil
}
func (s *putFailStore) Head(context.Context, string) (objectstorage.Object, error) {
	return objectstorage.Object{}, nil
}
func (s *putFailStore) ListExpired(context.Context, string, time.Time) ([]objectstorage.Object, error) {
	return nil, nil
}
func (s *putFailStore) ListByPrefix(context.Context, string) ([]objectstorage.Object, error) {
	return nil, nil
}

// deleteFailStore wraps MemoryStore but fails on Delete.
type deleteFailStore struct {
	*objectstorage.MemoryStore
	deleteErr error
}

func (s *deleteFailStore) Delete(ctx context.Context, key string) error {
	return s.deleteErr
}
