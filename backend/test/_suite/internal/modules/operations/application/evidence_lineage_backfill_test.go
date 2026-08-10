package application

import (
	"context"
	"errors"
	"testing"
)

type evidenceLineageBackfillRepositoryFake struct {
	inspection EvidenceLineageBackfillInspectionDTO
	started    int
	resumed    int
	completed  int
	batches    []EvidenceLineageBackfillBatchResultDTO
	commands   []EvidenceLineageBackfillBatchCommand
}

func (fake *evidenceLineageBackfillRepositoryFake) InspectEvidenceLineageBackfill(context.Context, EvidenceLineageBackfillInspectionQuery) (EvidenceLineageBackfillInspectionDTO, error) {
	return fake.inspection, nil
}

func (fake *evidenceLineageBackfillRepositoryFake) StartEvidenceLineageBackfill(context.Context, StartEvidenceLineageBackfillCommand) (EvidenceLineageMaintenanceRunDTO, error) {
	fake.started++
	return EvidenceLineageMaintenanceRunDTO{RunID: 41, Status: "running"}, nil
}

func (fake *evidenceLineageBackfillRepositoryFake) ResumeEvidenceLineageBackfill(context.Context, ResumeEvidenceLineageBackfillCommand) (EvidenceLineageMaintenanceRunDTO, error) {
	fake.resumed++
	return EvidenceLineageMaintenanceRunDTO{RunID: 51, Status: "running", LastResourceID: 8}, nil
}

func (fake *evidenceLineageBackfillRepositoryFake) ApplyEvidenceLineageBackfillBatch(_ context.Context, command EvidenceLineageBackfillBatchCommand) (EvidenceLineageBackfillBatchResultDTO, error) {
	fake.commands = append(fake.commands, command)
	if len(fake.batches) == 0 {
		return EvidenceLineageBackfillBatchResultDTO{}, errors.New("unexpected batch")
	}
	result := fake.batches[0]
	fake.batches = fake.batches[1:]
	return result, nil
}

func (fake *evidenceLineageBackfillRepositoryFake) CompleteEvidenceLineageBackfill(context.Context, CompleteEvidenceLineageBackfillCommand) (EvidenceLineageMaintenanceRunDTO, error) {
	fake.completed++
	return EvidenceLineageMaintenanceRunDTO{RunID: 41, Status: "completed", ExaminedCount: 3}, nil
}

func TestEvidenceLineageBackfillDryRunIsReadOnly(t *testing.T) {
	repository := &evidenceLineageBackfillRepositoryFake{inspection: EvidenceLineageBackfillInspectionDTO{
		Phase: "source", CandidateCount: 3, Blockers: []string{},
	}}
	service, err := NewEvidenceLineageMaintenanceService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Backfill(context.Background(), EvidenceLineageBackfillCommand{
		Phase: "source", BatchSize: 200, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspection.CandidateCount != 3 || repository.started != 0 || repository.resumed != 0 || len(repository.commands) != 0 || repository.completed != 0 {
		t.Fatalf("dry-run mutated repository: result=%+v fake=%+v", result, repository)
	}
}

func TestEvidenceLineageBackfillApplyRequiresIndependentEvidenceAndProcessesStableCursor(t *testing.T) {
	repository := &evidenceLineageBackfillRepositoryFake{
		inspection: EvidenceLineageBackfillInspectionDTO{Phase: "source", CandidateCount: 3},
		batches: []EvidenceLineageBackfillBatchResultDTO{
			{RunID: 41, ExaminedCount: 2, LastResourceID: 8, HasMore: true},
			{RunID: 41, ExaminedCount: 1, LastResourceID: 11, HasMore: false},
		},
	}
	service, err := NewEvidenceLineageMaintenanceService(repository)
	if err != nil {
		t.Fatal(err)
	}
	command := EvidenceLineageBackfillCommand{
		Phase: "source", BatchSize: 2, Apply: true, ConfirmNonEmpty: true,
		OperatorID: "operator-a", ReviewerID: "reviewer-b",
		BinarySHA256: "a" + string(make([]byte, 0)),
	}
	command.BinarySHA256 = "a" + repeatMaintenanceCharacter("a", 63)
	command.SchemaSHA256 = repeatMaintenanceCharacter("b", 64)
	command.ConfigurationSHA256 = repeatMaintenanceCharacter("c", 64)
	command.BackupEvidenceSHA256 = repeatMaintenanceCharacter("d", 64)
	command.RehearsalEvidenceSHA256 = repeatMaintenanceCharacter("e", 64)

	result, err := service.Backfill(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if repository.started != 1 || repository.completed != 1 || len(repository.commands) != 2 || result.Run.Status != "completed" {
		t.Fatalf("apply result=%+v fake=%+v", result, repository)
	}
	if repository.commands[0].AfterResourceID != 0 || repository.commands[1].AfterResourceID != 8 {
		t.Fatalf("batch cursors = %d,%d, want 0,8", repository.commands[0].AfterResourceID, repository.commands[1].AfterResourceID)
	}
}

func TestEvidenceLineageBackfillRejectsUnsafeMutationCommand(t *testing.T) {
	repository := &evidenceLineageBackfillRepositoryFake{}
	service, err := NewEvidenceLineageMaintenanceService(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Backfill(context.Background(), EvidenceLineageBackfillCommand{
		Phase: "source", BatchSize: 200, Apply: true,
	})
	if err == nil || repository.started != 0 {
		t.Fatalf("unsafe mutation error=%v started=%d", err, repository.started)
	}
}

func repeatMaintenanceCharacter(value string, count int) string {
	result := ""
	for range count {
		result += value
	}
	return result
}
