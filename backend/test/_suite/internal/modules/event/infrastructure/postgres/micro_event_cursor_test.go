package postgres

import (
	"errors"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
)

func TestMicroEventListCursorFreezesRankingAndRejectsChangedQuery(t *testing.T) {
	query := eventapplication.MicroEventListQuery{
		Statuses: []string{"review_pending", "active"}, MonitorID: 7, SourceTypes: []string{"rss", "x"},
		EvidenceStates: []string{"multiple_origins"},
	}
	filter := microEventFilterFingerprint(query)
	cursor := microEventListCursor{
		Sort: "heat", Filter: filter,
		AsOf:    time.Date(2026, time.August, 12, 8, 0, 0, 0, time.UTC),
		HasHeat: true, HeatScore: 73.4,
		HeatWindowEnd:  time.Date(2026, time.August, 12, 7, 59, 0, 0, time.UTC),
		EventStartedAt: time.Date(2026, time.August, 12, 7, 0, 0, 0, time.UTC), ID: 17,
	}
	encoded, err := encodeMicroEventListCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeMicroEventListCursor(encoded, "heat", filter)
	if err != nil || decoded.ID != cursor.ID || decoded.HeatScore != cursor.HeatScore || !decoded.AsOf.Equal(cursor.AsOf) {
		t.Fatalf("decoded cursor = %#v / %v", decoded, err)
	}
	changedQuery := query
	changedQuery.SourceTypes = []string{"hacker_news"}
	for _, mismatch := range []struct{ sort, filter string }{{"latest", filter}, {"heat", microEventFilterFingerprint(changedQuery)}} {
		if _, err := decodeMicroEventListCursor(encoded, mismatch.sort, mismatch.filter); !errors.Is(err, eventapplication.ErrInvalidMicroEventQuery) {
			t.Fatalf("mismatched cursor error = %v", err)
		}
	}
}

func TestMicroEventListCursorRejectsMalformedHeatTuple(t *testing.T) {
	filter := microEventFilterFingerprint(eventapplication.MicroEventListQuery{Statuses: []string{"active"}})
	encoded, err := encodeMicroEventListCursor(microEventListCursor{
		Sort: "heat", Filter: filter, AsOf: time.Now().UTC(), HasHeat: false,
		HeatScore: 42, EventStartedAt: time.Now().UTC(), ID: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeMicroEventListCursor(encoded, "heat", filter); !errors.Is(err, eventapplication.ErrInvalidMicroEventQuery) {
		t.Fatalf("malformed tuple error = %v", err)
	}
}

func TestMicroEventListCursorSupportsRelevanceTuple(t *testing.T) {
	filter := microEventFilterFingerprint(eventapplication.MicroEventListQuery{MonitorID: 9, SourceTypes: []string{"x"}})
	cursor := microEventListCursor{
		Sort: "relevance", Filter: filter, AsOf: time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC),
		HasRelevance: true, RelevanceScore: .83,
		EventStartedAt: time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC), ID: 23,
	}
	encoded, err := encodeMicroEventListCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeMicroEventListCursor(encoded, "relevance", filter)
	if err != nil || !decoded.HasRelevance || decoded.RelevanceScore != cursor.RelevanceScore {
		t.Fatalf("decoded cursor = %#v / %v", decoded, err)
	}
}
