package domain

import (
	"strings"
	"testing"
	"time"
)

func TestQueryNormalizesTypesAndValidatesLexicalFilters(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	to := from.Add(24 * time.Hour)
	sourceID, monitorID := int64(7), int64(11)
	query := Query{
		Keyword: "  芯片 release  ", Types: []ResourceType{ResourceKnowledge, ResourceContent, ResourceKnowledge, ResourceEvent},
		SourceConnectionID: &sourceID, MonitorID: &monitorID, Entity: "Acme-7", Status: "review_pending",
		From: &from, To: &to, Sort: SortLatest, Limit: 25,
	}.Normalized()
	if err := query.Validate(); err != nil {
		t.Fatal(err)
	}
	if query.Keyword != "芯片 release" || query.Sort != SortLatest || query.Limit != 25 || len(query.Types) != 3 || query.Types[0] != ResourceContent || query.Types[1] != ResourceEvent || query.Types[2] != ResourceKnowledge {
		t.Fatalf("normalized query = %#v", query)
	}
	if query.From.Location() != time.UTC || query.To.Location() != time.UTC {
		t.Fatalf("query times are not UTC: %#v", query)
	}
}

func TestQueryDefaultsToEveryP0ResourceAndRejectsUnboundedInput(t *testing.T) {
	query := Query{Keyword: "release"}.Normalized()
	if err := query.Validate(); err != nil || query.Sort != SortRelevance || query.Limit != DefaultLimit || len(query.Types) != 3 {
		t.Fatalf("default query = %#v/%v", query, err)
	}

	tooLong := strings.Repeat("搜", MaximumKeywordRunes+1)
	invalidReference := int64(0)
	from := time.Now().UTC()
	before := from.Add(-time.Minute)
	for name, invalid := range map[string]Query{
		"empty":          {},
		"keyword":        {Keyword: tooLong},
		"type":           {Keyword: "x", Types: []ResourceType{"embedding"}},
		"limit":          {Keyword: "x", Limit: MaximumLimit + 1},
		"source":         {Keyword: "x", SourceConnectionID: &invalidReference},
		"monitor":        {Keyword: "x", MonitorID: &invalidReference},
		"entity":         {Keyword: "x", Entity: strings.Repeat("e", MaximumEntityRunes+1)},
		"status":         {Keyword: "x", Status: "active OR true"},
		"sort":           {Keyword: "x", Sort: "semantic"},
		"reversed range": {Keyword: "x", From: &from, To: &before},
	} {
		t.Run(name, func(t *testing.T) {
			if err := invalid.Normalized().Validate(); err == nil {
				t.Fatalf("invalid query accepted: %#v", invalid)
			}
		})
	}
}

func TestCandidateRequiresBoundedOwnedDisplayProjection(t *testing.T) {
	now := time.Now().UTC()
	candidate := Candidate{Type: ResourceContent, ID: 1, Title: "Release", Snippet: "Body", Status: "active", OccurredAt: now, Score: 0.5}
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, invalid := range map[string]Candidate{
		"type":    {Type: "embedding", ID: 1, Title: "x", OccurredAt: now},
		"id":      {Type: ResourceContent, Title: "x", OccurredAt: now},
		"title":   {Type: ResourceContent, ID: 1, OccurredAt: now},
		"time":    {Type: ResourceContent, ID: 1, Title: "x"},
		"score":   {Type: ResourceContent, ID: 1, Title: "x", OccurredAt: now, Score: -1},
		"snippet": {Type: ResourceContent, ID: 1, Title: "x", Snippet: strings.Repeat("x", MaximumSnippetBytes+1), OccurredAt: now},
	} {
		t.Run(name, func(t *testing.T) {
			if err := invalid.Validate(); err == nil {
				t.Fatalf("invalid candidate accepted: %#v", invalid)
			}
		})
	}
}
