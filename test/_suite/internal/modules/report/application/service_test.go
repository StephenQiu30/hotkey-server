package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type serviceStoreFake struct {
	reports map[int64]domain.Report
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

func TestServicePublishFreezesDraftAndRejectsRepeat(t *testing.T) {
	period, err := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	store := &serviceStoreFake{reports: map[int64]domain.Report{7: {ID: 7, Version: 1, VersionNo: 1, Type: domain.ReportDaily, Period: period, Title: "daily", Status: domain.ReportDraft, Items: []domain.Item{{EventID: 9, Rank: 1, Title: "event", HeatScore: 80}}}}}
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
	report, err := service.Build(context.Background(), BuildInput{ID: 8, Type: domain.ReportWeekly, At: time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC), Timezone: "Asia/Shanghai", Events: []EventSnapshot{{EventID: 2, Title: "event", Summary: "snapshot", HeatScore: 91}}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != domain.ReportDraft || report.Summary == "" || report.Period.Location.String() != "Asia/Shanghai" || report.Items[0].InclusionReason == "" {
		t.Fatalf("built report = %#v", report)
	}
}
