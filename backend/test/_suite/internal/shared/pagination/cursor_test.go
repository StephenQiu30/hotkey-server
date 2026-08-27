package pagination

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

const testCursorSecret = "pagination-cursor-secret-for-tests-32-bytes"

func TestCodecBindsSignedCursorToVersionSortDirectionAndFilter(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	codec, err := NewCodec(testCursorSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}
	codec.now = func() time.Time { return now }

	encoded, err := codec.Encode("id", false, "monitor=1", 42)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	cursor, err := codec.Decode(encoded, "id", false, "monitor=1")
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if cursor.ID != 42 || !cursor.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("cursor = %#v", cursor)
	}
	if _, err := codec.Decode(encoded, "id", false, "monitor=2"); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("filter mismatch error = %v, want stale cursor", err)
	}
	if _, err := codec.Decode(encoded, "id", true, "monitor=1"); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("direction mismatch error = %v, want stale cursor", err)
	}
	if _, err := codec.Decode("not-a-cursor", "id", false, "monitor=1"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v, want invalid cursor", err)
	}
}

func TestCodecRejectsTamperingWrongKeyAndExpiredCursor(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	codec, err := NewCodec(testCursorSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return now }
	encoded, err := codec.Encode("id", true, "user:7", 99)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		t.Fatalf("signed cursor parts = %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-2] ^= 1
	tampered := base64.RawURLEncoding.EncodeToString(payload) + "." + parts[1]
	if _, err := codec.Decode(tampered, "id", true, "user:7"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("tampered cursor error = %v, want invalid cursor", err)
	}

	other, err := NewCodec("different-pagination-cursor-secret-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	other.now = codec.now
	if _, err := other.Decode(encoded, "id", true, "user:7"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("wrong-key cursor error = %v, want invalid cursor", err)
	}

	codec.now = func() time.Time { return now.Add(time.Minute) }
	if _, err := codec.Decode(encoded, "id", true, "user:7"); !errors.Is(err, ErrExpiredCursor) {
		t.Fatalf("expired cursor error = %v, want expired cursor", err)
	}
}

func TestCodecRejectsUnsafeConstructionAndUnboundedCursorFields(t *testing.T) {
	for _, candidate := range []struct {
		secret string
		ttl    time.Duration
	}{
		{secret: "short", ttl: time.Minute},
		{secret: testCursorSecret, ttl: 0},
		{secret: testCursorSecret, ttl: 25 * time.Hour},
	} {
		if _, err := NewCodec(candidate.secret, candidate.ttl); err == nil {
			t.Fatalf("NewCodec(%q, %s) accepted unsafe configuration", candidate.secret, candidate.ttl)
		}
	}
	codec, err := NewCodec(testCursorSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []struct {
		sort, filter string
		id           int64
	}{
		{sort: "", filter: "scope", id: 1},
		{sort: "id; DROP TABLE", filter: "scope", id: 1},
		{sort: "id", filter: strings.Repeat("x", 257), id: 1},
		{sort: "id", filter: "scope", id: 0},
	} {
		if _, err := codec.Encode(candidate.sort, false, candidate.filter, candidate.id); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("Encode(%q,%q,%d) error = %v", candidate.sort, candidate.filter, candidate.id, err)
		}
	}
	if cursor, err := codec.Decode("", "id", false, "scope"); err != nil || cursor != (Cursor{}) {
		t.Fatalf("empty cursor = %#v / %v", cursor, err)
	}
}
