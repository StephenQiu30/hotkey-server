package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type microEventListRepositoryStub struct{ query MicroEventListQuery }

func (stub *microEventListRepositoryStub) ListMicroEvents(_ context.Context, query MicroEventListQuery) (MicroEventPageDTO, error) {
	stub.query = query
	return MicroEventPageDTO{}, nil
}
func (*microEventListRepositoryStub) GetMicroEvent(context.Context, int64) (MicroEventProjectionDTO, error) {
	return MicroEventProjectionDTO{}, nil
}
func (*microEventListRepositoryStub) ListMicroEventEvidence(context.Context, MicroEventEvidenceQuery) (MicroEventEvidencePageDTO, error) {
	return MicroEventEvidencePageDTO{}, nil
}
func (*microEventListRepositoryStub) GetMicroEventSummary(context.Context, int64, int64) (*EvidenceSummaryDTO, error) {
	return nil, ErrEvidenceSummaryUnavailable
}

func TestMicroEventListValidatesAndCanonicalizesMultidimensionalQuery(t *testing.T) {
	repository := &microEventListRepositoryStub{}
	service, err := NewMicroEventQueryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 8, 1, 0, 0, 0, 123, time.FixedZone("offset", 8*60*60))
	to := from.Add(24 * time.Hour)
	_, err = service.List(context.Background(), MicroEventListQuery{
		MonitorID: 7, Statuses: []string{"closed", "active"}, SourceTypes: []string{"x", "rss"},
		EvidenceStates: []string{"multiple_origins", "single_origin"}, StartedFrom: &from, StartedTo: &to,
		Sort: "relevance", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.query.Sort != "relevance" || repository.query.MonitorID != 7 ||
		repository.query.StartedFrom == nil || repository.query.StartedFrom.Location() != time.UTC ||
		repository.query.StartedTo == nil || repository.query.StartedTo.Location() != time.UTC {
		t.Fatalf("canonical query = %#v", repository.query)
	}
}

func TestMicroEventListRejectsInvalidMultidimensionalQuery(t *testing.T) {
	service, err := NewMicroEventQueryService(&microEventListRepositoryStub{})
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	for _, query := range []MicroEventListQuery{
		{MonitorID: -1},
		{SourceTypes: []string{"unknown"}},
		{EvidenceStates: []string{"verified"}},
		{StartedFrom: &from, StartedTo: &to},
		{Sort: "importance"},
	} {
		if _, err := service.List(context.Background(), query); !errors.Is(err, ErrInvalidMicroEventQuery) {
			t.Fatalf("query %#v error = %v", query, err)
		}
	}
}
