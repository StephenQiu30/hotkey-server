package application

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrProjectionRecoveryBlocked   = errors.New("projection recovery blocked")
	ErrProjectionRecoveryIntegrity = errors.New("projection recovery integrity violation")
)

// ProjectionRecoveryFactSnapshotDTO is a body-free identity of the durable
// notification facts that a projection rebuild must never rewrite. Maxima
// protect stable cursors while the aggregate digest protects every row.
type ProjectionRecoveryFactSnapshotDTO struct {
	NotificationOutboxCount int64
	UserNotificationCount   int64
	ReadReceiptCount        int64
	DeliveryAttemptCount    int64
	MaxUserNotificationID   int64
	MaxReadReceiptID        int64
	MaxDeliveryAttemptID    int64
	FingerprintSHA256       string
}

type ProjectionRecoveryInspectionDTO struct {
	Facts                              ProjectionRecoveryFactSnapshotDTO
	VaultManualRegionFingerprintSHA256 string
	DisposableDeliveryClaimCount       int64
	StartedDeliveryClaimCount          int64
	UnknownDeliveryAttemptCount        int64
	MissingVaultProjectionCount        int64
	MissingSearchProjectionCount       int64
	Blockers                           []string
}

type ProjectionRecoveryCommand struct {
	DryRun                   bool
	Apply                    bool
	ConfirmIsolated          bool
	ProductionEgressDisabled bool
	OperatorID               string
	ReviewerID               string
	RunSHA256                string
	BackupEvidenceSHA256     string
	RehearsalEvidenceSHA256  string
}

// ApplyProjectionRecoveryCommand carries the exact read snapshot into the
// repository transaction. This turns a concurrent fact/catalog change into a
// conflict instead of silently preparing a different recovery set.
type ApplyProjectionRecoveryCommand struct {
	OperatorID                                 string
	ReviewerID                                 string
	RunSHA256                                  string
	BackupEvidenceSHA256                       string
	RehearsalEvidenceSHA256                    string
	ExpectedFacts                              ProjectionRecoveryFactSnapshotDTO
	ExpectedVaultManualRegionFingerprintSHA256 string
	ExpectedDisposableClaimCount               int64
	ExpectedStartedClaimCount                  int64
	ExpectedUnknownAttemptCount                int64
	ExpectedVaultRecoveryCount                 int64
	ExpectedSearchRebuildCount                 int64
}

type ProjectionRecoveryReceiptDTO struct {
	RunID                                    int64
	RunSHA256                                string
	Status                                   string
	BeforeFacts                              ProjectionRecoveryFactSnapshotDTO
	AfterFacts                               ProjectionRecoveryFactSnapshotDTO
	BeforeVaultManualRegionFingerprintSHA256 string
	AfterVaultManualRegionFingerprintSHA256  string
	RemovedDeliveryClaimCount                int64
	ScheduledVaultRecoveryCount              int64
	ScheduledSearchRebuildCount              int64
	PreservedStartedClaimCount               int64
	PreservedUnknownAttemptCount             int64
	Differences                              []string
}

type ProjectionRecoveryResult struct {
	Inspection ProjectionRecoveryInspectionDTO
	Receipt    ProjectionRecoveryReceiptDTO
}

type ProjectionRecoveryRepository interface {
	InspectProjectionRecovery(context.Context) (ProjectionRecoveryInspectionDTO, error)
	ApplyProjectionRecovery(context.Context, ApplyProjectionRecoveryCommand) (ProjectionRecoveryReceiptDTO, error)
}

type ProjectionRecoveryService struct {
	repository ProjectionRecoveryRepository
}

func NewProjectionRecoveryService(repository ProjectionRecoveryRepository) (*ProjectionRecoveryService, error) {
	if repository == nil {
		return nil, errors.New("projection recovery repository is required")
	}
	return &ProjectionRecoveryService{repository: repository}, nil
}

func (service *ProjectionRecoveryService) Recover(ctx context.Context, command ProjectionRecoveryCommand) (ProjectionRecoveryResult, error) {
	if service == nil || service.repository == nil {
		return ProjectionRecoveryResult{}, errors.New("projection recovery service is not initialized")
	}
	if err := validateProjectionRecoveryCommand(command); err != nil {
		return ProjectionRecoveryResult{}, err
	}
	inspection, err := service.repository.InspectProjectionRecovery(ctx)
	if err != nil {
		return ProjectionRecoveryResult{}, fmt.Errorf("inspect projection recovery: %w", err)
	}
	if err := validateProjectionRecoveryInspection(inspection); err != nil {
		return ProjectionRecoveryResult{}, fmt.Errorf("%w: %v", ErrProjectionRecoveryIntegrity, err)
	}
	result := ProjectionRecoveryResult{Inspection: inspection}
	if command.DryRun {
		return result, nil
	}
	if len(inspection.Blockers) != 0 {
		return result, fmt.Errorf("%w: %v", ErrProjectionRecoveryBlocked, inspection.Blockers)
	}
	receipt, err := service.repository.ApplyProjectionRecovery(ctx, ApplyProjectionRecoveryCommand{
		OperatorID: command.OperatorID, ReviewerID: command.ReviewerID, RunSHA256: command.RunSHA256,
		BackupEvidenceSHA256: command.BackupEvidenceSHA256, RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
		ExpectedFacts: inspection.Facts,
		ExpectedVaultManualRegionFingerprintSHA256: inspection.VaultManualRegionFingerprintSHA256,
		ExpectedDisposableClaimCount:               inspection.DisposableDeliveryClaimCount,
		ExpectedStartedClaimCount:                  inspection.StartedDeliveryClaimCount,
		ExpectedUnknownAttemptCount:                inspection.UnknownDeliveryAttemptCount,
		ExpectedVaultRecoveryCount:                 inspection.MissingVaultProjectionCount,
		ExpectedSearchRebuildCount:                 inspection.MissingSearchProjectionCount,
	})
	if err != nil {
		return result, fmt.Errorf("apply projection recovery: %w", err)
	}
	if err := validateProjectionRecoveryReceipt(command, inspection, receipt); err != nil {
		return result, fmt.Errorf("%w: %v", ErrProjectionRecoveryIntegrity, err)
	}
	result.Receipt = receipt
	return result, nil
}

