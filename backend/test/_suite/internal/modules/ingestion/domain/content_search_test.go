package domain

import (
	"strings"
	"testing"
	"time"
)

func TestContentListQueryValidatesSearchShapeAndBindsCursorFingerprint(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	sourceID, monitorID := int64(3), int64(7)
	decision := MatchDecisionAccepted
	base := ContentListQuery{
		Limit: 20, Keyword: "发布", SourceConnectionID: &sourceID,
		PublishedFrom: &from, PublishedTo: &to, MonitorID: &monitorID,
		Decision: &decision, Sort: ContentSortRelevance,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	fingerprint, err := base.ShapeFingerprint()
	if err != nil || len(fingerprint) != 64 {
		t.Fatalf("ShapeFingerprint() = %q/%v", fingerprint, err)
	}
	equivalent := base
	equivalent.Cursor, equivalent.Limit = "opaque", 100
	if got, _ := equivalent.ShapeFingerprint(); got != fingerprint {
		t.Fatalf("cursor/limit changed fingerprint: %q != %q", got, fingerprint)
	}
	for _, mutate := range []func(*ContentListQuery){
		func(query *ContentListQuery) { query.Keyword = "另一条" },
		func(query *ContentListQuery) { value := int64(4); query.SourceConnectionID = &value },
		func(query *ContentListQuery) { value := from.Add(time.Hour); query.PublishedFrom = &value },
		func(query *ContentListQuery) { value := to.Add(time.Hour); query.PublishedTo = &value },
		func(query *ContentListQuery) { value := int64(8); query.MonitorID = &value },
		func(query *ContentListQuery) { value := MatchDecisionReview; query.Decision = &value },
		func(query *ContentListQuery) { query.Sort = ContentSortLatest },
	} {
		changed := base
		mutate(&changed)
		if got, _ := changed.ShapeFingerprint(); got == fingerprint {
			t.Fatalf("semantic change did not change fingerprint: %#v", changed)
		}
	}
}

func TestContentListQueryRejectsInvalidPublicCombinations(t *testing.T) {
	from := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	zero := int64(0)
	decision := MatchDecisionAccepted
	for _, query := range []ContentListQuery{
		{Limit: 0, Sort: ContentSortLatest},
		{Limit: 20, Sort: ContentSort("unknown")},
		{Limit: 20, Keyword: strings.Repeat("界", 101), Sort: ContentSortLatest},
		{Limit: 20, SourceConnectionID: &zero, Sort: ContentSortLatest},
		{Limit: 20, PublishedFrom: &from, PublishedTo: &to, Sort: ContentSortLatest},
		{Limit: 20, Sort: ContentSortRelevance},
		{Limit: 20, Decision: &decision, Sort: ContentSortLatest},
	} {
		if err := query.Validate(); err == nil {
			t.Fatalf("Validate() accepted %#v", query)
		}
	}
}

func TestContentListQueryAcceptsAllHotspotSorts(t *testing.T) {
	monitorID := int64(7)
	for _, sortValue := range []ContentSort{ContentSortDiscovered, ContentSortPublished, ContentSortImportance, ContentSortHeat} {
		if err := (ContentListQuery{Limit: 20, Sort: sortValue}).Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v", sortValue, err)
		}
	}
	if err := (ContentListQuery{Limit: 20, Sort: ContentSortRelevance, MonitorID: &monitorID}).Validate(); err != nil {
		t.Fatalf("Validate(relevance) error = %v", err)
	}
	base := ContentListQuery{Limit: 20, Sort: ContentSortHeat}
	fingerprint, _ := base.ShapeFingerprint()
	base.IncludeSummary = true
	if withSummary, _ := base.ShapeFingerprint(); withSummary != fingerprint {
		t.Fatalf("summary-only query changed item cursor fingerprint")
	}
}
