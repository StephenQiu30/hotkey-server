package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestEventUpdateServiceRecordsFromStrictlyEarlier24HourSnapshot(t *testing.T) {
	current := applicationEventUpdateHeat(time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC))
	previous := current
	previous.WindowEnd = current.WindowEnd.Add(-time.Hour)
	previous.HeatScore = 40
	previous.TrendStatus = domain.TrendStable
	repository := &eventUpdateRepositoryFake{
		previous:      &previous,
		appendResult:  &domain.EventUpdate{ID: 91, Version: 1, EventID: current.EventID, SequenceNo: 3, Kind: domain.EventUpdateRising},
		appendCreated: true,
	}
	update, created, err := NewUpdateService(repository).Record(context.Background(), current)
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if !created || update == nil || update.ID != 91 {
		t.Fatalf("Record() = %#v/%t, want created update 91", update, created)
	}
	if repository.previousEventID != current.EventID || repository.previousBefore != current.WindowEnd || repository.previousWindowHours != 24 {
		t.Fatalf("previous snapshot query = event %d/window %d/before %v", repository.previousEventID, repository.previousWindowHours, repository.previousBefore)
	}
	if repository.appended == nil || repository.appended.Kind != domain.EventUpdateRising || repository.appended.EventID != current.EventID {
		t.Fatalf("AppendUpdate() candidate = %#v", repository.appended)
	}
}

func TestEventUpdateServiceSkipsAppendForNonMaterialRecomputation(t *testing.T) {
	current := applicationEventUpdateHeat(time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC))
	previous := current
	previous.WindowEnd = current.WindowEnd.Add(-time.Hour)
	previous.HeatScore = current.HeatScore - 5
	repository := &eventUpdateRepositoryFake{previous: &previous}
	update, created, err := NewUpdateService(repository).Record(context.Background(), current)
	if err != nil || update != nil || created {
		t.Fatalf("Record() = %#v/%t/%v, want no update", update, created, err)
	}
	if repository.appendCalls != 0 {
		t.Fatalf("AppendUpdate() calls = %d, want 0", repository.appendCalls)
	}
}

func TestEventUpdateServiceReturnsExistingUpdateOnIdempotentRecord(t *testing.T) {
	current := applicationEventUpdateHeat(time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC))
	existing := &domain.EventUpdate{ID: 77, Version: 1, EventID: current.EventID, SequenceNo: 1, Kind: domain.EventUpdateEventCreated}
	repository := &eventUpdateRepositoryFake{appendResult: existing, appendCreated: false}
	got, created, err := NewUpdateService(repository).Record(context.Background(), current)
	if err != nil || created || got != existing {
		t.Fatalf("Record() = %#v/%t/%v, want reused update", got, created, err)
	}
}

func TestEventUpdateServiceListsSequenceDescendingPage(t *testing.T) {
	repository := &eventUpdateRepositoryFake{page: domain.EventUpdatePage{
		Items:      []domain.EventUpdate{{ID: 5, EventID: 42, SequenceNo: 5}, {ID: 4, EventID: 42, SequenceNo: 4}},
		NextCursor: 4,
	}}
	page, err := NewUpdateService(repository).List(context.Background(), 42, 2, 6)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].SequenceNo != 5 || page.Items[1].SequenceNo != 4 || page.NextCursor != 4 {
		t.Fatalf("List() = %#v, want sequence-descending page", page)
	}
	if repository.listQuery.EventID != 42 || repository.listQuery.Limit != 2 || repository.listQuery.Cursor != 6 {
		t.Fatalf("ListUpdates() query = %#v", repository.listQuery)
	}
	for _, input := range []struct{ eventID, limit, cursor int64 }{{0, 2, 0}, {42, 0, 0}, {42, 101, 0}, {42, 2, -1}} {
		if _, err := NewUpdateService(repository).List(context.Background(), input.eventID, int(input.limit), input.cursor); err == nil {
			t.Fatalf("List(%d,%d,%d) error = nil", input.eventID, input.limit, input.cursor)
		}
	}
}

func TestEventUpdateServicePropagatesSnapshotFailureWithoutAppend(t *testing.T) {
	repository := &eventUpdateRepositoryFake{previousErr: errors.New("snapshot unavailable")}
	if _, _, err := NewUpdateService(repository).Record(context.Background(), applicationEventUpdateHeat(time.Now().UTC())); err == nil {
		t.Fatal("Record() error = nil")
	}
	if repository.appendCalls != 0 {
		t.Fatalf("AppendUpdate() calls = %d, want 0", repository.appendCalls)
	}
}

type eventUpdateRepositoryFake struct {
	previous            *domain.HeatResult
	previousErr         error
	previousEventID     int64
	previousWindowHours int
	previousBefore      time.Time
	appendResult        *domain.EventUpdate
	appendCreated       bool
	appendErr           error
	appended            *domain.EventUpdateCandidate
	appendCalls         int
	page                domain.EventUpdatePage
	listQuery           domain.EventUpdateListQuery
	listErr             error
}

