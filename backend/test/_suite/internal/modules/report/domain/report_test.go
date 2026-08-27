package domain

import (
	"errors"
	"testing"
	"time"
)

func TestPeriodUsesSubscriberTimezone(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	period, err := PeriodFor(time.Date(2026, 7, 16, 23, 30, 0, 0, time.UTC), ReportDaily, location)
	if err != nil {
		t.Fatal(err)
	}
	if period.Start.Day() != 17 {
		t.Fatalf("period start = %v", period.Start)
	}
}
func TestSortItemsIsDeterministic(t *testing.T) {
	items := SortItems([]Item{{EventID: 2, Title: "b", HeatScore: 90}, {EventID: 1, Title: "a", HeatScore: 90}})
	if items[0].EventID != 1 || items[0].Rank != 1 {
		t.Fatalf("items = %#v", items)
	}
}

func TestReportPublicationRequiresExactMicroEventSentenceCitations(t *testing.T) {
	period, _ := PeriodFor(time.Now().UTC(), ReportDaily, time.UTC)
	valid := Report{ID: 7, Version: 1, VersionNo: 1, Type: ReportDaily, Period: period, Title: "daily", Status: ReportDraft,
		Items: []Item{{MicroEventID: 9, MicroEventVersion: 2, MicroEventUpdateID: 19, MicroEventSummaryID: 29,
			Rank: 1, Title: "event", HeatScore: 80, EvidenceSetHash: testEvidenceHash, ReasonCodes: []string{"rising"},
			Sentences: []Sentence{
				{SourceSummarySentenceID: 39, Ordinal: 0, Text: "A sourced fact.", DecisionOrigin: "manual", ActorUserID: int64Pointer(3), ClaimEvidenceVersionIDs: []int64{49}},
				{SourceSummarySentenceID: 40, Ordinal: 1, Text: "Editorial note.", EditorialNote: true, DecisionOrigin: "manual", ActorUserID: int64Pointer(3)},
			}}}}
	if err := valid.ValidatePublicationShape(); err != nil {
		t.Fatalf("valid publication shape: %v", err)
	}

	cases := map[string]Report{
		"legacy event aggregate": func() Report {
			changed := valid
			changed.Items = append([]Item(nil), valid.Items...)
			changed.Items[0].EventID, changed.Items[0].EventUpdateID, changed.Items[0].MicroEventID = 9, 19, 0
			return changed
		}(),
		"event state without sentence": func() Report {
			changed := valid
			changed.Items = append([]Item(nil), valid.Items...)
			changed.Items[0].Sentences = nil
			return changed
		}(),
		"fact without citation": func() Report {
			changed := valid
			changed.Items = clonePublicationItems(valid.Items)
			changed.Items[0].Sentences[0].ClaimEvidenceVersionIDs = nil
			return changed
		}(),
		"editorial citation": func() Report {
			changed := valid
			changed.Items = clonePublicationItems(valid.Items)
			changed.Items[0].Sentences[1].ClaimEvidenceVersionIDs = []int64{49}
			return changed
		}(),
		"duplicate citation": func() Report {
			changed := valid
			changed.Items = clonePublicationItems(valid.Items)
			changed.Items[0].Sentences[0].ClaimEvidenceVersionIDs = []int64{49, 49}
			return changed
		}(),
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if err := report.ValidatePublicationShape(); !errors.Is(err, ErrEvidenceInvalid) {
				t.Fatalf("ValidatePublicationShape() error = %v, want ErrEvidenceInvalid", err)
			}
		})
	}
}

func clonePublicationItems(items []Item) []Item {
	cloned := append([]Item(nil), items...)
	for index := range cloned {
		cloned[index].Sentences = append([]Sentence(nil), items[index].Sentences...)
	}
	return cloned
}

func int64Pointer(value int64) *int64 { return &value }

const testEvidenceHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
