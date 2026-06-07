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

type mockObjectStore struct {
	putErr        error
	deleteErr     error
	listExpired   []objectstorage.Object
	listExpiredErr error
	listByPrefix  []objectstorage.Object
	listByPrefixErr error
	putCalls      []putCall
	deleteCalls   []string
}

type putCall struct {
	key      string
	userID   string
	data     []byte
}

func (m *mockObjectStore) Put(_ context.Context, obj objectstorage.Object, reader io.Reader) error {
	if m.putErr != nil {
		return m.putErr
	}
	data, _ := io.ReadAll(reader)
	m.putCalls = append(m.putCalls, putCall{
		key:    obj.Key,
		userID: obj.Metadata.UserID,
		data:   data,
	})
	return nil
}

func (m *mockObjectStore) Delete(_ context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deleteCalls = append(m.deleteCalls, key)
	return nil
}

func (m *mockObjectStore) ListExpired(_ context.Context, _ string, _ time.Time) ([]objectstorage.Object, error) {
	if m.listExpiredErr != nil {
		return nil, m.listExpiredErr
	}
	return m.listExpired, nil
}

func (m *mockObjectStore) ListByPrefix(_ context.Context, prefix string) ([]objectstorage.Object, error) {
	if m.listByPrefixErr != nil {
		return nil, m.listByPrefixErr
	}
	return m.listByPrefix, nil
}

type mockSnapshotContenter struct {
	data []byte
}

func (m *mockSnapshotContenter) SnapshotContent(_, _, _ string) []byte {
	return m.data
}

func TestStoreSnapshotHandler(t *testing.T) {
	fixedTime := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	t.Run("stores snapshot with correct metadata", func(t *testing.T) {
		store := &mockObjectStore{}
		contenter := &mockSnapshotContenter{data: []byte("snapshot content")}

		handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)
		handler.now = func() time.Time { return fixedTime }

		payload := queue.StoreSnapshotPayload{
			SourceItemID: "item-123",
			SourceID:     "src-abc",
			UserID:       "user-1",
			Platform:     "rss",
			Title:        "Test Title",
			Snippet:      "Test snippet",
			OriginalURL:  "https://example.com/article",
		}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeStoreSnapshot,
			Payload: payloadBytes,
		})
		if err != nil {
			t.Fatalf("Handle() error = %v", err)
		}

		if len(store.putCalls) != 1 {
			t.Fatalf("expected 1 put call, got %d", len(store.putCalls))
		}

		call := store.putCalls[0]
		expectedKey := "src-abc/2026/06/07/item-123"
		if call.key != expectedKey {
			t.Errorf("key = %q, want %q", call.key, expectedKey)
		}
		if call.userID != "user-1" {
			t.Errorf("userID = %q, want %q", call.userID, "user-1")
		}
		if !bytes.Equal(call.data, []byte("snapshot content")) {
			t.Errorf("data = %q, want %q", call.data, "snapshot content")
		}
	})

	t.Run("returns error for missing source_item_id", func(t *testing.T) {
		store := &mockObjectStore{}
		contenter := &mockSnapshotContenter{data: []byte("data")}

		handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)
		handler.now = func() time.Time { return fixedTime }

		payload := queue.StoreSnapshotPayload{
			SourceID: "src-abc",
		}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeStoreSnapshot,
			Payload: payloadBytes,
		})
		if err == nil {
			t.Fatal("Handle() expected error for missing source_item_id")
		}
	})

	t.Run("returns error for missing source_id", func(t *testing.T) {
		store := &mockObjectStore{}
		contenter := &mockSnapshotContenter{data: []byte("data")}

		handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)
		handler.now = func() time.Time { return fixedTime }

		payload := queue.StoreSnapshotPayload{
			SourceItemID: "item-123",
		}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeStoreSnapshot,
			Payload: payloadBytes,
		})
		if err == nil {
			t.Fatal("Handle() expected error for missing source_id")
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := &mockObjectStore{putErr: errors.New("storage unavailable")}
		contenter := &mockSnapshotContenter{data: []byte("data")}

		handler := NewStoreSnapshotHandler(store, contenter, objectstorage.RetentionRawSnapshot)
		handler.now = func() time.Time { return fixedTime }

		payload := queue.StoreSnapshotPayload{
			SourceItemID: "item-123",
			SourceID:     "src-abc",
		}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeStoreSnapshot,
			Payload: payloadBytes,
		})
		if err == nil {
			t.Fatal("Handle() expected error")
		}
	})
}

