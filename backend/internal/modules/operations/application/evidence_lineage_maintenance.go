package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
)

var maintenanceSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type EvidenceLineageBackfillCommand struct {
	Phase                   string
	BatchSize               int
	DryRun                  bool
	Apply                   bool
	ConfirmNonEmpty         bool
	Resume                  bool
	RunID                   int64
	OperatorID              string
	ReviewerID              string
	BinarySHA256            string
	SchemaSHA256            string
	ConfigurationSHA256     string
	BackupEvidenceSHA256    string
	RehearsalEvidenceSHA256 string
}

type EvidenceLineageBackfillInspectionQuery struct {
	Phase     string
	BatchSize int
}

type EvidenceLineageBackfillInspectionDTO struct {
	Phase               string
	CandidateCount      int64
	AlreadyMappedCount  int64
	BlockedCount        int64
	ActiveProducerCount int64
	CatalogFingerprint  string
	Blockers            []string
}

type StartEvidenceLineageBackfillCommand struct {
	Phase                   string
	BatchSize               int
	OperatorID              string
	ReviewerID              string
	BinarySHA256            string
	SchemaSHA256            string
	ConfigurationSHA256     string
	BackupEvidenceSHA256    string
	RehearsalEvidenceSHA256 string
}

type ResumeEvidenceLineageBackfillCommand struct {
	RunID                   int64
	Phase                   string
	BatchSize               int
	OperatorID              string
	ReviewerID              string
	BinarySHA256            string
	SchemaSHA256            string
	ConfigurationSHA256     string
	BackupEvidenceSHA256    string
	RehearsalEvidenceSHA256 string
}

type EvidenceLineageMaintenanceRunDTO struct {
	RunID          int64
	Status         string
	LastResourceID int64
	ExaminedCount  int64
	ReusedCount    int64
	CreatedCount   int64
	SkippedCount   int64
	BlockedCount   int64
	FailedCount    int64
}

type EvidenceLineageBackfillBatchCommand struct {
	RunID           int64
	Phase           string
	AfterResourceID int64
	BatchSize       int
}

type EvidenceLineageBackfillBatchResultDTO struct {
	RunID          int64
	ExaminedCount  int64
	ReusedCount    int64
	CreatedCount   int64
	SkippedCount   int64
	BlockedCount   int64
	FailedCount    int64
	LastResourceID int64
	HasMore        bool
}

type CompleteEvidenceLineageBackfillCommand struct {
	RunID          int64
	LastResourceID int64
}

type EvidenceLineageBackfillResult struct {
	Inspection EvidenceLineageBackfillInspectionDTO
	Run        EvidenceLineageMaintenanceRunDTO
}

type EvidenceLineageMaintenanceRepository interface {
	InspectEvidenceLineageBackfill(context.Context, EvidenceLineageBackfillInspectionQuery) (EvidenceLineageBackfillInspectionDTO, error)
	StartEvidenceLineageBackfill(context.Context, StartEvidenceLineageBackfillCommand) (EvidenceLineageMaintenanceRunDTO, error)
	ResumeEvidenceLineageBackfill(context.Context, ResumeEvidenceLineageBackfillCommand) (EvidenceLineageMaintenanceRunDTO, error)
	ApplyEvidenceLineageBackfillBatch(context.Context, EvidenceLineageBackfillBatchCommand) (EvidenceLineageBackfillBatchResultDTO, error)
	CompleteEvidenceLineageBackfill(context.Context, CompleteEvidenceLineageBackfillCommand) (EvidenceLineageMaintenanceRunDTO, error)
}

type EvidenceLineageMaintenanceService struct {
	repository EvidenceLineageMaintenanceRepository
}

func NewEvidenceLineageMaintenanceService(repository EvidenceLineageMaintenanceRepository) (*EvidenceLineageMaintenanceService, error) {
	if repository == nil {
		return nil, errors.New("evidence lineage maintenance repository is required")
	}
	return &EvidenceLineageMaintenanceService{repository: repository}, nil
}

