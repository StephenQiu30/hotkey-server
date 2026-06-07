package objectstorage

import (
	"testing"
	"time"
)

func TestBuildKey(t *testing.T) {
	tests := []struct {
		name         string
		sourceID     string
		sourceItemID string
		t            time.Time
		want         string
	}{
		{
			name:         "standard key format",
			sourceID:     "src-abc",
			sourceItemID: "item-123",
			t:            time.Date(2026, 6, 7, 15, 30, 0, 0, time.UTC),
			want:         "src-abc/2026/06/07/item-123",
		},
		{
			name:         "date boundary",
			sourceID:     "src-x",
			sourceItemID: "item-y",
			t:            time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			want:         "src-x/2026/01/01/item-y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildKey(tt.sourceID, tt.sourceItemID, tt.t)
			if got != tt.want {
				t.Errorf("BuildKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultExpiry(t *testing.T) {
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		retention RetentionPolicy
		wantNil   bool
		wantDur   time.Duration
	}{
		{
			name:      "raw_snapshot expires in 30 days",
			retention: RetentionRawSnapshot,
			wantNil:   false,
			wantDur:   30 * 24 * time.Hour,
		},
		{
			name:      "derived never expires",
			retention: RetentionDerived,
			wantNil:   true,
		},
		{
			name:      "permanent never expires",
			retention: RetentionPermanent,
			wantNil:   true,
		},
		{
			name:      "unknown defaults to 30 days",
			retention: RetentionPolicy("unknown"),
			wantNil:   false,
			wantDur:   30 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultExpiry(tt.retention, now)
			if tt.wantNil {
				if got != nil {
					t.Errorf("DefaultExpiry() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("DefaultExpiry() = nil, want non-nil")
			}
			if got.Sub(now) != tt.wantDur {
				t.Errorf("DefaultExpiry() = %v, want %v after now", got, tt.wantDur)
			}
		})
	}
}

func TestMetadata_ExpiryRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	exp := now.Add(30 * 24 * time.Hour)
	meta := Metadata{
		SourceItemID: "item-1",
		SourceID:     "src-1",
		UserID:       "user-1",
		Platform:     "rss",
		Retention:    RetentionRawSnapshot,
		ExpiresAt:    &exp,
		OriginalURL:  "https://example.com/article",
	}

	if meta.ExpiresAt == nil {
		t.Fatal("ExpiresAt should not be nil for raw_snapshot")
	}
	if meta.ExpiresAt.Sub(now) != 30*24*time.Hour {
		t.Errorf("ExpiresAt should be 30 days from now, got %v", meta.ExpiresAt.Sub(now))
	}
	if meta.Retention != RetentionRawSnapshot {
		t.Errorf("Retention = %q, want %q", meta.Retention, RetentionRawSnapshot)
	}
}
