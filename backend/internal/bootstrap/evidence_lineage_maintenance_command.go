package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func runMaintenanceCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("maintenance command is required: expected backfill-evidence-lineage, reconcile-evidence-lineage, expire-raw-evidence, expire-derived-artifacts, recover-projections, record-backup, or rotate-source-credentials")
	}
	if output == nil {
		return errors.New("maintenance output is required")
	}
	switch args[0] {
	case "backfill-evidence-lineage":
		return runEvidenceLineageBackfillCommand(ctx, cfg, args[1:], output)
	case "reconcile-evidence-lineage":
		return runEvidenceLineageReconciliationCommand(ctx, cfg, args[1:], output)
	case "expire-raw-evidence":
		return runRawEvidenceRetentionCommand(ctx, cfg, args[1:], output)
	case "expire-derived-artifacts":
		return runDerivedArtifactRetentionCommand(ctx, cfg, args[1:], output)
	case "recover-projections":
		return runProjectionRecoveryCommand(ctx, cfg, args[1:], output)
	case "record-backup":
		return runBackupRunCommand(ctx, cfg, args[1:], output)
	case "rotate-source-credentials":
		return runSourceCredentialRotationCommand(ctx, cfg, args[1:], output)
	default:
		return fmt.Errorf("unknown maintenance command %q", args[0])
	}
}

func runEvidenceLineageReconciliationCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	command, err := parseEvidenceLineageReconciliationFlags(args)
	if err != nil {
		return err
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate evidence lineage reconciliation configuration: %w", err)
	}
	if command.Apply {
		command.BinarySHA256, err = runningBinarySHA256()
		if err != nil {
			return err
		}
		command.SchemaSHA256 = bytesSHA256([]byte(canonicaldb.SchemaSQL))
		command.ConfigurationSHA256 = maintenanceConfigurationSHA256(cfg)
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	var repository *operationspostgres.EvidenceLineageMaintenanceRepository
	switch operationsdomain.EvidenceLineageReconciliationScope(command.Scope) {
	case operationsdomain.ReconciliationScopePostgresMinIO:
		repository, err = operationspostgres.NewEvidenceLineageMaintenanceRepositoryWithMinIO(runtime, cfg.MinIO)
	case operationsdomain.ReconciliationScopePostgresVault:
		repository, err = operationspostgres.NewEvidenceLineageMaintenanceRepositoryWithVault(runtime, cfg.VaultPath)
	case operationsdomain.ReconciliationScopeRightsRetention:
		repository = operationspostgres.NewEvidenceLineageMaintenanceRepository(runtime)
	case operationsdomain.ReconciliationScopeAll:
		repository, err = operationspostgres.NewEvidenceLineageMaintenanceRepositoryWithStorage(runtime, cfg)
	default:
		err = errors.New("evidence lineage reconciliation scope is invalid")
	}
	if err != nil {
		return err
	}
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		return err
	}
	result, err := service.Reconcile(ctx, command)
	if err != nil && result.Inspection.Scope == "" {
		return err
	}
	if command.DryRun {
		_, _ = fmt.Fprintf(output,
			"evidence lineage reconciliation dry-run: scope=%s candidates=%d active_producers=%d catalog_fingerprint=%s findings=%s blockers=%v\n",
			result.Inspection.Scope, result.Inspection.CandidateCount, result.Inspection.ActiveProducerCount,
			result.Inspection.CatalogFingerprint, formatEvidenceLineageFindingCounts(result.Inspection.FindingCounts),
			result.Inspection.Blockers)
		if err == nil && len(result.Inspection.Blockers) != 0 {
			err = fmt.Errorf("evidence lineage reconciliation dry-run found blockers: %v", result.Inspection.Blockers)
		}
	}
	if err != nil {
		return err
	}
	if command.Apply {
		_, _ = fmt.Fprintf(output,
			"evidence lineage reconciliation completed: run_id=%d scope=%s examined=%d healthy=%d findings=%d repaired=%d failed=%d last_asset_cursor=%d\n",
			result.Run.RunID, command.Scope, result.Run.ExaminedCount, result.Run.HealthyCount,
			result.Run.FindingCount, result.Run.RepairedCount, result.Run.FailedCount, result.Run.LastAssetCursor)
	}
	return nil
}