func validateProjectionRecoveryCommand(command ProjectionRecoveryCommand) error {
	if command.DryRun == command.Apply {
		return errors.New("projection recovery requires exactly one of dry-run or apply")
	}
	if command.DryRun {
		if command.ConfirmIsolated || command.ProductionEgressDisabled || command.OperatorID != "" || command.ReviewerID != "" ||
			command.RunSHA256 != "" || command.BackupEvidenceSHA256 != "" || command.RehearsalEvidenceSHA256 != "" {
			return errors.New("projection recovery dry-run does not accept mutation evidence")
		}
		return nil
	}
	if !command.ConfirmIsolated || !command.ProductionEgressDisabled || !validMaintenanceIdentity(command.OperatorID) ||
		!validMaintenanceIdentity(command.ReviewerID) || command.OperatorID == command.ReviewerID ||
		!maintenanceSHA256Pattern.MatchString(command.RunSHA256) || !maintenanceSHA256Pattern.MatchString(command.BackupEvidenceSHA256) ||
		!maintenanceSHA256Pattern.MatchString(command.RehearsalEvidenceSHA256) {
		return errors.New("projection recovery apply evidence is incomplete")
	}
	return nil
}

func validateProjectionRecoveryInspection(inspection ProjectionRecoveryInspectionDTO) error {
	if err := validateProjectionRecoveryFactSnapshot(inspection.Facts); err != nil {
		return err
	}
	if inspection.DisposableDeliveryClaimCount < 0 || inspection.StartedDeliveryClaimCount < 0 ||
		inspection.UnknownDeliveryAttemptCount < 0 || inspection.MissingVaultProjectionCount < 0 ||
		inspection.MissingSearchProjectionCount < 0 ||
		!maintenanceSHA256Pattern.MatchString(inspection.VaultManualRegionFingerprintSHA256) {
		return errors.New("projection recovery inspection counts are invalid")
	}
	for _, blocker := range inspection.Blockers {
		if blocker == "" {
			return errors.New("projection recovery blocker is invalid")
		}
	}
	return nil
}

func validateProjectionRecoveryFactSnapshot(snapshot ProjectionRecoveryFactSnapshotDTO) error {
	if snapshot.NotificationOutboxCount < 0 || snapshot.UserNotificationCount < 0 || snapshot.ReadReceiptCount < 0 ||
		snapshot.DeliveryAttemptCount < 0 || snapshot.MaxUserNotificationID < 0 || snapshot.MaxReadReceiptID < 0 ||
		snapshot.MaxDeliveryAttemptID < 0 || !maintenanceSHA256Pattern.MatchString(snapshot.FingerprintSHA256) {
		return errors.New("projection recovery fact snapshot is invalid")
	}
	return nil
}

func validateProjectionRecoveryReceipt(command ProjectionRecoveryCommand, inspection ProjectionRecoveryInspectionDTO, receipt ProjectionRecoveryReceiptDTO) error {
	if receipt.RunID <= 0 || receipt.RunSHA256 != command.RunSHA256 || receipt.Status != "scheduled" || len(receipt.Differences) != 0 ||
		receipt.BeforeFacts != inspection.Facts || receipt.AfterFacts != inspection.Facts ||
		receipt.BeforeVaultManualRegionFingerprintSHA256 != inspection.VaultManualRegionFingerprintSHA256 ||
		receipt.AfterVaultManualRegionFingerprintSHA256 != inspection.VaultManualRegionFingerprintSHA256 ||
		receipt.RemovedDeliveryClaimCount != inspection.DisposableDeliveryClaimCount ||
		receipt.ScheduledVaultRecoveryCount != inspection.MissingVaultProjectionCount ||
		receipt.ScheduledSearchRebuildCount != inspection.MissingSearchProjectionCount ||
		receipt.PreservedStartedClaimCount != inspection.StartedDeliveryClaimCount ||
		receipt.PreservedUnknownAttemptCount != inspection.UnknownDeliveryAttemptCount {
		return errors.New("projection recovery receipt does not match the inspected immutable catalog")
	}
	return nil
}