func (service *EvidenceLineageMaintenanceService) Backfill(ctx context.Context, command EvidenceLineageBackfillCommand) (EvidenceLineageBackfillResult, error) {
	if service == nil || service.repository == nil {
		return EvidenceLineageBackfillResult{}, errors.New("evidence lineage maintenance service is not initialized")
	}
	if err := validateEvidenceLineageBackfillCommand(command); err != nil {
		return EvidenceLineageBackfillResult{}, err
	}
	inspection, err := service.repository.InspectEvidenceLineageBackfill(ctx, EvidenceLineageBackfillInspectionQuery{
		Phase: command.Phase, BatchSize: command.BatchSize,
	})
	if err != nil {
		return EvidenceLineageBackfillResult{}, fmt.Errorf("inspect evidence lineage backfill: %w", err)
	}
	result := EvidenceLineageBackfillResult{Inspection: cloneEvidenceLineageBackfillInspection(inspection)}
	if command.DryRun {
		return result, nil
	}
	if len(inspection.Blockers) != 0 {
		return result, fmt.Errorf("evidence lineage backfill is blocked: %s", strings.Join(inspection.Blockers, ","))
	}

	run, err := service.startOrResumeBackfill(ctx, command)
	if err != nil {
		return result, err
	}
	if run.RunID <= 0 || run.Status != "running" || run.LastResourceID < 0 {
		return result, errors.New("evidence lineage backfill run receipt is invalid")
	}
	after := run.LastResourceID
	for {
		batch, batchErr := service.repository.ApplyEvidenceLineageBackfillBatch(ctx, EvidenceLineageBackfillBatchCommand{
			RunID: run.RunID, Phase: command.Phase, AfterResourceID: after, BatchSize: command.BatchSize,
		})
		if batchErr != nil {
			return result, fmt.Errorf("apply evidence lineage backfill batch: %w", batchErr)
		}
		if batch.RunID != run.RunID || batch.ExaminedCount < 0 || batch.LastResourceID < after ||
			(batch.HasMore && batch.LastResourceID <= after) {
			return result, errors.New("evidence lineage backfill batch receipt is invalid")
		}
		after = batch.LastResourceID
		if !batch.HasMore {
			break
		}
	}
	completed, err := service.repository.CompleteEvidenceLineageBackfill(ctx, CompleteEvidenceLineageBackfillCommand{
		RunID: run.RunID, LastResourceID: after,
	})
	if err != nil {
		return result, fmt.Errorf("complete evidence lineage backfill: %w", err)
	}
	if completed.RunID != run.RunID || completed.Status != "completed" {
		return result, errors.New("completed evidence lineage backfill receipt is invalid")
	}
	result.Run = completed
	return result, nil
}

func (service *EvidenceLineageMaintenanceService) startOrResumeBackfill(ctx context.Context, command EvidenceLineageBackfillCommand) (EvidenceLineageMaintenanceRunDTO, error) {
	if command.Resume {
		run, err := service.repository.ResumeEvidenceLineageBackfill(ctx, ResumeEvidenceLineageBackfillCommand{
			RunID: command.RunID, Phase: command.Phase, BatchSize: command.BatchSize,
			OperatorID: command.OperatorID, ReviewerID: command.ReviewerID,
			BinarySHA256: command.BinarySHA256, SchemaSHA256: command.SchemaSHA256,
			ConfigurationSHA256:     command.ConfigurationSHA256,
			BackupEvidenceSHA256:    command.BackupEvidenceSHA256,
			RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
		})
		if err != nil {
			return EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("resume evidence lineage backfill: %w", err)
		}
		return run, nil
	}
	run, err := service.repository.StartEvidenceLineageBackfill(ctx, StartEvidenceLineageBackfillCommand{
		Phase: command.Phase, BatchSize: command.BatchSize,
		OperatorID: command.OperatorID, ReviewerID: command.ReviewerID,
		BinarySHA256: command.BinarySHA256, SchemaSHA256: command.SchemaSHA256,
		ConfigurationSHA256:     command.ConfigurationSHA256,
		BackupEvidenceSHA256:    command.BackupEvidenceSHA256,
		RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
	})
	if err != nil {
		return EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("start evidence lineage backfill: %w", err)
	}
	return run, nil
}

func validateEvidenceLineageBackfillCommand(command EvidenceLineageBackfillCommand) error {
	phase := operationsdomain.EvidenceLineageMigrationPhase(command.Phase)
	if !phase.Valid() || command.BatchSize < 1 || command.BatchSize > 1000 || command.DryRun == command.Apply {
		return errors.New("evidence lineage backfill command is invalid")
	}
	if command.DryRun {
		if command.ConfirmNonEmpty || command.Resume || command.RunID != 0 {
			return errors.New("evidence lineage backfill dry-run must be read-only")
		}
		return nil
	}
	if !command.ConfirmNonEmpty || command.OperatorID == command.ReviewerID || !validMaintenanceIdentity(command.OperatorID) || !validMaintenanceIdentity(command.ReviewerID) ||
		!maintenanceSHA256Pattern.MatchString(command.BinarySHA256) || !maintenanceSHA256Pattern.MatchString(command.SchemaSHA256) ||
		!maintenanceSHA256Pattern.MatchString(command.ConfigurationSHA256) || !maintenanceSHA256Pattern.MatchString(command.BackupEvidenceSHA256) ||
		!maintenanceSHA256Pattern.MatchString(command.RehearsalEvidenceSHA256) || command.Resume != (command.RunID > 0) {
		return errors.New("evidence lineage backfill apply evidence is incomplete")
	}
	return nil
}

func validMaintenanceIdentity(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 128 && !strings.ContainsAny(value, "\x00\r\n")
}

func cloneEvidenceLineageBackfillInspection(value EvidenceLineageBackfillInspectionDTO) EvidenceLineageBackfillInspectionDTO {
	value.Blockers = append([]string(nil), value.Blockers...)
	return value
}
