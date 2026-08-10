package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDocumentMatchQueryServiceReturnsExactEffectiveDecisionPage(t *testing.T) {
	reader := &documentMatchReaderFake{result: DocumentMatchPageResult{
		Items: []DocumentMatchListItemDTO{{
			Automatic: DocumentMatchDecisionDTO{
				ID: 31, MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13,
				DocumentVersionID: 17, RelevanceProfileID: 19, MatchingAlgorithmVersion: "rrf-k60-v1",
				RerankerVersion: "cross-encoder-v1", CalibrationVersion: "uncalibrated-v1",
				InputHash: repeatedDocumentMatchHash('a'), Decision: "review", Degraded: true,
				CreatedAt: time.Now().UTC(),
			},
			EffectiveDecision: "accepted", OverrideSequence: 1,
		}},
		NextCursor: "opaque-next",
	}}
	service, err := NewDocumentMatchQueryService(reader)
	if err != nil {
		t.Fatalf("NewDocumentMatchQueryService(): %v", err)
	}
	page, err := service.List(context.Background(), ListDocumentMatchesQuery{
		ActorUserID: 5, MonitorID: 7, EffectiveDecision: "accepted", Limit: 25,
	})
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if reader.query.ActorUserID != 5 || reader.query.MonitorID != 7 || reader.query.Limit != 25 ||
		len(page.Items) != 1 || page.Items[0].Automatic.DocumentVersionID != 17 || page.NextCursor != "opaque-next" {
		t.Fatalf("query/page = %#v / %#v", reader.query, page)
	}
}

func TestDocumentMatchQueryServiceRejectsInvalidFilterAndReceiptDrift(t *testing.T) {
	reader := &documentMatchReaderFake{}
	service, err := NewDocumentMatchQueryService(reader)
	if err != nil {
		t.Fatalf("NewDocumentMatchQueryService(): %v", err)
	}
	if _, err := service.List(context.Background(), ListDocumentMatchesQuery{
		ActorUserID: 5, MonitorID: 7, EffectiveDecision: "trusted",
	}); !errors.Is(err, ErrInvalidDocumentMatchContract) || reader.calls != 0 {
		t.Fatalf("invalid filter error=%v calls=%d", err, reader.calls)
	}
	reader.result = DocumentMatchPageResult{Items: []DocumentMatchListItemDTO{{
		Automatic: DocumentMatchDecisionDTO{ID: 31, MonitorID: 99}, EffectiveDecision: "review",
	}}}
	if _, err := service.List(context.Background(), ListDocumentMatchesQuery{ActorUserID: 5, MonitorID: 7}); !errors.Is(err, ErrInvalidDocumentMatchContract) {
		t.Fatalf("receipt drift error = %v", err)
	}
}

type documentMatchReaderFake struct {
	query  ListDocumentMatchesQuery
	result DocumentMatchPageResult
	err    error
	calls  int
}

func (fake *documentMatchReaderFake) ListDocumentMatches(_ context.Context, query ListDocumentMatchesQuery) (DocumentMatchPageResult, error) {
	fake.calls++
	fake.query = query
	return fake.result, fake.err
}

func repeatedDocumentMatchHash(value byte) string {
	result := make([]byte, 64)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
