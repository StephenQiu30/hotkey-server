package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
)

type EvidenceLineageReconciliationCommand struct {
	Scope                   string
	BatchSize               int
	GracePeriodHours        int
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

type EvidenceLineageReconciliationInspectionQuery struct {
	Scope            string
	BatchSize        int
	GracePeriodHours int
	FencedAt         time.Time
}

type EvidenceLineageReconciliationInspectionDTO struct {
	Scope               string
	CandidateCount      int64
	ActiveProducerCount int64
	CatalogFingerprint  string
	FindingCounts       []EvidenceLineageFindingCountDTO
	Blockers            []string
}

type EvidenceLineageFindingCountDTO struct {
	Finding string
	Count   int64
}

type StartEvidenceLineageReconciliationCommand struct {
	Scope                   string
	BatchSize               int
	GracePeriodHours        int
	OperatorID              string
	ReviewerID              string
	BinarySHA256            string
	SchemaSHA256            string
	ConfigurationSHA256     string
	BackupEvidenceSHA256    string
	RehearsalEvidenceSHA256 string
}

type ResumeEvidenceLineageReconciliationCommand struct {
	RunID                   int64
	Scope                   string
	BatchSize               int
	GracePeriodHours        int
	OperatorID              string
	ReviewerID              string
	BinarySHA256            string
	SchemaSHA256            string
	ConfigurationSHA256     string
	BackupEvidenceSHA256    string
	RehearsalEvidenceSHA256 string
}

type EvidenceLineageReconciliationRunDTO struct {
	RunID           int64
	Status          string
	FencedAt        time.Time
	LastAssetCursor int64
	ExaminedCount   int64
	HealthyCount    int64
	FindingCount    int64
	RepairedCount   int64
	FailedCount     int64
}

type EvidenceLineageReconciliationBatchCommand struct {
	RunID            int64
	Scope            string
	FencedAt         time.Time
	AfterAssetCursor int64
	BatchSize        int
	GracePeriodHours int
}

type EvidenceLineageReconciliationBatchResultDTO struct {
	RunID           int64
	ExaminedCount   int64
	HealthyCount    int64
	FindingCount    int64
	RepairedCount   int64
	FailedCount     int64
	LastAssetCursor int64
	HasMore         bool
}

type CompleteEvidenceLineageReconciliationCommand struct {
	RunID           int64
	LastAssetCursor int64
}

type EvidenceLineageReconciliationResult struct {
	Inspection EvidenceLineageReconciliationInspectionDTO
	Run        EvidenceLineageReconciliationRunDTO
}

type EvidenceLineageReconciliationRepository interface {
	InspectEvidenceLineageReconciliation(context.Context, EvidenceLineageReconciliationInspectionQuery) (EvidenceLineageReconciliationInspectionDTO, error)
	StartEvidenceLineageReconciliation(context.Context, StartEvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationRunDTO, error)
	ResumeEvidenceLineageReconciliation(context.Context, ResumeEvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationRunDTO, error)
	ApplyEvidenceLineageReconciliationBatch(context.Context, EvidenceLineageReconciliationBatchCommand) (EvidenceLineageReconciliationBatchResultDTO, error)
	CompleteEvidenceLineageReconciliation(context.Context, CompleteEvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationRunDTO, error)
}

type EvidenceLineageReconciliationService struct {
	repository EvidenceLineageReconciliationRepository
}

func NewEvidenceLineageReconciliationService(repository EvidenceLineageReconciliationRepository) (*EvidenceLineageReconciliationService, error) {
	if repository == nil {
		return nil, errors.New("evidence lineage reconciliation repository is required")
	}
	return &EvidenceLineageReconciliationService{repository: repository}, nil
}

func (service *EvidenceLineageReconciliationService) Reconcile(ctx context.Context, command EvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationResult, error) {
	if service == nil || service.repository == nil {
		return EvidenceLineageReconciliationResult{}, errors.New("evidence lineage reconciliation service is not initialized")
	}
	if err := validateEvidenceLineageReconciliationCommand(command); err != nil {
		return EvidenceLineageReconciliationResult{}, err
	}
	inspection, err := service.repository.InspectEvidenceLineageReconciliation(ctx, EvidenceLineageReconciliationInspectionQuery{
		Scope: command.Scope, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
		FencedAt: time.Now().UTC(),
	})
	if err != nil {
		return EvidenceLineageReconciliationResult{}, fmt.Errorf("inspect evidence lineage reconciliation: %w", err)
	}
	inspection.Blockers = append([]string(nil), inspection.Blockers...)
	inspection.FindingCounts = append([]EvidenceLineageFindingCountDTO(nil), inspection.FindingCounts...)
	result := EvidenceLineageReconciliationResult{Inspection: inspection}
	if command.DryRun {
		return result, nil
	}
	if len(inspection.Blockers) != 0 {
		return result, fmt.Errorf("evidence lineage reconciliation is blocked: %s", strings.Join(inspection.Blockers, ","))
	}
	run, err := service.startOrResumeReconciliation(ctx, command)
	if err != nil {
		return result, err
	}
	if run.RunID <= 0 || run.Status != "running" || run.FencedAt.IsZero() || run.LastAssetCursor < 0 {
		return result, errors.New("evidence lineage reconciliation run receipt is invalid")
	}
	after := run.LastAssetCursor
	for {
		batch, batchErr := service.repository.ApplyEvidenceLineageReconciliationBatch(ctx, EvidenceLineageReconciliationBatchCommand{
			RunID: run.RunID, Scope: command.Scope, FencedAt: run.FencedAt, AfterAssetCursor: after,
			BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
		})
		if batchErr != nil {
			return result, fmt.Errorf("apply evidence lineage reconciliation batch: %w", batchErr)
		}
		if batch.RunID != run.RunID || batch.ExaminedCount < 0 || batch.LastAssetCursor < after ||
			(batch.HasMore && batch.LastAssetCursor <= after) {
			return result, errors.New("evidence lineage reconciliation batch receipt is invalid")
		}
		after = batch.LastAssetCursor
		if !batch.HasMore {
			break
		}
	}
	completed, err := service.repository.CompleteEvidenceLineageReconciliation(ctx, CompleteEvidenceLineageReconciliationCommand{RunID: run.RunID, LastAssetCursor: after})
	if err != nil {
		return result, fmt.Errorf("complete evidence lineage reconciliation: %w", err)
	}
	if completed.RunID != run.RunID || completed.Status != "completed" || !completed.FencedAt.Equal(run.FencedAt) {
		return result, errors.New("completed evidence lineage reconciliation receipt is invalid")
	}
	result.Run = completed
	return result, nil
}

func (service *EvidenceLineageReconciliationService) startOrResumeReconciliation(ctx context.Context, command EvidenceLineageReconciliationCommand) (EvidenceLineageReconciliationRunDTO, error) {
	if command.Resume {
		value, err := service.repository.ResumeEvidenceLineageReconciliation(ctx, ResumeEvidenceLineageReconciliationCommand{
			RunID: command.RunID, Scope: command.Scope, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
			OperatorID: command.OperatorID, ReviewerID: command.ReviewerID, BinarySHA256: command.BinarySHA256,
			SchemaSHA256: command.SchemaSHA256, ConfigurationSHA256: command.ConfigurationSHA256,
			BackupEvidenceSHA256: command.BackupEvidenceSHA256, RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
		})
		if err != nil {
			return EvidenceLineageReconciliationRunDTO{}, fmt.Errorf("resume evidence lineage reconciliation: %w", err)
		}
		return value, nil
	}
	value, err := service.repository.StartEvidenceLineageReconciliation(ctx, StartEvidenceLineageReconciliationCommand{
		Scope: command.Scope, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
		OperatorID: command.OperatorID, ReviewerID: command.ReviewerID, BinarySHA256: command.BinarySHA256,
		SchemaSHA256: command.SchemaSHA256, ConfigurationSHA256: command.ConfigurationSHA256,
		BackupEvidenceSHA256: command.BackupEvidenceSHA256, RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
	})
	if err != nil {
		return EvidenceLineageReconciliationRunDTO{}, fmt.Errorf("start evidence lineage reconciliation: %w", err)
	}
	return value, nil
}

func validateEvidenceLineageReconciliationCommand(command EvidenceLineageReconciliationCommand) error {
	if !operationsdomain.EvidenceLineageReconciliationScope(command.Scope).Valid() || command.BatchSize < 1 || command.BatchSize > 1000 ||
		command.GracePeriodHours < 1 || command.GracePeriodHours > 720 || command.DryRun == command.Apply {
		return errors.New("evidence lineage reconciliation command is invalid")
	}
	if command.DryRun {
		if command.ConfirmNonEmpty || command.Resume || command.RunID != 0 {
			return errors.New("evidence lineage reconciliation dry-run must be read-only")
		}
		return nil
	}
	if !command.ConfirmNonEmpty || command.OperatorID == command.ReviewerID || !validMaintenanceIdentity(command.OperatorID) || !validMaintenanceIdentity(command.ReviewerID) ||
		!maintenanceSHA256Pattern.MatchString(command.BinarySHA256) || !maintenanceSHA256Pattern.MatchString(command.SchemaSHA256) ||
		!maintenanceSHA256Pattern.MatchString(command.ConfigurationSHA256) || !maintenanceSHA256Pattern.MatchString(command.BackupEvidenceSHA256) ||
		!maintenanceSHA256Pattern.MatchString(command.RehearsalEvidenceSHA256) || command.Resume != (command.RunID > 0) {
		return errors.New("evidence lineage reconciliation apply evidence is incomplete")
	}
	return nil
}