func parseEvidenceLineageReconciliationFlags(args []string) (operationsapplication.EvidenceLineageReconciliationCommand, error) {
	flags := flag.NewFlagSet("hotkey maintenance reconcile-evidence-lineage", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	scope := flags.String("scope", "", "evidence lineage reconciliation scope")
	batchSize := flags.Int("batch-size", 200, "bounded stable-cursor batch size")
	gracePeriodHours := flags.Int("grace-period-hours", 24, "untracked storage grace period in hours")
	dryRun := flags.Bool("dry-run", false, "inspect without changing external state")
	apply := flags.Bool("apply", false, "apply conservative evidence lineage repairs")
	confirmNonEmpty := flags.Bool("confirm-non-empty", false, "confirm maintenance of a non-empty catalog")
	resume := flags.Bool("resume", false, "resume an existing running reconciliation")
	runID := flags.String("run-id", "", "existing numeric reconciliation run ID")
	operatorID := flags.String("operator-id", "", "operator record identity")
	reviewerID := flags.String("reviewer-id", "", "independent reviewer record identity")
	backupEvidenceSHA256 := flags.String("backup-evidence-sha256", "", "consistent backup evidence digest")
	rehearsalEvidenceSHA256 := flags.String("rehearsal-evidence-sha256", "", "restored-environment rehearsal evidence digest")
	if err := flags.Parse(args); err != nil {
		return operationsapplication.EvidenceLineageReconciliationCommand{}, fmt.Errorf("parse evidence lineage reconciliation flags: %w", err)
	}
	if flags.NArg() != 0 {
		return operationsapplication.EvidenceLineageReconciliationCommand{}, fmt.Errorf("unexpected evidence lineage reconciliation arguments: %v", flags.Args())
	}
	if !operationsdomain.EvidenceLineageReconciliationScope(*scope).Valid() || *batchSize < 1 || *batchSize > 1000 ||
		*gracePeriodHours < 1 || *gracePeriodHours > 720 || *dryRun == *apply {
		return operationsapplication.EvidenceLineageReconciliationCommand{}, errors.New("evidence lineage reconciliation requires a valid --scope, bounded batch/grace values, and exactly one of --dry-run or --apply")
	}
	command := operationsapplication.EvidenceLineageReconciliationCommand{
		Scope: *scope, BatchSize: *batchSize, GracePeriodHours: *gracePeriodHours,
		DryRun: *dryRun, Apply: *apply, ConfirmNonEmpty: *confirmNonEmpty, Resume: *resume,
		OperatorID: *operatorID, ReviewerID: *reviewerID,
		BackupEvidenceSHA256: *backupEvidenceSHA256, RehearsalEvidenceSHA256: *rehearsalEvidenceSHA256,
	}
	if *runID != "" {
		parsed, err := strconv.ParseInt(*runID, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != *runID {
			return operationsapplication.EvidenceLineageReconciliationCommand{}, errors.New("evidence lineage reconciliation --run-id must be a positive canonical integer")
		}
		command.RunID = parsed
	}
	if command.DryRun {
		if command.ConfirmNonEmpty || command.Resume || command.RunID != 0 || command.OperatorID != "" || command.ReviewerID != "" ||
			command.BackupEvidenceSHA256 != "" || command.RehearsalEvidenceSHA256 != "" {
			return operationsapplication.EvidenceLineageReconciliationCommand{}, errors.New("evidence lineage reconciliation dry-run does not accept mutation evidence")
		}
		return command, nil
	}
	if !command.ConfirmNonEmpty || command.OperatorID == "" || command.ReviewerID == "" || command.OperatorID == command.ReviewerID ||
		!maintenanceFlagSHA256(command.BackupEvidenceSHA256) || !maintenanceFlagSHA256(command.RehearsalEvidenceSHA256) ||
		command.Resume != (command.RunID > 0) {
		return operationsapplication.EvidenceLineageReconciliationCommand{}, errors.New("evidence lineage reconciliation apply requires confirmation, independent operator/reviewer, backup/rehearsal digests, and a consistent resume run ID")
	}
	return command, nil
}

func formatEvidenceLineageFindingCounts(counts []operationsapplication.EvidenceLineageFindingCountDTO) string {
	values := make([]string, 0, len(counts))
	for _, count := range counts {
		values = append(values, count.Finding+"="+strconv.FormatInt(count.Count, 10))
	}
	return strings.Join(values, ",")
}

func runEvidenceLineageBackfillCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	command, err := parseEvidenceLineageBackfillFlags(args)
	if err != nil {
		return err
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate evidence lineage maintenance configuration: %w", err)
	}
	if command.Apply {
		command.BinarySHA256, err = runningBinarySHA256()
		if err != nil {
			return err
		}
		command.SchemaSHA256 = bytesSHA256([]byte(canonicaldb.SchemaSQL))
		command.ConfigurationSHA256 = maintenanceConfigurationSHA256(cfg)
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	service, err := operationsapplication.NewEvidenceLineageMaintenanceService(operationspostgres.NewEvidenceLineageMaintenanceRepository(runtime))
	if err != nil {
		return err
	}
	result, err := service.Backfill(ctx, command)
	if err != nil && result.Inspection.Phase == "" {
		return err
	}
	if command.DryRun {
		_, _ = fmt.Fprintf(output,
			"evidence lineage backfill dry-run: phase=%s candidates=%d mapped=%d blocked=%d active_producers=%d catalog_fingerprint=%s blockers=%v\n",
			result.Inspection.Phase, result.Inspection.CandidateCount, result.Inspection.AlreadyMappedCount,
			result.Inspection.BlockedCount, result.Inspection.ActiveProducerCount,
			result.Inspection.CatalogFingerprint, result.Inspection.Blockers)
		if err == nil && len(result.Inspection.Blockers) != 0 {
			err = fmt.Errorf("evidence lineage backfill dry-run found blockers: %v", result.Inspection.Blockers)
		}
	}
	if err != nil {
		return err
	}
	if command.Apply {
		_, _ = fmt.Fprintf(output,
			"evidence lineage backfill completed: run_id=%d phase=%s examined=%d reused=%d created=%d skipped=%d blocked=%d failed=%d last_resource_id=%d\n",
			result.Run.RunID, command.Phase, result.Run.ExaminedCount, result.Run.ReusedCount,
			result.Run.CreatedCount, result.Run.SkippedCount, result.Run.BlockedCount,
			result.Run.FailedCount, result.Run.LastResourceID)
	}
	return nil
}

func parseEvidenceLineageBackfillFlags(args []string) (operationsapplication.EvidenceLineageBackfillCommand, error) {
	flags := flag.NewFlagSet("hotkey maintenance backfill-evidence-lineage", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	phase := flags.String("phase", "", "evidence lineage migration phase")
	batchSize := flags.Int("batch-size", 200, "bounded stable-cursor batch size")
	dryRun := flags.Bool("dry-run", false, "inspect without changing external state")
	apply := flags.Bool("apply", false, "apply the evidence lineage backfill")
	confirmNonEmpty := flags.Bool("confirm-non-empty", false, "confirm maintenance of a non-empty catalog")
	resume := flags.Bool("resume", false, "resume an existing running migration")
	runID := flags.String("run-id", "", "existing numeric migration run ID")
	operatorID := flags.String("operator-id", "", "operator record identity")
	reviewerID := flags.String("reviewer-id", "", "independent reviewer record identity")
	backupEvidenceSHA256 := flags.String("backup-evidence-sha256", "", "consistent backup evidence digest")
	rehearsalEvidenceSHA256 := flags.String("rehearsal-evidence-sha256", "", "restored-environment rehearsal evidence digest")
	if err := flags.Parse(args); err != nil {
		return operationsapplication.EvidenceLineageBackfillCommand{}, fmt.Errorf("parse evidence lineage backfill flags: %w", err)
	}
	if flags.NArg() != 0 {
		return operationsapplication.EvidenceLineageBackfillCommand{}, fmt.Errorf("unexpected evidence lineage backfill arguments: %v", flags.Args())
	}
	if !operationsdomain.EvidenceLineageMigrationPhase(*phase).Valid() || *batchSize < 1 || *batchSize > 1000 || *dryRun == *apply {
		return operationsapplication.EvidenceLineageBackfillCommand{}, errors.New("evidence lineage backfill requires a valid --phase, bounded --batch-size, and exactly one of --dry-run or --apply")
	}
	command := operationsapplication.EvidenceLineageBackfillCommand{
		Phase: *phase, BatchSize: *batchSize, DryRun: *dryRun, Apply: *apply,
		ConfirmNonEmpty: *confirmNonEmpty, Resume: *resume,
		OperatorID: *operatorID, ReviewerID: *reviewerID,
		BackupEvidenceSHA256:    *backupEvidenceSHA256,
		RehearsalEvidenceSHA256: *rehearsalEvidenceSHA256,
	}
	if *runID != "" {
		parsed, err := strconv.ParseInt(*runID, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != *runID {
			return operationsapplication.EvidenceLineageBackfillCommand{}, errors.New("evidence lineage backfill --run-id must be a positive canonical integer")
		}
		command.RunID = parsed
	}
	if command.DryRun {
		if command.ConfirmNonEmpty || command.Resume || command.RunID != 0 || command.OperatorID != "" || command.ReviewerID != "" ||
			command.BackupEvidenceSHA256 != "" || command.RehearsalEvidenceSHA256 != "" {
			return operationsapplication.EvidenceLineageBackfillCommand{}, errors.New("evidence lineage backfill dry-run does not accept mutation evidence")
		}
		return command, nil
	}
	if !command.ConfirmNonEmpty || command.OperatorID == "" || command.ReviewerID == "" || command.OperatorID == command.ReviewerID ||
		!maintenanceFlagSHA256(command.BackupEvidenceSHA256) || !maintenanceFlagSHA256(command.RehearsalEvidenceSHA256) ||
		command.Resume != (command.RunID > 0) {
		return operationsapplication.EvidenceLineageBackfillCommand{}, errors.New("evidence lineage backfill apply requires confirmation, independent operator/reviewer, backup/rehearsal digests, and a consistent resume run ID")
	}
	return command, nil
}

func runningBinarySHA256() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve maintenance binary: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open maintenance binary: %w", err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash maintenance binary: %w", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func maintenanceConfigurationSHA256(cfg config.Config) string {
	// Credentials, DSNs, token material, and raw paths never enter the
	// maintenance fingerprint. The stable deployment semantics below are
	// sufficient to reject a resume under a changed storage topology.
	value := strings.Join([]string{
		cfg.Environment,
		cfg.MinIO.Endpoint,
		cfg.MinIO.Bucket,
		strconv.FormatBool(cfg.MinIO.UseSSL),
		filepath.Clean(cfg.VaultPath),
	}, "\x00")
	return bytesSHA256([]byte(value))
}

func bytesSHA256(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func maintenanceFlagSHA256(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}
