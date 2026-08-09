package domain

import (
	"strings"
	"testing"
	"time"
)

func TestDocumentVersionCandidatePreservesBodyOriginAndCompleteness(t *testing.T) {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		origin       BodyOrigin
		completeness BodyCompleteness
		body         string
		wantError    bool
	}{
		{"atom_content_full", BodyOriginFeedContent, BodyCompletenessFull, "complete feed body", false},
		{"rss_description_summary", BodyOriginFeedSummary, BodyCompletenessSummary, "publisher summary", false},
		{"search_snippet", BodyOriginSearchSnippet, BodyCompletenessSnippet, "provider snippet", false},
		{"platform_post", BodyOriginPlatformPost, BodyCompletenessFull, "complete post payload", false},
		{"metadata_only", BodyOriginFeedSummary, BodyCompletenessMetadataOnly, "", false},
		{"summary_cannot_be_full", BodyOriginFeedSummary, BodyCompletenessFull, "summary", true},
		{"snippet_cannot_be_summary", BodyOriginSearchSnippet, BodyCompletenessSummary, "snippet", true},
		{"metadata_cannot_have_body", BodyOriginFeedContent, BodyCompletenessMetadataOnly, "body", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := DocumentVersionCandidate{
				DocumentID: 1, SourceObservationID: 2,
				BodyOrigin: test.origin, Completeness: test.completeness, Body: test.body,
				Language: "zh-CN", ExtractorVersion: "feed-v1",
				ExtractorProfileVersion: "feed-profile-v1",
				ExtractorProfileSHA256:  strings.Repeat("a", 64), QualityScore: pointer(92.5), CapturedAt: now,
			}
			_, err := candidate.Normalize()
			if (err != nil) != test.wantError {
				t.Fatalf("Normalize() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestDocumentVersionCandidateBuildsStableIdentity(t *testing.T) {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	base := DocumentVersionCandidate{
		DocumentID: 7, SourceObservationID: 11,
		BodyOrigin: BodyOriginFeedContent, Completeness: BodyCompletenessFull,
		Body: "  First\r\n\r\nBody  ", Language: "en", ExtractorVersion: "atom-v1",
		ExtractorProfileVersion: "atom-profile-v1",
		ExtractorProfileSHA256:  strings.Repeat("b", 64), CapturedAt: now,
	}
	first, err := base.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	second, err := base.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionKey == "" || first.VersionKey != second.VersionKey || first.ContentSHA256 != second.ContentSHA256 {
		t.Fatalf("stable identity = %#v / %#v", first, second)
	}
	if first.Body != "First\n\nBody" || first.WordCount != 2 {
		t.Fatalf("normalized body = %q, words = %d", first.Body, first.WordCount)
	}

	changed := base
	changed.Body = "First\n\nChanged body"
	third, err := changed.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionKey == third.VersionKey || first.ContentSHA256 == third.ContentSHA256 {
		t.Fatal("changed body reused the immutable version identity")
	}

	laterObservation := base
	laterObservation.SourceObservationID++
	laterObservation.CapturedAt = laterObservation.CapturedAt.Add(time.Hour)
	fourth, err := laterObservation.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionKey == fourth.VersionKey || first.ContentSHA256 != fourth.ContentSHA256 {
		t.Fatal("a later observation did not retain distinct provenance identity")
	}

	changedProfile := base
	changedProfile.ExtractorProfileVersion = "atom-profile-v2"
	fifth, err := changedProfile.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionKey == fifth.VersionKey || first.ContentSHA256 != fifth.ContentSHA256 {
		t.Fatal("a changed extractor profile version reused the immutable version identity")
	}
}

func TestDocumentVersionCandidateUsesNFCAndPreservesUnknownQuality(t *testing.T) {
	base := DocumentVersionCandidate{
		DocumentID: 9, SourceObservationID: 12, BodyOrigin: BodyOriginFeedContent,
		Completeness: BodyCompletenessFull, Body: "Cafe\u0301", Language: "fr",
		ExtractorVersion: "atom-v1", ExtractorProfileVersion: "atom-profile-v1",
		ExtractorProfileSHA256: strings.Repeat("c", 64), CapturedAt: time.Now(),
	}
	normalized, err := base.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Body != "Café" || normalized.QualityScore != nil {
		t.Fatalf("normalized body/quality = %q / %v", normalized.Body, normalized.QualityScore)
	}
	scored := base
	scored.QualityScore = pointer(87.567)
	scoredVersion, err := scored.Normalize()
	if err != nil || scoredVersion.QualityScore == nil || *scoredVersion.QualityScore != 87.57 {
		t.Fatalf("quantized quality = %v / %v", scoredVersion.QualityScore, err)
	}
	invalid := base
	invalid.QualityScore = pointer(100.01)
	if _, err := invalid.Normalize(); err == nil {
		t.Fatal("out-of-range quality score was accepted")
	}
}

func TestDocumentLifecycleAllowsOnlyAuditedSagaTransitions(t *testing.T) {
	allowed := []struct {
		from DocumentLifecycleState
		to   DocumentLifecycleState
	}{
		{DocumentPolicyPending, DocumentDerivedPending},
		{DocumentDerivedPending, DocumentDerivedAvailable},
		{DocumentDerivedAvailable, DocumentReadable},
		{DocumentPolicyPending, DocumentPolicyBlocked},
		{DocumentDerivedAvailable, DocumentQuarantined},
		{DocumentReadable, DocumentTombstoned},
		{DocumentReadable, DocumentRetentionBlocked},
		{DocumentDerivedPending, DocumentDerivedFailed},
		{DocumentDerivedFailed, DocumentDerivedPending},
		{DocumentPolicyBlocked, DocumentDerivedPending},
		{DocumentPolicyBlocked, DocumentReadable},
	}
	for _, transition := range allowed {
		if err := ValidateDocumentTransition(transition.from, transition.to); err != nil {
			t.Errorf("transition %s -> %s: %v", transition.from, transition.to, err)
		}
	}
	for _, transition := range [][2]DocumentLifecycleState{
		{DocumentPolicyPending, DocumentReadable},
		{DocumentPolicyPending, DocumentRawPending},
		{DocumentReadable, DocumentRawAvailable},
		{DocumentTombstoned, DocumentReadable},
		{DocumentQuarantined, DocumentDerivedPending},
	} {
		if err := ValidateDocumentTransition(transition[0], transition[1]); err == nil || !strings.Contains(err.Error(), "transition") {
			t.Errorf("invalid transition %s -> %s was accepted: %v", transition[0], transition[1], err)
		}
	}
}

func pointer[T any](value T) *T { return &value }
