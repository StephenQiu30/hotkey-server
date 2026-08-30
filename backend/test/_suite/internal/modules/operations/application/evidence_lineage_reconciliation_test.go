package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

var evidenceLineageReconciliationFence = time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)

type evidenceLineageReconciliationRepositoryFake struct {
	inspection EvidenceLineageReconciliationInspectionDTO
	started    int
	completed  int
	batches    []EvidenceLineageReconciliationBatchResultDTO
	commands   []EvidenceLineageReconciliationBatchCommand
}

func (fake *evidenceLineageReconciliationRepositoryFake) InspectEvidenceLineageReconciliation(context.Context, EvidenceLineageReconciliationInspectionQuery) (EvidenceLineageReconciliationInspectionDTO, error) {
	return fake.inspection, nil
}
func (fake *evidenceLineageReconciliationRepositoryFake) StartEvidenceLineageReconciliation(context.Context, StartEvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationRunDTO, error) {
	fake.started++
	return EvidenceLineageReconciliationRunDTO{RunID: 7, Status: "running", FencedAt: evidenceLineageReconciliationFence}, nil
}
func (fake *evidenceLineageReconciliationRepositoryFake) ResumeEvidenceLineageReconciliation(context.Context, ResumeEvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationRunDTO, error) {
	return EvidenceLineageReconciliationRunDTO{}, errors.New("unexpected resume")
}
func (fake *evidenceLineageReconciliationRepositoryFake) ApplyEvidenceLineageReconciliationBatch(_ context.Context, command EvidenceLineageReconciliationBatchCommand) (EvidenceLineageReconciliationBatchResultDTO, error) {
	fake.commands = append(fake.commands, command)
	result := fake.batches[0]
	fake.batches = fake.batches[1:]
	return result, nil
}
func (fake *evidenceLineageReconciliationRepositoryFake) CompleteEvidenceLineageReconciliation(context.Context, CompleteEvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationRunDTO, error) {
	fake.completed++
	return EvidenceLineageReconciliationRunDTO{RunID: 7, Status: "completed", FencedAt: evidenceLineageReconciliationFence, ExaminedCount: 2, FindingCount: 1, HealthyCount: 1}, nil
}

func TestEvidenceLineageReconciliationDryRunNeverStartsRepair(t *testing.T) {
	repository := &evidenceLineageReconciliationRepositoryFake{inspection: EvidenceLineageReconciliationInspectionDTO{Scope: "all", CandidateCount: 2}}
	service, err := NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Reconcile(context.Background(), EvidenceLineageReconciliationCommand{Scope: "all", BatchSize: 100, GracePeriodHours: 24, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspection.CandidateCount != 2 || repository.started != 0 || len(repository.commands) != 0 {
		t.Fatalf("read-only result=%+v fake=%+v", result, repository)
	}
}

func TestEvidenceLineageReconciliationApplyUsesStableCursorAndCompletes(t *testing.T) {
	repository := &evidenceLineageReconciliationRepositoryFake{
		inspection: EvidenceLineageReconciliationInspectionDTO{Scope: "all", CandidateCount: 2},
		batches: []EvidenceLineageReconciliationBatchResultDTO{
			{RunID: 7, ExaminedCount: 1, HealthyCount: 1, LastAssetCursor: 1, HasMore: true},
			{RunID: 7, ExaminedCount: 1, FindingCount: 1, LastAssetCursor: 2},
		},
	}
	service, err := NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}
	command := validEvidenceLineageReconciliationCommand()
	result, err := service.Reconcile(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if repository.started != 1 || repository.completed != 1 || len(repository.commands) != 2 || result.Run.Status != "completed" {
		t.Fatalf("apply result=%+v fake=%+v", result, repository)
	}
	if repository.commands[0].AfterAssetCursor != 0 || repository.commands[1].AfterAssetCursor != 1 {
		t.Fatalf("cursors=%d,%d", repository.commands[0].AfterAssetCursor, repository.commands[1].AfterAssetCursor)
	}
	if !repository.commands[0].FencedAt.Equal(evidenceLineageReconciliationFence) ||
		!repository.commands[1].FencedAt.Equal(evidenceLineageReconciliationFence) {
		t.Fatalf("time fences=%s,%s", repository.commands[0].FencedAt, repository.commands[1].FencedAt)
	}
}

func validEvidenceLineageReconciliationCommand() EvidenceLineageReconciliationCommand {
	return EvidenceLineageReconciliationCommand{
		Scope: "all", BatchSize: 1, GracePeriodHours: 24, Apply: true, ConfirmNonEmpty: true,
		OperatorID: "operator-a", ReviewerID: "reviewer-b",
		BinarySHA256: repeatMaintenanceCharacter("1", 64), SchemaSHA256: repeatMaintenanceCharacter("2", 64),
		ConfigurationSHA256: repeatMaintenanceCharacter("3", 64), BackupEvidenceSHA256: repeatMaintenanceCharacter("4", 64),
		RehearsalEvidenceSHA256: repeatMaintenanceCharacter("5", 64),
	}
}
