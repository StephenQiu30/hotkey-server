package domain

import (
	"testing"
	"time"
)

func TestBuildEventFingerprintUsesVersionedFactsAndDailyBucket(t *testing.T) {
	first, ok := BuildEventFingerprint(EventFingerprintFacts{
		EntityTerms: []string{"  Acme ", "Acme"}, ActionTerms: []string{"Launch"}, Regions: []string{"CN"},
		PublishedAt: time.Date(2026, time.July, 17, 9, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
	})
	if !ok || first.Version != EventFingerprintVersion || len(first.Value) != 64 || !first.TimeBucket.Equal(time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("BuildEventFingerprint() = %#v/%t", first, ok)
	}
	sameDay, ok := BuildEventFingerprint(EventFingerprintFacts{EntityTerms: []string{"acme"}, ActionTerms: []string{"launch"}, Regions: []string{"cn"}, PublishedAt: time.Date(2026, time.July, 17, 23, 59, 0, 0, time.UTC)})
	if !ok || sameDay.Value != first.Value {
		t.Fatalf("same-day normalized facts = %#v/%t, want %q", sameDay, ok, first.Value)
	}
	nextDay, ok := BuildEventFingerprint(EventFingerprintFacts{EntityTerms: []string{"acme"}, ActionTerms: []string{"launch"}, Regions: []string{"cn"}, PublishedAt: time.Date(2026, time.July, 18, 0, 0, 0, 0, time.UTC)})
	if !ok || nextDay.Value == first.Value {
		t.Fatalf("next-day facts = %#v/%t, want a different time-bucket hash", nextDay, ok)
	}
}

func TestBuildEventFingerprintRefusesBroadFacts(t *testing.T) {
	if _, ok := BuildEventFingerprint(EventFingerprintFacts{EntityTerms: []string{"acme"}, PublishedAt: time.Now()}); ok {
		t.Fatal("BuildEventFingerprint() accepted a fingerprint without an action term")
	}
}
