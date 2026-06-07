package objectstorage

import (
	"testing"
	"time"
)

func TestBuildKey_IncludesUserID(t *testing.T) {
	userID := "user-123"
	sourceID := "src-456"
	itemID := "item-789"
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	key := BuildKey(userID, sourceID, itemID, ts)

	// Key must start with userID so ListByPrefix(userID+"/") works for user deletion
	want := "user-123/src-456/2026/06/07/item-789"
	if key != want {
		t.Errorf("BuildKey() = %q, want %q", key, want)
	}
}

func TestBuildKey_UserIDAsTopLevelPrefix(t *testing.T) {
	userID := "u1"
	sourceID := "s1"
	itemID := "i1"
	ts := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	key := BuildKey(userID, sourceID, itemID, ts)

	prefix := userID + "/"
	if len(key) < len(prefix) || key[:len(prefix)] != prefix {
		t.Errorf("BuildKey() = %q does not start with user prefix %q", key, prefix)
	}
}

func TestBuildKey_EmptyUserID(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	key := BuildKey("", "src", "item", ts)
	// Even with empty userID, key structure should be consistent
	want := "/src/2026/01/01/item"
	if key != want {
		t.Errorf("BuildKey(empty userID) = %q, want %q", key, want)
	}
}

func TestDefaultExpiry_RawSnapshot(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	exp := DefaultExpiry(RetentionRawSnapshot, now)

	if exp == nil {
		t.Fatal("DefaultExpiry(raw_snapshot) returned nil, want non-nil")
	}
	want := now.Add(30 * 24 * time.Hour)
	if !exp.Equal(want) {
		t.Errorf("DefaultExpiry(raw_snapshot) = %v, want %v", exp, want)
	}
}

func TestDefaultExpiry_Derived(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	exp := DefaultExpiry(RetentionDerived, now)

	if exp != nil {
		t.Errorf("DefaultExpiry(derived) = %v, want nil", exp)
	}
}

func TestDefaultExpiry_Permanent(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	exp := DefaultExpiry(RetentionPermanent, now)

	if exp != nil {
		t.Errorf("DefaultExpiry(permanent) = %v, want nil", exp)
	}
}

func TestDefaultExpiry_Unknown(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	exp := DefaultExpiry(RetentionPolicy("unknown"), now)

	if exp == nil {
		t.Fatal("DefaultExpiry(unknown) returned nil, want fallback expiry")
	}
	want := now.Add(30 * 24 * time.Hour)
	if !exp.Equal(want) {
		t.Errorf("DefaultExpiry(unknown) = %v, want %v", exp, want)
	}
}

func TestMetadata_Fields(t *testing.T) {
	exp := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	meta := Metadata{
		SourceItemID: "item-1",
		SourceID:     "src-1",
		UserID:       "user-1",
		Platform:     "twitter",
		Retention:    RetentionRawSnapshot,
		ExpiresAt:    &exp,
		OriginalURL:  "https://example.com/post/1",
	}

	if meta.SourceItemID != "item-1" {
		t.Errorf("SourceItemID = %q, want %q", meta.SourceItemID, "item-1")
	}
	if meta.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", meta.UserID, "user-1")
	}
	if meta.ExpiresAt == nil || !meta.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt = %v, want %v", meta.ExpiresAt, exp)
	}
}
