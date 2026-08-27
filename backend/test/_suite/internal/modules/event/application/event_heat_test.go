package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
)

func TestEventHeatServiceUsesActiveProfileWeightsAndStableSnapshot(t *testing.T) {
	endedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	engagement := .8
	repository := &eventHeatRepositoryFake{target: eventapplication.EventHeatTargetDTO{
		MicroEventID: 7, MicroEventVersion: 3, HeatProfileID: 5, HeatProfileVersion: "heat-v2",
		Weights: eventapplication.EventHeatWeightsDTO{Lineage: .25, Velocity: .20, Acceleration: .15,
			Coverage: .15, Engagement: .15, Recency: .10},
		WindowStartedAt: endedAt.Add(-24 * time.Hour), WindowEndedAt: endedAt,
		IndependentLineageRoots: 4, ReportsInWindow: 8, ReportsInPreviousWindow: 4,
		ReportsInPriorWindow: 2, PublisherCoverage: 3, SourceTypeCoverage: 2,
		NormalizedEngagement: &engagement, NormalizationFallback: true, AgeHours: 2,
	}}
	service, err := eventapplication.NewEventHeatService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Calculate(context.Background(), eventapplication.CalculateEventHeatCommand{
		MicroEventID: 7, WindowHours: 24, WindowEndedAt: endedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.HeatScore <= 0 || repository.committed.IndependentLineageRoots != 4 ||
		repository.committed.AvailableWeight != 1 || len(repository.committed.ReasonCodes) != 1 || repository.committed.ReasonCodes[0] != "normalization_fallback" {
		t.Fatalf("snapshot/command = %#v / %#v", result, repository.committed)
	}
}

func TestEventHeatServiceRejectsRepositoryRewriteOfDeterministicScore(t *testing.T) {
	endedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	repository := &eventHeatRepositoryFake{target: eventapplication.EventHeatTargetDTO{
		MicroEventID: 7, MicroEventVersion: 3, HeatProfileID: 5, HeatProfileVersion: "heat-v2",
		Weights: eventapplication.EventHeatWeightsDTO{Lineage: .25, Velocity: .20, Acceleration: .15,
			Coverage: .15, Engagement: .15, Recency: .10},
		WindowStartedAt: endedAt.Add(-24 * time.Hour), WindowEndedAt: endedAt,
		IndependentLineageRoots: 2, ReportsInWindow: 3, ReportsInPreviousWindow: 1,
		PublisherCoverage: 1, SourceTypeCoverage: 1, AgeHours: 5,
	}, mutate: func(snapshot *eventapplication.EventHeatSnapshotDTO) {
		snapshot.HeatScore = 99
		snapshot.ReasonCodes = []string{"model_override"}
	}}
	service, _ := eventapplication.NewEventHeatService(repository)
	_, err := service.Calculate(context.Background(), eventapplication.CalculateEventHeatCommand{
		MicroEventID: 7, WindowHours: 24, WindowEndedAt: endedAt,
	})
	if !errors.Is(err, eventapplication.ErrInvalidEventHeatContract) {
		t.Fatalf("Calculate() error = %v, want invalid contract", err)
	}
}

func TestEventHeatServiceRenormalizesWhenEngagementIsUnavailable(t *testing.T) {
	endedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	repository := &eventHeatRepositoryFake{target: eventapplication.EventHeatTargetDTO{
		MicroEventID: 7, MicroEventVersion: 3, HeatProfileID: 5, HeatProfileVersion: "heat-v2",
		Weights: eventapplication.EventHeatWeightsDTO{Lineage: .25, Velocity: .20, Acceleration: .15,
			Coverage: .15, Engagement: .15, Recency: .10},
		WindowStartedAt: endedAt.Add(-24 * time.Hour), WindowEndedAt: endedAt,
		IndependentLineageRoots: 2, ReportsInWindow: 3, ReportsInPreviousWindow: 1,
		PublisherCoverage: 1, SourceTypeCoverage: 1, AgeHours: 5,
	}}
	service, _ := eventapplication.NewEventHeatService(repository)
	result, err := service.Calculate(context.Background(), eventapplication.CalculateEventHeatCommand{
		MicroEventID: 7, WindowHours: 24, WindowEndedAt: endedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.AvailableWeight != .85 || result.Snapshot.NormalizedEngagement != nil ||
		len(result.Snapshot.ReasonCodes) != 1 || result.Snapshot.ReasonCodes[0] != "metrics_unavailable" {
		t.Fatalf("missing engagement snapshot = %#v", result.Snapshot)
	}
}

type eventHeatRepositoryFake struct {
	target    eventapplication.EventHeatTargetDTO
	committed eventapplication.CommitEventHeatSnapshotCommand
	mutate    func(*eventapplication.EventHeatSnapshotDTO)
}

func (repository *eventHeatRepositoryFake) ReadEventHeatTarget(_ context.Context, _ eventapplication.ReadEventHeatTargetQuery) (eventapplication.EventHeatTargetDTO, error) {
	return repository.target, nil
}

func (repository *eventHeatRepositoryFake) CommitEventHeatSnapshot(_ context.Context, command eventapplication.CommitEventHeatSnapshotCommand) (eventapplication.EventHeatSnapshotDTO, error) {
	repository.committed = command
	snapshot := eventapplication.EventHeatSnapshotDTO{ID: 9, MicroEventID: command.MicroEventID,
		MicroEventVersion: command.MicroEventVersion, HeatProfileID: command.HeatProfileID,
		WindowStartedAt: command.WindowStartedAt, WindowEndedAt: command.WindowEndedAt,
		IndependentLineageRoots: command.IndependentLineageRoots, Velocity: command.Velocity,
		Acceleration: command.Acceleration, Coverage: command.Coverage,
		NormalizedEngagement: command.NormalizedEngagement, Recency: command.Recency,
		AvailableWeight: command.AvailableWeight, HeatScore: command.HeatScore,
		ReasonCodes: command.ReasonCodes, Created: true}
	if repository.mutate != nil {
		repository.mutate(&snapshot)
	}
	return snapshot, nil
}
