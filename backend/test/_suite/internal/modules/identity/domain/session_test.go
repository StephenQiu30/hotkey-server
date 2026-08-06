package domain

import (
	"testing"
	"time"
)

func TestNewSessionUsesThirtyDayAbsoluteExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	session := NewSession(42, "family-1", now)

	if got, want := session.AbsoluteExpiresAt, now.Add(30*24*time.Hour); !got.Equal(want) {
		t.Errorf("AbsoluteExpiresAt = %s, want %s", got, want)
	}
}

func TestSessionRefreshExpiryIsSevenDaysButNeverPastAbsoluteExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	session := NewSession(42, "family-1", now)
	if got, want := session.RefreshExpiry(now.Add(2*24*time.Hour)), now.Add(9*24*time.Hour); !got.Equal(want) {
		t.Errorf("RefreshExpiry() = %s, want %s", got, want)
	}

	if got, want := session.RefreshExpiry(now.Add(29*24*time.Hour)), session.AbsoluteExpiresAt; !got.Equal(want) {
		t.Errorf("RefreshExpiry() near cap = %s, want %s", got, want)
	}
}