func (fake *eventUpdateRepositoryFake) PreviousHeatSnapshot(_ context.Context, eventID int64, windowHours int, before time.Time) (*domain.HeatResult, error) {
	fake.previousEventID, fake.previousWindowHours, fake.previousBefore = eventID, windowHours, before
	return fake.previous, fake.previousErr
}

func (fake *eventUpdateRepositoryFake) AppendUpdate(_ context.Context, candidate domain.EventUpdateCandidate) (*domain.EventUpdate, bool, error) {
	fake.appendCalls++
	fake.appended = &candidate
	return fake.appendResult, fake.appendCreated, fake.appendErr
}

func (fake *eventUpdateRepositoryFake) ListUpdates(_ context.Context, query domain.EventUpdateListQuery) (domain.EventUpdatePage, error) {
	fake.listQuery = query
	return fake.page, fake.listErr
}

func applicationEventUpdateHeat(observedAt time.Time) domain.HeatResult {
	return domain.HeatResult{
		EventID: 42, HeatScore: 50, TrendScore: 10, TrendStatus: domain.TrendRising,
		SourceCount: 2, ContentCount: 4, HeatVersion: domain.HeatAlgorithmVersionV1,
		EvidenceSetHash: strings.Repeat("a", 64), CapabilityProfileSetHash: strings.Repeat("b", 64),
		WindowHours: 24, WindowEnd: observedAt,
	}
}

type radarRepositoryFake struct {
	page    domain.RadarPage
	err     error
	queries []domain.RadarQuery
}

func (fake *radarRepositoryFake) ListRadar(_ context.Context, query domain.RadarQuery) (domain.RadarPage, error) {
	fake.queries = append(fake.queries, query)
	return fake.page, fake.err
}

func TestRadarServiceDefaultsAndFreezesFirstPageAsOf(t *testing.T) {
	repository := &radarRepositoryFake{page: domain.RadarPage{Items: []domain.RadarEvent{{EventID: 7}}}}
	before := time.Now().UTC()
	page, err := NewRadarService(repository).List(context.Background(), domain.RadarQuery{})
	after := time.Now().UTC()
	if err != nil || len(page.Items) != 1 || len(repository.queries) != 1 {
		t.Fatalf("List() = %#v/%v queries=%#v", page, err, repository.queries)
	}
	query := repository.queries[0]
	if query.Window != domain.RadarWindow24Hours || query.Sort != domain.RadarSortMomentum || query.Limit != 50 || query.AsOf.Location() != time.UTC || query.AsOf.Before(before) || query.AsOf.After(after) {
		t.Fatalf("normalized query = %#v", query)
	}
}

func TestRadarServiceRejectsInvalidPublicQueryBeforeRepository(t *testing.T) {
	valid := domain.RadarQuery{Window: domain.RadarWindow24Hours, Sort: domain.RadarSortMomentum, Limit: 25}
	for _, mutate := range []func(*domain.RadarQuery){
		func(q *domain.RadarQuery) { q.Window = "2h" },
		func(q *domain.RadarQuery) { id := int64(0); q.MonitorID = &id },
		func(q *domain.RadarQuery) { q.Lifecycles = []domain.LifecycleStatus{"invented"} },
		func(q *domain.RadarQuery) { q.Trends = []domain.TrendStatus{"invented"} },
		func(q *domain.RadarQuery) { q.Verifications = []domain.RadarConfirmation{"invented"} },
		func(q *domain.RadarQuery) { value := -1.0; q.MinHeat = &value },
		func(q *domain.RadarQuery) { q.Sort = "invented" },
		func(q *domain.RadarQuery) { q.Limit = 101 },
		func(q *domain.RadarQuery) { q.Sort = domain.RadarSortRelevance },
	} {
		repository := &radarRepositoryFake{}
		query := valid
		mutate(&query)
		if _, err := NewRadarService(repository).List(context.Background(), query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("List(%#v) error = %v, want ErrInvalidInput", query, err)
		}
		if len(repository.queries) != 0 {
			t.Fatalf("invalid query reached repository: %#v", repository.queries)
		}
	}
	if _, err := NewRadarService(nil).List(context.Background(), valid); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("nil repository error = %v, want ErrUnavailable", err)
	}
}

func TestRadarServicePassesMonitorCursorAndRepositoryError(t *testing.T) {
	monitorID := int64(9)
	repository := &radarRepositoryFake{err: sharedrepository.ErrUnavailable}
	query := domain.RadarQuery{Window: domain.RadarWindow6Hours, MonitorID: &monitorID, Sort: domain.RadarSortRelevance, Limit: 10, Cursor: "opaque"}
	if _, err := NewRadarService(repository).List(context.Background(), query); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("List() error = %v", err)
	}
	if len(repository.queries) != 1 || repository.queries[0].MonitorID == nil || *repository.queries[0].MonitorID != monitorID || repository.queries[0].Cursor != "opaque" {
		t.Fatalf("repository query = %#v", repository.queries)
	}
}
