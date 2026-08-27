package application

import (
	"context"
	"testing"
	"time"
)

type productEventRefreshRepositoryFake struct {
	target   ProductEventRefreshTargetDTO
	commands []CommitProductEventUpdateCommand
	updateID int64
}

func (fake *productEventRefreshRepositoryFake) ReadProductEventRefreshTarget(context.Context, ProductEventRefreshTargetQuery) (ProductEventRefreshTargetDTO, error) {
	return fake.target, nil
}

func (fake *productEventRefreshRepositoryFake) CommitProductEventUpdate(_ context.Context, command CommitProductEventUpdateCommand) (ProductEventUpdateDTO, error) {
	fake.commands = append(fake.commands, command)
	return ProductEventUpdateDTO{ID: fake.updateID, Version: 1, MicroEventID: command.MicroEventID,
		MicroEventVersion: command.MicroEventVersion, WindowEndedAt: command.WindowEndedAt,
		WindowProfile: command.WindowProfile, HeatProfileID: command.HeatProfileID,
		HeatProfileVersion: command.HeatProfileVersion, EvidenceStateProfileID: command.EvidenceStateProfileID,
		EvidenceStateAlgorithmVersion: command.EvidenceStateAlgorithmVersion,
		HeatSnapshot1HourID:           command.HeatSnapshot1HourID, HeatSnapshot6HourID: command.HeatSnapshot6HourID,
		HeatSnapshot24HourID: command.HeatSnapshot24HourID, EvidenceStateSnapshotID: command.EvidenceStateSnapshotID,
		HeatScore: command.HeatScore, EvidenceState: command.EvidenceState,
		IndependentOriginCount: command.IndependentOriginCount, ReasonCodes: command.ReasonCodes,
		RefreshKey: command.RefreshKey, CreatedAt: command.WindowEndedAt, Created: len(fake.commands) == 1}, nil
}

type productEventHeatCalculatorFake struct{ commands []CalculateEventHeatCommand }

func (fake *productEventHeatCalculatorFake) Calculate(_ context.Context, command CalculateEventHeatCommand) (CalculateEventHeatResult, error) {
	fake.commands = append(fake.commands, command)
	return CalculateEventHeatResult{Snapshot: EventHeatSnapshotDTO{ID: int64(100 + command.WindowHours),
		MicroEventID: command.MicroEventID, MicroEventVersion: 3, HeatProfileID: 8,
		HeatProfileVersion: "heat-v2", WindowEndedAt: command.WindowEndedAt,
		HeatScore: 77.5, WarmingUp: command.WindowHours == 24}}, nil
}

type productEventEvidenceCalculatorFake struct {
	commands []CalculateEvidenceStateCommand
}

func (fake *productEventEvidenceCalculatorFake) CalculateState(_ context.Context, command CalculateEvidenceStateCommand) (CalculateEvidenceStateResult, error) {
	fake.commands = append(fake.commands, command)
	return CalculateEvidenceStateResult{Snapshot: EvidenceStateSnapshotDTO{ID: 201, Version: 1,
		MicroEventID: command.MicroEventID, EventVersion: command.ExpectedEventVersion, ProfileID: 9,
		AlgorithmVersion: command.AlgorithmVersion, State: "multiple_origins", IndependentOriginCount: 2}}, nil
}

type productEventAlertEvaluatorFake struct {
	updates []ProductEventUpdateDTO
}

func (fake *productEventAlertEvaluatorFake) EvaluateProductEventUpdate(_ context.Context, update ProductEventUpdateDTO) (ProductEventAlertEvaluationResult, error) {
	fake.updates = append(fake.updates, update)
	if len(fake.updates) == 1 {
		return ProductEventAlertEvaluationResult{CandidateCount: 1, EligibleCount: 1, NotificationCount: 1}, nil
	}
	return ProductEventAlertEvaluationResult{CandidateCount: 1, EligibleCount: 1, DuplicateCount: 1}, nil
}

func TestProductEventRefreshReplaysOneVersionedUpdateAndAlertEvaluation(t *testing.T) {
	windowEnd := time.Date(2026, time.August, 27, 12, 34, 50, 0, time.UTC)
	repository := &productEventRefreshRepositoryFake{target: ProductEventRefreshTargetDTO{MicroEventID: 7,
		MicroEventVersion: 3, HeatProfileID: 8, HeatProfileVersion: "heat-v2", EvidenceStateProfileID: 9,
		EvidenceStateAlgorithmVersion: CanonicalEvidenceStateAlgorithmVersion}, updateID: 301}
	heat, evidence, alerts := &productEventHeatCalculatorFake{}, &productEventEvidenceCalculatorFake{}, &productEventAlertEvaluatorFake{}
	service, err := NewProductEventRefreshService(repository, heat, evidence, alerts)
	if err != nil {
		t.Fatal(err)
	}
	command := RefreshProductEventCommand{MicroEventID: 7, ExpectedEventVersion: 3,
		WindowEndedAt: windowEnd, WindowProfile: ProductEventRefreshWindowProfile,
		HeatProfileVersion: "heat-v2", EvidenceStateAlgorithmVersion: CanonicalEvidenceStateAlgorithmVersion}
	first, err := service.Refresh(context.Background(), command)
	if err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}
	second, err := service.Refresh(context.Background(), command)
	if err != nil {
		t.Fatalf("replayed Refresh() error = %v", err)
	}
	if first.Update.ID != second.Update.ID || !first.Update.Created || second.Update.Created ||
		first.Update.RefreshKey == "" || first.Update.RefreshKey != second.Update.RefreshKey {
		t.Fatalf("refresh replay = first %#v, second %#v", first.Update, second.Update)
	}
	if len(heat.commands) != 6 || heat.commands[0].WindowHours != 1 || heat.commands[1].WindowHours != 6 ||
		heat.commands[2].WindowHours != 24 || !heat.commands[0].WindowEndedAt.Equal(windowEnd.Truncate(time.Minute)) {
		t.Fatalf("heat commands = %#v", heat.commands)
	}
	if len(evidence.commands) != 2 || evidence.commands[0].ExpectedEventVersion != 3 ||
		evidence.commands[0].AlgorithmVersion != CanonicalEvidenceStateAlgorithmVersion {
		t.Fatalf("evidence commands = %#v", evidence.commands)
	}
	if first.AlertEvaluation.NotificationCount != 1 || second.AlertEvaluation.DuplicateCount != 1 || len(alerts.updates) != 2 {
		t.Fatalf("alert evaluations = first %#v, second %#v", first.AlertEvaluation, second.AlertEvaluation)
	}
}
