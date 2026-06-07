package objectstorage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound      = errors.New("object not found")
	ErrAlreadyExists = errors.New("object already exists")
	ErrInvalidInput  = errors.New("invalid input")
)

type RetentionPolicy string

const (
	RetentionRawSnapshot RetentionPolicy = "raw_snapshot" // 30-day default
	RetentionDerived     RetentionPolicy = "derived"      // long-term
	RetentionPermanent   RetentionPolicy = "permanent"    // never expires
)

type Object struct {
	Key         string
	Bucket      string
	ContentType string
	Size        int64
	ETag        string
	Metadata    Metadata
	CreatedAt   time.Time
}

type Metadata struct {
	SourceItemID string
	SourceID     string
	UserID       string
	Platform     string
	Retention    RetentionPolicy
	ExpiresAt    *time.Time
	OriginalURL  string
}

type Store interface {
	Put(ctx context.Context, obj Object, reader io.Reader) error
	Get(ctx context.Context, key string) (Object, io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Head(ctx context.Context, key string) (Object, error)
	ListExpired(ctx context.Context, bucket string, before time.Time) ([]Object, error)
	ListByPrefix(ctx context.Context, prefix string) ([]Object, error)
}

type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	ExpiresAt   *time.Time
	Retention   RetentionPolicy
}

// BuildKey constructs a deterministic object key from metadata.
// Format: {user_id}/{source_id}/{YYYY}/{MM}/{DD}/{source_item_id}
func BuildKey(userID, sourceID, sourceItemID string, t time.Time) string {
	date := t.UTC().Format("2006/01/02")
	return userID + "/" + sourceID + "/" + date + "/" + sourceItemID
}

// DefaultExpiry computes the expiry time based on retention policy.
func DefaultExpiry(retention RetentionPolicy, now time.Time) *time.Time {
	switch retention {
	case RetentionRawSnapshot:
		exp := now.UTC().Add(30 * 24 * time.Hour)
		return &exp
	case RetentionDerived, RetentionPermanent:
		return nil
	default:
		exp := now.UTC().Add(30 * 24 * time.Hour)
		return &exp
	}
}
