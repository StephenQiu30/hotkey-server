package application

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type metricRecomputeFake struct {
	commands []MetricRecomputeCommand
}

func (fake *metricRecomputeFake) RecomputeEventMetrics(_ context.Context, command MetricRecomputeCommand) ([]domain.HeatResult, error) {
	fake.commands = append(fake.commands, command)
	return nil, nil
}

func TestEventMutationsInvokeTheSameMetricRecomputeUseCase(t *testing.T) {
	recompute := &metricRecomputeFake{}
	store := &lifecycleStoreFake{event: lifecycleEvent()}
	if _, err := NewLifecycleService(store, recompute).Transition(context.Background(), LifecycleInput{EventID: 1, ExpectedVersion: 2, To: domain.LifecycleActive, ReasonCode: "two_sources"}); err != nil {
		t.Fatalf("lifecycle transition: %v", err)
	}
	governance := &governanceFake{}
	service := NewGovernanceService(governance, recompute)
	if _, err := service.Merge(context.Background(), MergeCommand{SourceEventID: 2, TargetEventID: 3, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: "confirmed"}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if _, err := service.Split(context.Background(), SplitCommand{SourceEventID: 4, SourceExpectedVersion: 1, Members: []SplitMember{{ContentID: 9, ExpectedVersion: 1}}, ReasonCode: "separate"}); err != nil {
		t.Fatalf("split: %v", err)
	}
	if _, err := service.SetMemberLock(context.Background(), MemberLockCommand{EventID: 5, ContentID: 10, ExpectedVersion: 1, ReasonCode: "reviewed"}); err != nil {
		t.Fatalf("member lock: %v", err)
	}
	evidenceStore := &evidenceStoreFake{event: lifecycleEvent()}
	if _, err := NewEvidenceService(evidenceStore, recompute).Recompute(context.Background(), EvidenceRecomputeCommand{EventID: 1, ReasonCode: "content_deleted"}); err != nil {
		t.Fatalf("evidence recompute: %v", err)
	}

	gotIDs := make([]int64, 0, len(recompute.commands))
	for _, command := range recompute.commands {
		if command.HeatVersion != domain.HeatAlgorithmVersionV1 || command.WindowEnd.IsZero() || command.WindowEnd.Location() != time.UTC {
			t.Fatalf("metric command = %#v", command)
		}
		gotIDs = append(gotIDs, command.EventID)
	}
	wantIDs := []int64{1, 3, 2, 4, 99, 5, 1}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("metric recompute ids = %#v, want %#v", gotIDs, wantIDs)
	}
	for index := range wantIDs {
		if gotIDs[index] != wantIDs[index] {
			t.Fatalf("metric recompute ids = %#v, want %#v", gotIDs, wantIDs)
		}
	}
}

func TestClusteringExecutionInvokesMetricRecomputeForChangedMembership(t *testing.T) {
	now := time.Now().UTC()
	event := domain.Event{ID: 7, Version: 1, EventKey: "evt_7", TitleZH: "事件", LifecycleStatus: domain.LifecycleDetected, FirstSeenAt: now, LastSeenAt: now}
	recompute := &metricRecomputeFake{}
	service := NewClusteringExecutionService(NewRecallService(recallReaderFake{}), NewClusteringService(), &clusteringWriterFake{result: ClusteringWriteResult{Event: &event, Created: true}}, recompute)
	if _, err := service.Execute(context.Background(), ClusteringExecutionInput{ContentID: 1, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("content", "1"), Scores: map[string]domain.ScoreBreakdown{}}); err != nil {
		t.Fatalf("clustering execution: %v", err)
	}
	if len(recompute.commands) != 1 || recompute.commands[0].EventID != event.ID {
		t.Fatalf("metric recompute commands = %#v", recompute.commands)
	}
}
