package application

import (
	"context"
	"errors"
	"testing"
)

type projectionRecoveryRepositoryFake struct {
	inspection ProjectionRecoveryInspectionDTO
	receipt    ProjectionRecoveryReceiptDTO
	inspectErr error
	applyErr   error
	inspected  int
	applied    int
	command    ApplyProjectionRecoveryCommand
}

func (fake *projectionRecoveryRepositoryFake) InspectProjectionRecovery(context.Context) (ProjectionRecoveryInspectionDTO, error) {
	fake.inspected++
	return fake.inspection, fake.inspectErr
}

func (fake *projectionRecoveryRepositoryFake) ApplyProjectionRecovery(_ context.Context, command ApplyProjectionRecoveryCommand) (ProjectionRecoveryReceiptDTO, error) {
	fake.applied++
	fake.command = command
	return fake.receipt, fake.applyErr
}

func TestProjectionRecoveryDryRunNeverMutates(t *testing.T) {
	repository := &projectionRecoveryRepositoryFake{inspection: validProjectionRecoveryInspection()}
	service, err := NewProjectionRecoveryService(repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Recover(context.Background(), ProjectionRecoveryCommand{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if repository.inspected != 1 || repository.applied != 0 || result.Inspection.MissingVaultProjectionCount != 2 {
		t.Fatalf("result=%+v fake=%+v", result, repository)
	}
}

func TestProjectionRecoveryApplyRequiresIsolatedDualControlEvidence(t *testing.T) {
	repository := &projectionRecoveryRepositoryFake{inspection: validProjectionRecoveryInspection()}
	service, err := NewProjectionRecoveryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	command := validProjectionRecoveryCommand()
	command.ProductionEgressDisabled = false

	if _, err := service.Recover(context.Background(), command); err == nil {
		t.Fatal("apply without disabled production egress must fail")
	}
	if repository.inspected != 0 || repository.applied != 0 {
		t.Fatalf("invalid apply reached repository: %+v", repository)
	}
}

func TestProjectionRecoveryApplyStopsOnStartedClaimBlocker(t *testing.T) {
	inspection := validProjectionRecoveryInspection()
	inspection.StartedDeliveryClaimCount = 1
	inspection.Blockers = []string{"started_delivery_claim_requires_provider_reconciliation"}
	repository := &projectionRecoveryRepositoryFake{inspection: inspection}
	service, err := NewProjectionRecoveryService(repository)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Recover(context.Background(), validProjectionRecoveryCommand()); err == nil {
		t.Fatal("started delivery claim blocker must stop recovery")
	}
	if repository.inspected != 1 || repository.applied != 0 {
		t.Fatalf("blocked apply mutated repository: %+v", repository)
	}
}

func TestProjectionRecoveryApplyPreservesFactsAndSchedulesEveryMissingProjection(t *testing.T) {
	inspection := validProjectionRecoveryInspection()
	repository := &projectionRecoveryRepositoryFake{
		inspection: inspection,
		receipt: ProjectionRecoveryReceiptDTO{
			RunID:                                    9,
			RunSHA256:                                repeatMaintenanceCharacter("6", 64),
			Status:                                   "scheduled",
			BeforeFacts:                              inspection.Facts,
			AfterFacts:                               inspection.Facts,
			BeforeVaultManualRegionFingerprintSHA256: inspection.VaultManualRegionFingerprintSHA256,
			AfterVaultManualRegionFingerprintSHA256:  inspection.VaultManualRegionFingerprintSHA256,
			RemovedDeliveryClaimCount:                inspection.DisposableDeliveryClaimCount,
			ScheduledVaultRecoveryCount:              inspection.MissingVaultProjectionCount,
			ScheduledSearchRebuildCount:              inspection.MissingSearchProjectionCount,
			PreservedStartedClaimCount:               inspection.StartedDeliveryClaimCount,
			PreservedUnknownAttemptCount:             inspection.UnknownDeliveryAttemptCount,
		},
	}
	service, err := NewProjectionRecoveryService(repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Recover(context.Background(), validProjectionRecoveryCommand())
	if err != nil {
		t.Fatal(err)
	}
	if repository.applied != 1 || result.Receipt.Status != "scheduled" || repository.command.RunSHA256 != repeatMaintenanceCharacter("6", 64) {
		t.Fatalf("result=%+v fake=%+v", result, repository)
	}
}

func TestProjectionRecoveryRejectsChangedAppendOnlyFacts(t *testing.T) {
	inspection := validProjectionRecoveryInspection()
	changed := inspection.Facts
	changed.DeliveryAttemptCount++
	repository := &projectionRecoveryRepositoryFake{
		inspection: inspection,
		receipt: ProjectionRecoveryReceiptDTO{
			RunID:                                    9,
			RunSHA256:                                repeatMaintenanceCharacter("6", 64),
			Status:                                   "scheduled",
			BeforeFacts:                              inspection.Facts,
			AfterFacts:                               changed,
			BeforeVaultManualRegionFingerprintSHA256: inspection.VaultManualRegionFingerprintSHA256,
			AfterVaultManualRegionFingerprintSHA256:  inspection.VaultManualRegionFingerprintSHA256,
			RemovedDeliveryClaimCount:                inspection.DisposableDeliveryClaimCount,
			ScheduledVaultRecoveryCount:              inspection.MissingVaultProjectionCount,
			ScheduledSearchRebuildCount:              inspection.MissingSearchProjectionCount,
			PreservedUnknownAttemptCount:             inspection.UnknownDeliveryAttemptCount,
		},
	}
	service, err := NewProjectionRecoveryService(repository)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Recover(context.Background(), validProjectionRecoveryCommand()); !errors.Is(err, ErrProjectionRecoveryIntegrity) {
		t.Fatalf("changed facts error=%v", err)
	}
}

func validProjectionRecoveryInspection() ProjectionRecoveryInspectionDTO {
	return ProjectionRecoveryInspectionDTO{
		VaultManualRegionFingerprintSHA256: repeatMaintenanceCharacter("b", 64),
		Facts: ProjectionRecoveryFactSnapshotDTO{
			NotificationOutboxCount: 3, UserNotificationCount: 5, ReadReceiptCount: 2, DeliveryAttemptCount: 4,
			MaxUserNotificationID: 17, MaxReadReceiptID: 8, MaxDeliveryAttemptID: 13,
			FingerprintSHA256: repeatMaintenanceCharacter("a", 64),
		},
		DisposableDeliveryClaimCount: 1,
		UnknownDeliveryAttemptCount:  1,
		MissingVaultProjectionCount:  2,
		MissingSearchProjectionCount: 3,
	}
}

func validProjectionRecoveryCommand() ProjectionRecoveryCommand {
	return ProjectionRecoveryCommand{
		Apply: true, ConfirmIsolated: true, ProductionEgressDisabled: true,
		OperatorID: "operator-a", ReviewerID: "reviewer-b",
		RunSHA256:               repeatMaintenanceCharacter("6", 64),
		BackupEvidenceSHA256:    repeatMaintenanceCharacter("7", 64),
		RehearsalEvidenceSHA256: repeatMaintenanceCharacter("8", 64),
	}
}