func TestCleanupExpiredObjectsHandler(t *testing.T) {
	t.Run("deletes expired objects", func(t *testing.T) {
		store := &mockObjectStore{
			listExpired: []objectstorage.Object{
				{Key: "expired-1"},
				{Key: "expired-2"},
			},
		}

		handler := NewCleanupExpiredObjectsHandler(store, "test-bucket")
		handler.now = func() time.Time { return time.Now() }

		payload := queue.CleanupExpiredObjectsPayload{Bucket: "test-bucket"}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeCleanupExpiredObjects,
			Payload: payloadBytes,
		})
		if err != nil {
			t.Fatalf("Handle() error = %v", err)
		}

		if len(store.deleteCalls) != 2 {
			t.Errorf("expected 2 delete calls, got %d", len(store.deleteCalls))
		}
	})

	t.Run("returns retryable error when ListExpired fails", func(t *testing.T) {
		store := &mockObjectStore{
			listExpiredErr: errors.New("connection refused"),
		}

		handler := NewCleanupExpiredObjectsHandler(store, "test-bucket")
		handler.now = func() time.Time { return time.Now() }

		payload := queue.CleanupExpiredObjectsPayload{Bucket: "test-bucket"}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeCleanupExpiredObjects,
			Payload: payloadBytes,
		})
		if err == nil {
			t.Fatal("Handle() expected error")
		}
		if !queue.IsRetryable(err) {
			t.Error("error should be retryable")
		}
	})
}

func TestDeleteUserObjectsHandler(t *testing.T) {
	t.Run("deletes all objects for user", func(t *testing.T) {
		store := &mockObjectStore{
			listByPrefix: []objectstorage.Object{
				{Key: "user-1/item-1", Metadata: objectstorage.Metadata{UserID: "user-1"}},
				{Key: "user-1/item-2", Metadata: objectstorage.Metadata{UserID: "user-1"}},
				{Key: "user-1/item-3", Metadata: objectstorage.Metadata{UserID: "user-2"}}, // different user
			},
		}

		handler := NewDeleteUserObjectsHandler(store)

		payload := queue.DeleteUserObjectsPayload{UserID: "user-1"}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeDeleteUserObjects,
			Payload: payloadBytes,
		})
		if err != nil {
			t.Fatalf("Handle() error = %v", err)
		}

		if len(store.deleteCalls) != 2 {
			t.Errorf("expected 2 delete calls, got %d", len(store.deleteCalls))
		}
	})

	t.Run("returns error for missing user_id", func(t *testing.T) {
		store := &mockObjectStore{}
		handler := NewDeleteUserObjectsHandler(store)

		payload := queue.DeleteUserObjectsPayload{}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeDeleteUserObjects,
			Payload: payloadBytes,
		})
		if err == nil {
			t.Fatal("Handle() expected error for missing user_id")
		}
	})

	t.Run("returns retryable error when ListByPrefix fails", func(t *testing.T) {
		store := &mockObjectStore{
			listByPrefixErr: errors.New("connection refused"),
		}

		handler := NewDeleteUserObjectsHandler(store)

		payload := queue.DeleteUserObjectsPayload{UserID: "user-1"}
		payloadBytes, _ := json.Marshal(payload)

		err := handler.Handle(context.Background(), queue.Job{
			Type:    queue.JobTypeDeleteUserObjects,
			Payload: payloadBytes,
		})
		if err == nil {
			t.Fatal("Handle() expected error")
		}
		if !queue.IsRetryable(err) {
			t.Error("error should be retryable")
		}
	})
}
