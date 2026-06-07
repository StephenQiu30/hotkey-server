package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/objectstorage"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

// ObjectStore is the worker's interface to object storage.
type ObjectStore interface {
	Put(ctx context.Context, obj objectstorage.Object, reader io.Reader) error
	Delete(ctx context.Context, key string) error
	ListExpired(ctx context.Context, bucket string, before time.Time) ([]objectstorage.Object, error)
	ListByPrefix(ctx context.Context, prefix string) ([]objectstorage.Object, error)
}

// SnapshotContenter builds a snapshot payload from content fields.
type SnapshotContenter interface {
	SnapshotContent(title, snippet, originalURL string) []byte
}

// SnapshotContentFunc is a function adapter for SnapshotContenter.
type SnapshotContentFunc func(title, snippet, originalURL string) []byte

func (f SnapshotContentFunc) SnapshotContent(title, snippet, originalURL string) []byte {
	return f(title, snippet, originalURL)
}

// StoreSnapshotHandler stores a content snapshot in object storage.
type StoreSnapshotHandler struct {
	store     ObjectStore
	contenter SnapshotContenter
	now       func() time.Time
	retention objectstorage.RetentionPolicy
}

func NewStoreSnapshotHandler(store ObjectStore, contenter SnapshotContenter, retention objectstorage.RetentionPolicy) *StoreSnapshotHandler {
	if store == nil {
		panic("store snapshot handler requires store")
	}
	if contenter == nil {
		panic("store snapshot handler requires contenter")
	}
	return &StoreSnapshotHandler{
		store:     store,
		contenter: contenter,
		now:       time.Now,
		retention: retention,
	}
}

func (h *StoreSnapshotHandler) Handle(ctx context.Context, job queue.Job) error {
	var payload queue.StoreSnapshotPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if payload.SourceItemID == "" || payload.SourceID == "" {
		return errors.New("store_snapshot payload missing source_item_id or source_id")
	}

	now := h.now().UTC()
	key := objectstorage.BuildKey(payload.UserID, payload.SourceID, payload.SourceItemID, now)
	data := h.contenter.SnapshotContent(payload.Title, payload.Snippet, payload.OriginalURL)

	expiresAt := objectstorage.DefaultExpiry(h.retention, now)

	obj := objectstorage.Object{
		Key:         key,
		ContentType: "text/plain; charset=utf-8",
		Size:        int64(len(data)),
		Metadata: objectstorage.Metadata{
			SourceItemID: payload.SourceItemID,
			SourceID:     payload.SourceID,
			UserID:       payload.UserID,
			Platform:     payload.Platform,
			Retention:    h.retention,
			ExpiresAt:    expiresAt,
			OriginalURL:  payload.OriginalURL,
		},
		CreatedAt: now,
	}

	if err := h.store.Put(ctx, obj, bytes.NewReader(data)); err != nil {
		return queue.NewRetryableError(err)
	}
	return nil
}

// CleanupExpiredObjectsHandler removes expired objects from storage.
type CleanupExpiredObjectsHandler struct {
	store  ObjectStore
	bucket string
	now    func() time.Time
}

func NewCleanupExpiredObjectsHandler(store ObjectStore, bucket string) *CleanupExpiredObjectsHandler {
	if store == nil {
		panic("cleanup expired objects handler requires store")
	}
	return &CleanupExpiredObjectsHandler{
		store:  store,
		bucket: bucket,
		now:    time.Now,
	}
}

func (h *CleanupExpiredObjectsHandler) Handle(ctx context.Context, job queue.Job) error {
	var payload queue.CleanupExpiredObjectsPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	bucket := payload.Bucket
	if bucket == "" {
		bucket = h.bucket
	}

	now := h.now().UTC()
	expired, err := h.store.ListExpired(ctx, bucket, now)
	if err != nil {
		return queue.NewRetryableError(err)
	}

	for _, obj := range expired {
		if err := h.store.Delete(ctx, obj.Key); err != nil {
			return queue.NewRetryableError(err)
		}
	}
	return nil
}

// DeleteUserObjectsHandler deletes all objects for a user.
type DeleteUserObjectsHandler struct {
	store ObjectStore
}

func NewDeleteUserObjectsHandler(store ObjectStore) *DeleteUserObjectsHandler {
	if store == nil {
		panic("delete user objects handler requires store")
	}
	return &DeleteUserObjectsHandler{store: store}
}

func (h *DeleteUserObjectsHandler) Handle(ctx context.Context, job queue.Job) error {
	var payload queue.DeleteUserObjectsPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if payload.UserID == "" {
		return errors.New("delete_user_objects payload missing user_id")
	}

	objects, err := h.store.ListByPrefix(ctx, payload.UserID+"/")
	if err != nil {
		return queue.NewRetryableError(err)
	}

	for _, obj := range objects {
		// Double-check metadata to guard against key/metadata mismatches
		if obj.Metadata.UserID != payload.UserID {
			continue
		}
		if err := h.store.Delete(ctx, obj.Key); err != nil {
			return queue.NewRetryableError(err)
		}
	}
	return nil
}
