package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type serviceStoreFake struct {
	reports                map[int64]domain.Report
	publicationError       error
	publicationValidations int
}

func (fake *serviceStoreFake) Save(_ context.Context, report domain.Report) error {
	fake.reports[report.ID] = report
	return nil
}

func (fake *serviceStoreFake) Get(_ context.Context, reportID int64) (domain.Report, error) {
	report, ok := fake.reports[reportID]
	if !ok {
		return domain.Report{}, sharedrepository.ErrNotFound
	}
	return report, nil
}

func (fake *serviceStoreFake) List(_ context.Context, _ domain.ListQuery) (domain.Page, error) {
	return domain.Page{}, nil
}

func (fake *serviceStoreFake) ValidatePublication(_ context.Context, _ domain.Report) error {
	fake.publicationValidations++
	return fake.publicationError
}

func (fake *serviceStoreFake) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	before := make(map[int64]domain.Report, len(fake.reports))
	for id, report := range fake.reports {
		before[id] = report
	}
	if err := fn(ctx); err != nil {
		fake.reports = before
		return err
	}
	return nil
}

type failingArchive struct{ err error }

func (archive failingArchive) Prepare(context.Context, domain.Report) error { return archive.err }

func TestServicePublishFreezesDraftAndRejectsRepeat(t *testing.T) {
	period, err := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	draft := citedReportDraft(period, 7)
	store := &serviceStoreFake{reports: map[int64]domain.Report{7: draft}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.Publish(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != domain.ReportPublished || !published.Frozen || published.Version != 2 {
		t.Fatalf("published report = %#v", published)
	}
	if store.publicationValidations != 1 {
		t.Fatalf("publication validations = %d, want 1", store.publicationValidations)
	}
	if _, err := service.Publish(context.Background(), 7); !errors.Is(err, sharedrepository.ErrImmutable) {
		t.Fatalf("repeat publish error = %v, want ErrImmutable", err)
	}
}

func TestServiceBuildUsesTimezoneAndDeterministicFallback(t *testing.T) {
	store := &serviceStoreFake{reports: make(map[int64]domain.Report)}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Build(context.Background(), BuildInput{ID: 8, Type: domain.ReportWeekly, At: time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC), Timezone: "Asia/Shanghai", Events: []EventSnapshot{{EventID: 2, EventUpdateID: 12, Title: "event", Summary: "snapshot", HeatScore: 91, EvidenceSetHash: testHash, ReasonCodes: []string{"rising"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != domain.ReportDraft || report.Title != "周报" || report.Summary == "" || report.Period.Location.String() != "Asia/Shanghai" || report.Items[0].InclusionReason == "" {
		t.Fatalf("built report = %#v", report)
	}
}

func TestServicePublishRollsBackWhenArchiveProposalFails(t *testing.T) {
	period, _ := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	draft := citedReportDraft(period, 11)
	store := &serviceStoreFake{reports: map[int64]domain.Report{draft.ID: draft}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	service.SetArchivePlanner(failingArchive{err: errors.New("archive unavailable")})
	if _, err := service.Publish(context.Background(), draft.ID); err == nil {
		t.Fatal("Publish() error = nil")
	}
	if stored := store.reports[draft.ID]; stored.Status != domain.ReportDraft || stored.Version != 1 {
		t.Fatalf("report escaped failed transaction: %#v", stored)
	}
}

func TestServicePublishRejectsInvalidEvidenceBeforeAnyWrite(t *testing.T) {
	period, _ := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	draft := citedReportDraft(period, 13)
	store := &serviceStoreFake{reports: map[int64]domain.Report{draft.ID: draft}, publicationError: domain.ErrEvidenceInvalid}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishAs(context.Background(), draft.ID, 7); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("PublishAs() error = %v, want ErrEvidenceInvalid", err)
	}
	if stored := store.reports[draft.ID]; stored.Status != domain.ReportDraft || stored.Version != 1 || stored.UpdatedBy != nil {
		t.Fatalf("invalid evidence changed report: %#v", stored)
	}
}

func citedReportDraft(period domain.Period, id int64) domain.Report {
	actorID := int64(3)
	return domain.Report{ID: id, Version: 1, VersionNo: 1, Type: domain.ReportDaily, Period: period, Title: "daily", Status: domain.ReportDraft,
		Items: []domain.Item{{MicroEventID: 9, MicroEventVersion: 2, MicroEventUpdateID: 19, MicroEventSummaryID: 29,
			Rank: 1, Title: "event", HeatScore: 80, EvidenceSetHash: testHash, ReasonCodes: []string{"rising"},
			Sentences: []domain.Sentence{{SourceSummarySentenceID: 39, Ordinal: 0, Text: "Sourced fact.",
				DecisionOrigin: "manual", ActorUserID: &actorID, ClaimEvidenceVersionIDs: []int64{49}}}}}}
}

const testHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
