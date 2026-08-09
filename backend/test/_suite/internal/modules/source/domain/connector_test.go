package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCollectionConnectorExposesOnlySourceItems(t *testing.T) {
	t.Parallel()

	var _ Connector = (*collectionConnectorFake)(nil)
	result, err := (&collectionConnectorFake{}).Fetch(context.Background(), FetchRequest{Limit: 1})
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ExternalID != "fixture-1" || result.NextCursor != "cursor-2" {
		t.Fatalf("FetchResult = %#v, want source-item-only fixture result", result)
	}
}

func TestEvidenceSnapshotIdentityUsesPayloadAndCollectorProfileOnly(t *testing.T) {
	t.Parallel()

	payload := []byte("publisher response")
	redirects := []string{"https://feed.example/redirected.xml"}
	capturedAt := time.Date(2026, time.August, 9, 4, 5, 6, 0, time.UTC)
	profile, err := NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewEvidenceSnapshot(EvidenceSnapshot{
		Payload: payload, MIMEType: "Application/Atom+XML; Charset=UTF-8", StatusCode: 200,
		RequestedURL: "https://feed.example/source.xml", FinalURL: redirects[0], RedirectChain: redirects,
		CapturedAt: capturedAt, CollectorProfileVersion: profile,
	})
	if err != nil {
		t.Fatalf("NewEvidenceSnapshot(): %v", err)
	}
	wantPayloadHash := sha256.Sum256([]byte("publisher response"))
	if snapshot.Key == "" || snapshot.PayloadSHA256 != hex.EncodeToString(wantPayloadHash[:]) || !snapshot.VerifyPayload() {
		t.Fatalf("snapshot identity = %#v, want verifiable SHA-256", snapshot)
	}
	if snapshot.MIMEType != "application/atom+xml; charset=utf-8" || !snapshot.CapturedAt.Equal(capturedAt) {
		t.Fatalf("normalized metadata = %#v", snapshot)
	}
	laterSnapshot, err := NewEvidenceSnapshot(EvidenceSnapshot{
		Payload: []byte("publisher response"), MIMEType: "application/rss+xml", StatusCode: 206,
		RequestedURL: "https://feed.example/another.xml", FinalURL: "https://feed.example/another.xml",
		CapturedAt: capturedAt.Add(time.Hour), CollectorProfileVersion: profile,
	})
	if err != nil {
		t.Fatalf("NewEvidenceSnapshot(later): %v", err)
	}
	if laterSnapshot.Key != snapshot.Key {
		t.Fatalf("evidence identity changed with receipt metadata: %q != %q", laterSnapshot.Key, snapshot.Key)
	}
	otherProfile, err := NewCollectorProfileVersion("rss-http-feed-go-xml-v2")
	if err != nil {
		t.Fatal(err)
	}
	reprofiled, err := NewEvidenceSnapshot(EvidenceSnapshot{
		Payload: []byte("publisher response"), MIMEType: "application/atom+xml; charset=utf-8", StatusCode: 200,
		RequestedURL: "https://feed.example/source.xml", FinalURL: "https://feed.example/source.xml",
		CapturedAt: capturedAt, CollectorProfileVersion: otherProfile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reprofiled.Key == snapshot.Key {
		t.Fatal("evidence identity collapsed distinct collector profile versions")
	}

	payload[0] = 'X'
	redirects[0] = "https://attacker.example/changed"
	if string(snapshot.Payload) != "publisher response" || snapshot.RedirectChain[0] != "https://feed.example/redirected.xml" || !snapshot.VerifyPayload() {
		t.Fatalf("snapshot changed through caller-owned slices: %#v", snapshot)
	}
	if _, exists := reflect.TypeOf(SourceItem{}).FieldByName("RawPayload"); exists {
		t.Fatal("SourceItem must reference a snapshot, not copy raw response bytes")
	}
}

func TestEvidenceSnapshotRejectsUnverifiableOrUnsafeMetadata(t *testing.T) {
	t.Parallel()

	profile, err := NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		t.Fatal(err)
	}
	valid := EvidenceSnapshot{
		Payload: []byte("response"), MIMEType: "application/rss+xml", StatusCode: 200,
		RequestedURL: "https://feed.example/source.xml", FinalURL: "https://feed.example/source.xml",
		CapturedAt: time.Date(2026, time.August, 9, 4, 5, 6, 0, time.UTC), CollectorProfileVersion: profile,
	}
	tests := []struct {
		name   string
		mutate func(*EvidenceSnapshot)
	}{
		{"missing collector profile", func(input *EvidenceSnapshot) { input.CollectorProfileVersion = CollectorProfileVersion{} }},
		{"missing MIME", func(input *EvidenceSnapshot) { input.MIMEType = "" }},
		{"invalid status", func(input *EvidenceSnapshot) { input.StatusCode = 99 }},
		{"unsafe requested URL", func(input *EvidenceSnapshot) {
			input.RequestedURL = "https://user:secret@feed.example/source.xml"
		}},
		{"non HTTPS final URL", func(input *EvidenceSnapshot) { input.FinalURL = "http://feed.example/source.xml" }},
		{"redirect final mismatch", func(input *EvidenceSnapshot) {
			input.RedirectChain = []string{"https://feed.example/other.xml"}
		}},
		{"missing capture time", func(input *EvidenceSnapshot) { input.CapturedAt = time.Time{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			if _, err := NewEvidenceSnapshot(input); err == nil {
				t.Fatalf("NewEvidenceSnapshot() accepted %#v", input)
			}
		})
	}
}

func TestCollectorProfileVersionMatchesPersistenceContract(t *testing.T) {
	t.Parallel()

	valid := []string{"rss-http-feed-go-xml-v1", "a", "collector.profile_2:stable", strings.Repeat("a", 64)}
	for _, value := range valid {
		if _, err := NewCollectorProfileVersion(value); err != nil {
			t.Fatalf("NewCollectorProfileVersion(%q): %v", value, err)
		}
	}
	invalid := []string{"", "-rss", "RSS-v1", "rss/profile", "rss+profile", "rss profile", strings.Repeat("a", 65)}
	for _, value := range invalid {
		if _, err := NewCollectorProfileVersion(value); err == nil {
			t.Fatalf("NewCollectorProfileVersion(%q) unexpectedly succeeded", value)
		}
	}
}

func TestEvidenceSnapshotErrorsNeverEchoPayloadOrCredentialShapedURL(t *testing.T) {
	t.Parallel()

	secret := "raw-sensitive-material"
	profile, profileErr := NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if profileErr != nil {
		t.Fatal(profileErr)
	}
	_, err := NewEvidenceSnapshot(EvidenceSnapshot{
		Payload: []byte(secret), MIMEType: "application/rss+xml", StatusCode: 200,
		RequestedURL: "https://user:" + secret + "@feed.example/source.xml", FinalURL: "https://feed.example/source.xml",
		CapturedAt: time.Date(2026, time.August, 9, 4, 5, 6, 0, time.UTC), CollectorProfileVersion: profile,
	})
	if err == nil {
		t.Fatal("NewEvidenceSnapshot() error = nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("snapshot validation error leaked sensitive input: %q", err)
	}
}

type collectionConnectorFake struct{}

func (*collectionConnectorFake) Validate(context.Context, SourceConnection) error { return nil }
func (*collectionConnectorFake) Fetch(context.Context, FetchRequest) (FetchResult, error) {
	return FetchResult{Items: []SourceItem{{SourceCode: "rss", ExternalID: "fixture-1", ContentType: "article"}}, NextCursor: "cursor-2"}, nil
}
func (*collectionConnectorFake) Health(context.Context, SourceConnection) HealthResult {
	return HealthResult{Healthy: true}
}
