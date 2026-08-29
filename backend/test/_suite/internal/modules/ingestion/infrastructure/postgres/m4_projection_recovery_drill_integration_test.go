//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	ingestiontextstructure "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/textstructure"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	knowledgejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/jobs"
	knowledgeminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/minio"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/postgres"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	platformdatabase "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	goredis "github.com/redis/go-redis/v9"
)

const m4ProjectionRecoveryEvidenceVersion = "hotkey-m4-projection-recovery-drill-v1"

type m4ProjectionRecoveryConfig struct {
	Output, Environment, Hardware, GitRevision string
	ProductionEgressDisabled                   bool
}

type m4ProjectionRecoveryFactEvidence struct {
	NotificationOutboxCount int64  `json:"notification_outbox_count"`
	UserNotificationCount   int64  `json:"user_notification_count"`
	ReadReceiptCount        int64  `json:"read_receipt_count"`
	DeliveryAttemptCount    int64  `json:"delivery_attempt_count"`
	MaxUserNotificationID   int64  `json:"max_user_notification_id"`
	MaxReadReceiptID        int64  `json:"max_read_receipt_id"`
	MaxDeliveryAttemptID    int64  `json:"max_delivery_attempt_id"`
	FingerprintSHA256       string `json:"fingerprint_sha256"`
}

type m4ProjectionRecoveryFactIDs struct {
	NotificationOutboxIDs []int64 `json:"notification_outbox_ids"`
	UserNotificationIDs   []int64 `json:"user_notification_ids"`
	ReadReceiptIDs        []int64 `json:"read_receipt_ids"`
	DeliveryAttemptIDs    []int64 `json:"delivery_attempt_ids"`
}

type m4ProjectionRecoveryReport struct {
	Version                  string `json:"version"`
	Status                   string `json:"status"`
	Approval                 string `json:"approval"`
	GitRevision              string `json:"git_revision"`
	Environment              string `json:"environment"`
	Hardware                 string `json:"hardware"`
	GOOS                     string `json:"goos"`
	GOARCH                   string `json:"goarch"`
	LogicalCPUs              int    `json:"logical_cpus"`
	Isolated                 bool   `json:"isolated"`
	ProductionEgressDisabled bool   `json:"production_egress_disabled"`
	IndependentCopies        struct {
		PostgreSQL bool `json:"postgresql"`
		MinIO      bool `json:"minio"`
		Vault      bool `json:"vault"`
	} `json:"independent_copies"`
	BackupSHA256              string                                  `json:"backup_sha256"`
	ProtectedSnapshotSHA256   string                                  `json:"protected_snapshot_sha256"`
	BeforeFacts               m4ProjectionRecoveryFactEvidence        `json:"before_facts"`
	AfterFacts                m4ProjectionRecoveryFactEvidence        `json:"after_facts"`
	BeforeFactIDs             m4ProjectionRecoveryFactIDs             `json:"before_fact_ids"`
	AfterFactIDs              m4ProjectionRecoveryFactIDs             `json:"after_fact_ids"`
	BeforeManualRegionSHA256  string                                  `json:"before_manual_region_sha256"`
	AfterManualRegionSHA256   string                                  `json:"after_manual_region_sha256"`
	DisposableClaimsBefore    int64                                   `json:"disposable_claims_before"`
	DisposableClaimsAfter     int64                                   `json:"disposable_claims_after"`
	UnknownAttemptsBefore     int64                                   `json:"unknown_attempts_before"`
	UnknownAttemptsAfter      int64                                   `json:"unknown_attempts_after"`
	MissingVaultBefore        int64                                   `json:"missing_vault_before"`
	MissingVaultAfter         int64                                   `json:"missing_vault_after"`
	MissingSearchBefore       int64                                   `json:"missing_search_before"`
	MissingSearchAfter        int64                                   `json:"missing_search_after"`
	RedisEphemeralKeysRemoved int64                                   `json:"redis_ephemeral_keys_removed"`
	ScheduledJobs             int64                                   `json:"scheduled_jobs"`
	CompletedJobs             int64                                   `json:"completed_jobs"`
	FailedJobAttempts         int64                                   `json:"failed_job_attempts"`
	RebuildDurationMicros     int64                                   `json:"rebuild_duration_micros"`
	RevocationVisibleMicros   int64                                   `json:"revocation_visible_micros"`
	SearchRowsAfterRevocation int64                                   `json:"search_rows_after_revocation"`
	VisibleAfterRevocation    int64                                   `json:"visible_after_revocation"`
	ProjectionRecoveryRunID   int64                                   `json:"projection_recovery_run_id"`
	ProviderDisposition       m4ProjectionRecoveryProviderDisposition `json:"provider_disposition"`
	Differences               []string                                `json:"differences"`
}

type m4ProjectionRecoveryProviderDisposition struct {
	ProviderCalls     int64   `json:"provider_calls"`
	BlindReplays      int64   `json:"blind_replays"`
	UnknownAttemptIDs []int64 `json:"unknown_attempt_ids"`
	Disposition       string  `json:"disposition"`
}

func TestM4ProjectionRecoveryDrillRestoresIndependentCopyToZeroDifference(t *testing.T) {
	cfg := loadM4ProjectionRecoveryConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	drillStartedAt := time.Now()

	sourceRuntime := openDocumentVersionRuntime(t)
	defer func() { _ = sourceRuntime.Close() }()
	sourceVaultRoot := t.TempDir()
	targetVaultRoot := t.TempDir()
	sourceSnapshot, targetSnapshot, independentMinIO, cleanupMinIO := openM4ProjectionRecoveryMinIO(t, ctx)
	t.Cleanup(cleanupMinIO)

	knowledgeDocumentID, protectedKnowledge, manualSHA := createM4RecoverableKnowledge(
		t, ctx, sourceRuntime, sourceVaultRoot, sourceSnapshot,
	)
	fixture := createDerivedArtifactDocument(t, sourceRuntime, "m4-projection-recovery", 96)
	createDerivedArtifactRights(t, sourceRuntime, fixture, 1)
	_ = createDocumentDisplayDecision(
		t, sourceRuntime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID,
	)
	evidence, extraction := createM4SourceGenerationEvidence(t, sourceRuntime, fixture)
	sourceGenerator := newM4SourceDocumentGenerator(
		t, sourceRuntime, sourceVaultRoot, fixture.documentVersions, evidence, extraction,
	)
	generated, err := sourceGenerator.Generate(ctx, ingestionapplication.GenerateSourceDocumentCommand{EvidenceReferenceID: evidence.EvidenceReferenceID})
	if err != nil || generated.SearchProjection == nil || generated.PlaintextArtifact == nil || generated.MarkdownArtifact == nil {
		t.Fatalf("create source projections = %#v/%v", generated, err)
	}
	contentID := insertM4SearchContent(t, sourceRuntime, fixture)
	query := searchdomain.Query{Keyword: "authorized", Types: []searchdomain.ResourceType{searchdomain.ResourceContent}, Limit: 10}.Normalized()
	sourceSearch := ingestionpostgres.NewContentRepository(sourceRuntime)
	sourceCandidates, err := sourceSearch.Search(ctx, query)
	if err != nil || len(sourceCandidates) != 1 || sourceCandidates[0].ID != contentID {
		t.Fatalf("source search projection is not readable = %#v/%v", sourceCandidates, err)
	}

	insertM4ProjectionRecoveryNotificationFixture(t, sourceRuntime.SQL)
	backupPath := filepath.Join(t.TempDir(), "m4-projection-recovery.dump")
	backupSHA := dumpM4ProjectionRecoveryDatabase(t, ctx, sourceRuntime.Pool.Config().ConnString(), backupPath)
	targetDSN := postgresfixture.New(t)
	restoreM4ProjectionRecoveryDatabase(t, ctx, targetDSN, backupPath)
	targetRuntime, err := platformdatabase.Open(ctx, targetDSN)
	if err != nil {
		t.Fatalf("open independent restored PostgreSQL: %v", err)
	}
	defer func() { _ = targetRuntime.Close() }()

	copyM4VaultTree(t, sourceVaultRoot, targetVaultRoot)
	protectedFromSource, err := sourceSnapshot.ReadVaultSnapshot(ctx, knowledgeminio.ObjectKey(knowledgeDocumentID, 1), 4<<20)
	if err != nil || protectedFromSource != protectedKnowledge {
		t.Fatalf("read protected source snapshot: %v", err)
	}
	if err := targetSnapshot.Put(ctx, knowledgeminio.ObjectKey(knowledgeDocumentID, 1), protectedFromSource); err != nil {
		t.Fatalf("copy protected snapshot into independent MinIO bucket: %v", err)
	}
	protectedFromTarget, err := targetSnapshot.ReadVaultSnapshot(ctx, knowledgeminio.ObjectKey(knowledgeDocumentID, 1), 4<<20)
	if err != nil || protectedFromTarget != protectedKnowledge {
		t.Fatalf("verify independent protected snapshot: %v", err)
	}
	protectedSnapshotSHA := m4SHA256(protectedFromTarget)

	knowledgeRelativePath := filepath.Join("events", fmt.Sprintf("%d.md", knowledgeDocumentID))
	if err := os.Remove(filepath.Join(targetVaultRoot, knowledgeRelativePath)); err != nil {
		t.Fatalf("clear disposable knowledge projection from restored copy: %v", err)
	}
	var restoredSearchRows int64
	if err := targetRuntime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM document_version_search_indexes WHERE document_version_id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&restoredSearchRows); err != nil || restoredSearchRows != 0 {
		t.Fatalf("restored disposable search projection rows = %d/%v, want 0", restoredSearchRows, err)
	}
	assertM4SourceCopyStayedIntact(t, sourceRuntime, sourceVaultRoot, knowledgeRelativePath, fixture.persisted.DocumentVersion.ID)
	redisKeysRemoved := clearM4ProjectionRecoveryEphemeralState(t, ctx)

	targetKnowledgeRepository := knowledgepostgres.NewRepository(targetRuntime)
	targetVaultWriter := knowledgevault.NewWriter(targetVaultRoot)
	vaultRecovery := knowledgeapplication.NewVaultRecoveryService(
		targetKnowledgeRepository, targetVaultWriter, targetSnapshot, nil,
	)
	recoveryRepository, err := operationspostgres.NewProjectionRecoveryRepository(
		targetRuntime, vaultRecovery, queue.NewStore(targetRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := operationsapplication.NewProjectionRecoveryService(recoveryRepository)
	if err != nil {
		t.Fatal(err)
	}
	before, err := recovery.Recover(ctx, operationsapplication.ProjectionRecoveryCommand{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if before.Inspection.DisposableDeliveryClaimCount != 1 || before.Inspection.StartedDeliveryClaimCount != 0 ||
		before.Inspection.UnknownDeliveryAttemptCount != 1 || before.Inspection.MissingVaultProjectionCount != 1 ||
		before.Inspection.MissingSearchProjectionCount != 1 || len(before.Inspection.Blockers) != 0 {
		t.Fatalf("restored-copy preflight = %+v", before.Inspection)
	}
	beforeFactIDs := readM4ProjectionRecoveryFactIDs(t, targetRuntime.SQL)

	rehearsalSHA := m4SHA256(fmt.Sprintf(
		"m4-rehearsal-v1:%s:%s:%d:%d", backupSHA, protectedSnapshotSHA,
		before.Inspection.MissingVaultProjectionCount, before.Inspection.MissingSearchProjectionCount,
	))
	runSHA := m4SHA256("m4-projection-recovery-run-v1:" + backupSHA + ":" + cfg.GitRevision)
	apply, err := recovery.Recover(ctx, operationsapplication.ProjectionRecoveryCommand{
		Apply: true, ConfirmIsolated: true, ProductionEgressDisabled: true,
		OperatorID: "automated-operator-fixture", ReviewerID: "independent-reviewer-fixture",
		RunSHA256: runSHA, BackupEvidenceSHA256: backupSHA, RehearsalEvidenceSHA256: rehearsalSHA,
	})
	if err != nil {
		t.Fatal(err)
	}
	if apply.Receipt.RemovedDeliveryClaimCount != 1 || apply.Receipt.ScheduledVaultRecoveryCount != 1 ||
		apply.Receipt.ScheduledSearchRebuildCount != 1 || apply.Receipt.PreservedUnknownAttemptCount != 1 ||
		len(apply.Receipt.Differences) != 0 {
		t.Fatalf("projection recovery receipt = %+v", apply.Receipt)
	}

	targetDocumentVersions := newM4DocumentVersionService(t, targetRuntime, fixture)
	targetGenerator := newM4SourceDocumentGenerator(
		t, targetRuntime, targetVaultRoot, targetDocumentVersions, evidence, extraction,
	)
	vaultHandler, err := knowledgejobs.NewHandler(queue.KindProjectKnowledge, func(ctx context.Context, documentID int64) error {
		_, recoverErr := vaultRecovery.Recover(ctx, documentID)
		return recoverErr
	})
	if err != nil {
		t.Fatal(err)
	}
	searchHandler, err := ingestionjobs.NewSourceDocumentGenerationHandler(targetGenerator)
	if err != nil {
		t.Fatal(err)
	}
	worker := queue.NewWorker(targetRuntime, map[string]queue.Handler{
		queue.KindProjectKnowledge:       vaultHandler.Handle,
		queue.KindGenerateSourceDocument: searchHandler.Handle,
	})
	rebuildStartedAt := time.Now()
	worked := int64(0)
	for attempt := 0; attempt < 4; attempt++ {
		claimed, runErr := worker.RunOnce(ctx)
		if runErr != nil {
			t.Fatalf("recovery Worker attempt %d: %v", attempt+1, runErr)
		}
		if !claimed {
			break
		}
		worked++
	}
	if worked != 2 {
		t.Fatalf("completed recovery Worker jobs = %d, want 2", worked)
	}
	after, err := recovery.Recover(ctx, operationsapplication.ProjectionRecoveryCommand{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	rebuildDuration := time.Since(rebuildStartedAt)
	if after.Inspection.DisposableDeliveryClaimCount != 0 || after.Inspection.MissingVaultProjectionCount != 0 ||
		after.Inspection.MissingSearchProjectionCount != 0 || len(after.Inspection.Blockers) != 0 ||
		after.Inspection.Facts != before.Inspection.Facts ||
		after.Inspection.VaultManualRegionFingerprintSHA256 != before.Inspection.VaultManualRegionFingerprintSHA256 {
		t.Fatalf("post-recovery inspection = %+v, before = %+v", after.Inspection, before.Inspection)
	}
	afterFactIDs := readM4ProjectionRecoveryFactIDs(t, targetRuntime.SQL)
	if !reflect.DeepEqual(beforeFactIDs, afterFactIDs) {
		t.Fatalf("protected fact IDs changed: before=%+v after=%+v", beforeFactIDs, afterFactIDs)
	}

	recoveredKnowledge, err := os.ReadFile(filepath.Join(targetVaultRoot, knowledgeRelativePath))
	if err != nil || string(recoveredKnowledge) != protectedKnowledge {
		t.Fatalf("recovered knowledge bytes changed: %v", err)
	}
	afterManualSHA, err := knowledgedomain.VaultHumanRegionSHA256(string(recoveredKnowledge))
	if err != nil || afterManualSHA != manualSHA {
		t.Fatalf("recovered manual region hash = %q/%v, want %q", afterManualSHA, err, manualSHA)
	}
	targetSearch := ingestionpostgres.NewContentRepository(targetRuntime)
	rebuiltCandidates, err := targetSearch.Search(ctx, query)
	if err != nil || len(rebuiltCandidates) != 1 || rebuiltCandidates[0].ID != contentID {
		t.Fatalf("rebuilt production search = %#v/%v", rebuiltCandidates, err)
	}
	if visible, visibilityErr := targetSearch.CanDisplay(ctx, query, rebuiltCandidates[0]); visibilityErr != nil || !visible {
		t.Fatalf("rebuilt candidate visibility = %t/%v", visible, visibilityErr)
	}

	var completedJobs, failedJobAttempts, durableRuns int64
	if err := targetRuntime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE state='completed' AND kind IN ('project_knowledge','generate_source_document')`).Scan(&completedJobs); err != nil {
		t.Fatal(err)
	}
	if err := targetRuntime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job_attempt`).Scan(&failedJobAttempts); err != nil {
		t.Fatal(err)
	}
	if err := targetRuntime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM projection_recovery_runs WHERE id=$1 AND run_sha256=$2`, apply.Receipt.RunID, runSHA).Scan(&durableRuns); err != nil {
		t.Fatal(err)
	}
	if completedJobs != 2 || failedJobAttempts != 0 || durableRuns != 1 {
		t.Fatalf("durable recovery evidence = jobs %d attempts %d runs %d", completedJobs, failedJobAttempts, durableRuns)
	}

	revocationStartedAt := time.Now()
	revocationPolicy := createDocumentRightsPolicy(t, targetRuntime, fixture.sourceID, 99, time.Now().UTC().Add(-time.Hour))
	insertDocumentRightsDecisionWithOutcome(
		t, targetRuntime, revocationPolicy, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, "store_derived", "deny", nil, nil,
		fixture.persisted.DocumentVersion.ID,
	)
	afterRevocation, err := targetSearch.Search(ctx, query)
	if err != nil || len(afterRevocation) != 0 {
		t.Fatalf("revoked projection remained query-visible = %#v/%v", afterRevocation, err)
	}
	if visible, visibilityErr := targetSearch.CanDisplay(ctx, query, rebuiltCandidates[0]); visibilityErr != nil || visible {
		t.Fatalf("revoked candidate visibility = %t/%v", visible, visibilityErr)
	}
	revocationVisible := time.Since(revocationStartedAt)
	var searchRowsAfterRevocation, claimsAfter, unknownAfter int64
	if err := targetRuntime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM document_version_search_indexes WHERE document_version_id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&searchRowsAfterRevocation); err != nil {
		t.Fatal(err)
	}
	if err := targetRuntime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM notification_delivery_claims`).Scan(&claimsAfter); err != nil {
		t.Fatal(err)
	}
	if err := targetRuntime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM notification_delivery_attempts WHERE status='unknown'`).Scan(&unknownAfter); err != nil {
		t.Fatal(err)
	}

	report := m4ProjectionRecoveryReport{
		Version: m4ProjectionRecoveryEvidenceVersion, Status: "verified", Approval: "automated_isolated_fixture",
		GitRevision: cfg.GitRevision, Environment: cfg.Environment, Hardware: cfg.Hardware,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(),
		Isolated: true, ProductionEgressDisabled: cfg.ProductionEgressDisabled,
		BackupSHA256: backupSHA, ProtectedSnapshotSHA256: protectedSnapshotSHA,
		BeforeFacts:   m4ProjectionRecoveryFacts(before.Inspection.Facts),
		AfterFacts:    m4ProjectionRecoveryFacts(after.Inspection.Facts),
		BeforeFactIDs: beforeFactIDs, AfterFactIDs: afterFactIDs,
		BeforeManualRegionSHA256: before.Inspection.VaultManualRegionFingerprintSHA256,
		AfterManualRegionSHA256:  after.Inspection.VaultManualRegionFingerprintSHA256,
		DisposableClaimsBefore:   before.Inspection.DisposableDeliveryClaimCount, DisposableClaimsAfter: claimsAfter,
		UnknownAttemptsBefore: before.Inspection.UnknownDeliveryAttemptCount, UnknownAttemptsAfter: unknownAfter,
		MissingVaultBefore: before.Inspection.MissingVaultProjectionCount, MissingVaultAfter: after.Inspection.MissingVaultProjectionCount,
		MissingSearchBefore: before.Inspection.MissingSearchProjectionCount, MissingSearchAfter: after.Inspection.MissingSearchProjectionCount,
		RedisEphemeralKeysRemoved: redisKeysRemoved,
		ScheduledJobs:             apply.Receipt.ScheduledVaultRecoveryCount + apply.Receipt.ScheduledSearchRebuildCount,
		CompletedJobs:             completedJobs, FailedJobAttempts: failedJobAttempts,
		RebuildDurationMicros: rebuildDuration.Microseconds(), RevocationVisibleMicros: revocationVisible.Microseconds(),
		SearchRowsAfterRevocation: searchRowsAfterRevocation, VisibleAfterRevocation: int64(len(afterRevocation)),
		ProjectionRecoveryRunID: apply.Receipt.RunID,
		ProviderDisposition: m4ProjectionRecoveryProviderDisposition{
			ProviderCalls: 0, BlindReplays: 0,
			UnknownAttemptIDs: append([]int64(nil), afterFactIDs.DeliveryAttemptIDs...),
			Disposition:       "manual_provider_reconciliation_required_before_any_replay",
		},
		Differences: []string{},
	}
	report.IndependentCopies.PostgreSQL = sourceRuntime.Pool.Config().ConnString() != targetDSN
	report.IndependentCopies.MinIO = independentMinIO
	report.IndependentCopies.Vault = sourceVaultRoot != targetVaultRoot
	if !report.IndependentCopies.PostgreSQL || !report.IndependentCopies.MinIO || !report.IndependentCopies.Vault ||
		report.BeforeFacts != report.AfterFacts || !reflect.DeepEqual(report.BeforeFactIDs, report.AfterFactIDs) ||
		report.BeforeManualRegionSHA256 != report.AfterManualRegionSHA256 || report.DisposableClaimsAfter != 0 ||
		report.UnknownAttemptsAfter != 1 || report.MissingVaultAfter != 0 || report.MissingSearchAfter != 0 ||
		report.CompletedJobs != report.ScheduledJobs || report.FailedJobAttempts != 0 || report.VisibleAfterRevocation != 0 ||
		len(report.Differences) != 0 {
		t.Fatalf("M4 recovery report is not closed = %+v", report)
	}
	if cfg.Output != "" {
		writeM4ProjectionRecoveryEvidence(t, cfg.Output, report)
	}
	t.Logf("M4 independent recovery verified in %s (rebuild=%dus revocation=%dus)", time.Since(drillStartedAt), report.RebuildDurationMicros, report.RevocationVisibleMicros)
}

func TestM4ProjectionRecoveryEvidenceWriterIsExclusiveAndPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.json")
	report := m4ProjectionRecoveryReport{Version: m4ProjectionRecoveryEvidenceVersion, Status: "verified", Differences: []string{}}
	writeM4ProjectionRecoveryEvidence(t, path, report)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode = %o, want 600", info.Mode().Perm())
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_ = file.Close()
		t.Fatal("exclusive evidence path was overwritten")
	}
	if !os.IsExist(err) {
		t.Fatalf("second exclusive open = %v, want exists", err)
	}
}

func loadM4ProjectionRecoveryConfig(t *testing.T) m4ProjectionRecoveryConfig {
	t.Helper()
	cfg := m4ProjectionRecoveryConfig{
		Output:                   strings.TrimSpace(os.Getenv("HOTKEY_M4_PROJECTION_RECOVERY_OUTPUT")),
		Environment:              strings.TrimSpace(os.Getenv("HOTKEY_M4_PROJECTION_RECOVERY_ENVIRONMENT")),
		Hardware:                 strings.TrimSpace(os.Getenv("HOTKEY_M4_PROJECTION_RECOVERY_HARDWARE")),
		GitRevision:              strings.TrimSpace(os.Getenv("HOTKEY_M4_PROJECTION_RECOVERY_GIT_REVISION")),
		ProductionEgressDisabled: strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_M4_PROJECTION_RECOVERY_PRODUCTION_EGRESS_DISABLED")), "true"),
	}
	if cfg.Output == "" {
		cfg.Environment = "isolated-test-fixture"
		cfg.Hardware = "unpersisted-test-runtime"
		cfg.GitRevision = strings.Repeat("0", 40)
		cfg.ProductionEgressDisabled = true
		return cfg
	}
	if cfg.Environment == "" || cfg.Hardware == "" || strings.ContainsAny(cfg.Environment+cfg.Hardware, "\x00\r\n") ||
		len(cfg.Environment) > 256 || len(cfg.Hardware) > 512 || len(cfg.GitRevision) != 40 ||
		strings.Trim(cfg.GitRevision, "0123456789abcdef") != "" || !cfg.ProductionEgressDisabled {
		t.Fatal("M4 projection recovery evidence requires safe environment/hardware, a lowercase commit SHA and disabled production egress")
	}
	return cfg
}

func createM4RecoverableKnowledge(
	t *testing.T, ctx context.Context, runtime *platformdatabase.Runtime, vaultRoot string,
	snapshot *knowledgeminio.Store,
) (int64, string, string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var eventID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events(event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at) VALUES ($1,'M4 recovery knowledge','bounded fixture summary','active',$2,$2) RETURNING id`, fmt.Sprintf("m4-recovery-%d", now.UnixNano()), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	repository := knowledgepostgres.NewRepository(runtime)
	documentID := int64(9701)
	emptyHash := knowledgedomain.HashContent("", "")
	document := knowledgedomain.Document{
		ID: documentID, Version: 1, RevisionNo: 0, Type: knowledgedomain.DocumentEvent, EventID: &eventID,
		VaultPath: fmt.Sprintf("events/%d.md", documentID), ContentHash: emptyHash, GeneratedHash: emptyHash,
		Status: knowledgedomain.DocumentPlanned,
	}
	if err := repository.SaveDocument(ctx, document); err != nil {
		t.Fatal(err)
	}
	proposal := knowledgedomain.Proposal{
		ID: 9801, Version: 1, DocumentID: documentID, BaseRevisionNo: 0, BaseHash: emptyHash,
		ProposedFrontmatter: `{"title":"M4 recovery knowledge"}`,
		ProposedBody:        "approved recovery body", Reason: "independent recovery fixture", Status: knowledgedomain.ProposalPending,
	}
	if err := repository.SaveProposal(proposal); err != nil {
		t.Fatal(err)
	}
	approved, err := repository.UpdateProposalStatus(ctx, proposal.ID, proposal.Version, knowledgedomain.ProposalApproved)
	if err != nil {
		t.Fatal(err)
	}
	renderInput := knowledgedomain.VaultDocumentRenderInput{
		DocumentID: documentID, RevisionNo: 1, Type: document.Type, SourceID: eventID,
		Title: "M4 recovery knowledge", Generated: approved.ProposedBody,
	}
	protected, err := knowledgedomain.RenderVaultDocument(renderInput)
	if err != nil {
		t.Fatal(err)
	}
	human := knowledgedomain.HumanRegionBegin + "\n人工恢复笔记  \n\n- [ ] 保留原始空白\n" + knowledgedomain.HumanRegionEnd
	protected = strings.Replace(protected, knowledgedomain.HumanRegionBegin+"\n"+knowledgedomain.HumanRegionEnd, human, 1)
	manualSHA, err := knowledgedomain.VaultHumanRegionSHA256(protected)
	if err != nil {
		t.Fatal(err)
	}
	writer := knowledgevault.NewWriter(vaultRoot)
	if _, err := writer.Write("events", fmt.Sprint(documentID), protected); err != nil {
		t.Fatalf("write source knowledge projection: %v", err)
	}
	contentHash := knowledgedomain.HashContent("", protected)
	snapshotKey := knowledgeminio.ObjectKey(documentID, 1)
	if err := snapshot.Put(ctx, snapshotKey, protected); err != nil {
		t.Fatal(err)
	}
	next := document
	next.Version = 2
	next.RevisionNo = 1
	next.ContentHash = contentHash
	next.GeneratedHash = knowledgedomain.HashContent(approved.ProposedFrontmatter, approved.ProposedBody)
	next.Status = knowledgedomain.DocumentActive
	revision := knowledgedomain.Revision{
		DocumentID: documentID, RevisionNo: 1, ProposalID: proposal.ID, Source: "proposal",
		PreviousHash: emptyHash, NewHash: contentHash, SnapshotObjectKey: snapshotKey,
		Frontmatter: approved.ProposedFrontmatter,
	}
	if _, err := repository.ApplyProposal(ctx, proposal.ID, approved.Version, next, revision); err != nil {
		t.Fatal(err)
	}
	return documentID, protected, manualSHA
}

func createM4SourceGenerationEvidence(
	t *testing.T, runtime *platformdatabase.Runtime, fixture derivedArtifactDocumentFixture,
) (ingestionapplication.SelectedSourceEvidenceDTO, ingestionapplication.ExtractSelectedSourceBodyResult) {
	t.Helper()
	payload := []byte(`<rssItem><guid>m4-recovery</guid><encoded>authorized normalized document body</encoded></rssItem>`)
	payloadSHA := fmt.Sprintf("%x", sha256.Sum256(payload))
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	policy := createDocumentRightsPolicy(t, runtime, fixture.sourceID, 90, now.Add(-time.Hour))
	snapshotKey := documentRightsFixtureDigest("m4-recovery-snapshot", fmt.Sprint(fixture.observationID))
	storeRawDecisionID := insertCitationRawRightsDecision(t, runtime, policy, snapshotKey, payloadSHA, "store_raw", nil)
	retentionDays := 30
	retainRawDecisionID := insertCitationRawRightsDecision(t, runtime, policy, snapshotKey, payloadSHA, "retain", &retentionDays)
	var snapshotID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO evidence_snapshots(
  source_connection_id,store_raw_rights_decision_id,retain_rights_decision_id,
  snapshot_key,object_key,payload_sha256,collector_profile_version,mime_type,size_bytes,
  response_status,requested_url,final_url,redirect_chain,response_headers,captured_at,retention_until,lifecycle_state,available_at
) VALUES ($1,$2,$3,$4,$5,$6,'m4-recovery-collector-v1','application/rss+xml',$7,
          200,'https://feed.example.test/m4-recovery.xml','https://feed.example.test/m4-recovery.xml',
          '[]'::jsonb,'{}'::jsonb,$8,$9,'raw_available',CURRENT_TIMESTAMP)
RETURNING id`, fixture.sourceID, storeRawDecisionID, retainRawDecisionID, snapshotKey,
		fmt.Sprintf("source-raw/v1/%d/%s/%s.raw", fixture.sourceID, snapshotKey[:2], snapshotKey), payloadSHA,
		len(payload), now, now.Add(30*24*time.Hour)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	var evidenceReferenceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_observation_evidences(
  source_connection_id,source_observation_id,evidence_snapshot_id,usage,locator_type,locator_value,
  selected_payload_sha256,selector_version
) VALUES ($1,$2,$3,'document_source','whole_payload','/',$4,'m4-recovery-selector-v1')
RETURNING id`, fixture.sourceID, fixture.observationID, snapshotID, payloadSHA).Scan(&evidenceReferenceID); err != nil {
		t.Fatal(err)
	}
	evidence := ingestionapplication.SelectedSourceEvidenceDTO{
		EvidenceReferenceID: evidenceReferenceID, SourceObservationID: fixture.observationID, EvidenceSnapshotID: snapshotID,
		SourceConnectionID: fixture.sourceID, ExternalWorkID: "derived-artifact-m4-projection-recovery",
		UpstreamIdentity: fmt.Sprintf("%064x", 97), SourceCode: "rss", ContentType: "article",
		Title: "M4 recovery fixture", Language: "en",
		SourceRecordURL: "https://feed.example.test/m4-recovery",
		CanonicalURL:    "https://publisher.example.test/articles/derived-artifact-m4-projection-recovery",
		BodyOrigin:      ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
		CapturedAt: fixture.persisted.DocumentVersion.CapturedAt, SelectedPayload: payload,
		SelectedPayloadSHA256: payloadSHA, PayloadMIMEType: "application/rss+xml", SelectorVersion: "m4-recovery-selector-v1",
	}
	extraction := ingestionapplication.ExtractSelectedSourceBodyResult{
		BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
		Plaintext: "authorized normalized document body", Markdown: "authorized normalized document body", Language: "en",
		ExtractorVersion: "rss-entry-v2", ExtractorProfileVersion: "rss-profile-v3",
		ExtractorProfileSHA256:            strings.Repeat("f", 64),
		PlaintextTransformerProfileSHA256: strings.Repeat("1", 64),
		MarkdownTransformerProfileSHA256:  strings.Repeat("2", 64),
		PlaintextSHA256:                   fixture.persisted.DocumentVersion.ContentSHA256,
	}
	addSourceGenerationAnchorFacts(&extraction)
	return evidence, extraction
}

func newM4DocumentVersionService(t *testing.T, runtime *platformdatabase.Runtime, fixture derivedArtifactDocumentFixture) *ingestionapplication.DocumentVersionService {
	t.Helper()
	reader := &integrationDocumentObservationReader{observations: map[int64]ingestionapplication.DocumentObservationDTO{
		fixture.observationID: {
			ID: fixture.observationID, SourceConnectionID: fixture.sourceID,
			ExternalWorkID: "derived-artifact-m4-projection-recovery",
			BodyOrigin:     ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: "authorized normalized document body", Language: "en", CapturedAt: fixture.persisted.DocumentVersion.CapturedAt,
		},
	}}
	service, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{
		Observations: reader, Versions: ingestionpostgres.NewDocumentVersionRepository(runtime),
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func newM4SourceDocumentGenerator(
	t *testing.T, runtime *platformdatabase.Runtime, vaultRoot string,
	documentVersions *ingestionapplication.DocumentVersionService,
	evidence ingestionapplication.SelectedSourceEvidenceDTO, extraction ingestionapplication.ExtractSelectedSourceBodyResult,
) *ingestionapplication.SourceDocumentGenerationService {
	t.Helper()
	artifactProjection := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, vaultRoot), documentVersions)
	recallWriter, err := ingestionpostgres.NewDocumentRecallProjectionWriter(runtime)
	if err != nil {
		t.Fatal(err)
	}
	recallProjection, err := ingestionapplication.NewDocumentRecallProjectionService(recallWriter)
	if err != nil {
		t.Fatal(err)
	}
	familyRepository, err := ingestionpostgres.NewContentFamilyRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	contentFamilies, err := ingestionapplication.NewContentFamilyService(familyRepository)
	if err != nil {
		t.Fatal(err)
	}
	generator, err := ingestionapplication.NewSourceDocumentGenerationService(ingestionapplication.SourceDocumentGenerationDependencies{
		Evidence: &sourceGenerationEvidenceReader{evidence: evidence}, Extractor: &sourceGenerationBodyExtractor{result: extraction},
		DocumentVersions: documentVersions,
		Authorizations:   ingestionpostgres.NewDocumentProjectionAuthorizationReader(runtime),
		Projections:      artifactProjection, SearchProjections: recallProjection,
		ContentFamilies: contentFamilies, StructureExtractor: ingestiontextstructure.NewExtractor(),
		DocumentEmbeddings: &sourceGenerationEmbeddingProducer{}, PublishedMatchEvaluations: &sourceGenerationMatchScheduler{},
		Now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return generator
}

func insertM4SearchContent(t *testing.T, runtime *platformdatabase.Runtime, fixture derivedArtifactDocumentFixture) int64 {
	t.Helper()
	publishedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	var contentID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents(source_connection_id,external_id,content_type,title,excerpt,canonical_url,language,published_at,fetched_at,dedupe_key)
VALUES ($1,'derived-artifact-m4-projection-recovery','article','M4 recovery fixture','authorized fixture summary',
        'https://publisher.example.test/articles/derived-artifact-m4-projection-recovery','en',$2,$2,$3)
RETURNING id`, fixture.sourceID, publishedAt, m4SHA256("m4-search-content")).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	return contentID
}

func insertM4ProjectionRecoveryNotificationFixture(t *testing.T, database *sql.DB) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var userID, monitorID int64
	if err := database.QueryRow(`INSERT INTO users(email,password_hash,display_name,role) VALUES ($1,'fixture','M4 recovery operator','viewer') RETURNING id`, fmt.Sprintf("m4-recovery-%d@example.test", now.UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`INSERT INTO monitors(name,status,created_by,updated_by) VALUES ($1,'draft',$2,$2) RETURNING id`, fmt.Sprintf("m4-recovery-monitor-%d", now.UnixNano()), userID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	notificationIDs := make([]int64, 0, 2)
	for index := 1; index <= 2; index++ {
		var outboxID, notificationID int64
		if err := database.QueryRow(`
INSERT INTO notification_outbox_events(event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,1,$2,$3,'M4 recovery event','bounded summary','urgent',$4,$5) RETURNING id`,
			index, monitorID, now, fmt.Sprintf("/dashboard/contents/%d", index), fmt.Sprintf("m4-recovery:%d:%d", now.UnixNano(), index)).Scan(&outboxID); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRow(`
INSERT INTO user_notifications(outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,occurred_at,title,summary,resource_status,deep_link)
VALUES ($1,$2,$3,'micro_event.updated','micro_event',$4,1,$5,'M4 recovery event','bounded summary','urgent',$6) RETURNING id`,
			outboxID, userID, monitorID, index, now, fmt.Sprintf("/dashboard/contents/%d", index)).Scan(&notificationID); err != nil {
			t.Fatal(err)
		}
		notificationIDs = append(notificationIDs, notificationID)
	}
	if _, err := database.Exec(`INSERT INTO notification_read_receipts(user_id,read_through_id) VALUES ($1,$2)`, userID, notificationIDs[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
INSERT INTO notification_delivery_attempts(user_notification_id,channel,delivery_target_key,attempt_no,status,dispatch_key,fencing_generation,provider_supports_idempotency,provider_supports_receipt_lookup,error_code,attempted_at)
VALUES ($1,'email','primary',1,'unknown',$2,1,false,false,'provider_outcome_unconfirmed',$3)`, notificationIDs[0], strings.Repeat("a", 64), now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
INSERT INTO notification_delivery_claims(user_notification_id,channel,delivery_target_key,claim_token,fencing_generation,dispatch_key,provider_supports_idempotency,provider_supports_receipt_lookup,claimed_at,lease_until)
VALUES ($1,'email','primary',$2,1,$3,false,false,$4::timestamptz,$4::timestamptz+interval '5 minutes')`, notificationIDs[1], strings.Repeat("b", 64), strings.Repeat("c", 64), now); err != nil {
		t.Fatal(err)
	}
}

func openM4ProjectionRecoveryMinIO(
	t *testing.T, ctx context.Context,
) (*knowledgeminio.Store, *knowledgeminio.Store, bool, func()) {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_SECRET_KEY"))
	baseBucket := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_BUCKET"))
	useSSL := strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_USE_SSL")), "true")
	if endpoint == "" || accessKey == "" || secretKey == "" || baseBucket == "" {
		t.Fatal("isolated MinIO test configuration is required")
	}
	randomBytes := make([]byte, 6)
	if _, err := rand.Read(randomBytes); err != nil {
		t.Fatal(err)
	}
	runID := hex.EncodeToString(randomBytes)
	sourceBucket := m4RecoveryBucket(baseBucket, "source", runID)
	targetBucket := m4RecoveryBucket(baseBucket, "target", runID)
	client, err := miniosdk.New(endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: useSSL,
		Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, bucket := range []string{sourceBucket, targetBucket} {
		if err := client.MakeBucket(ctx, bucket, miniosdk.MakeBucketOptions{Region: "us-east-1"}); err != nil {
			t.Fatalf("create isolated M4 MinIO bucket: %v", err)
		}
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, bucket := range []string{sourceBucket, targetBucket} {
			for object := range client.ListObjects(cleanupCtx, bucket, miniosdk.ListObjectsOptions{Recursive: true}) {
				if object.Err == nil {
					_ = client.RemoveObject(cleanupCtx, bucket, object.Key, miniosdk.RemoveObjectOptions{})
				}
			}
			_ = client.RemoveBucket(cleanupCtx, bucket)
		}
	}
	newStore := func(bucket string) *knowledgeminio.Store {
		store, err := knowledgeminio.NewStore(config.MinIOConfig{
			Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey, Bucket: bucket, UseSSL: useSSL,
		})
		if err != nil {
			cleanup()
			t.Fatal(err)
		}
		return store
	}
	return newStore(sourceBucket), newStore(targetBucket), sourceBucket != targetBucket, cleanup
}

func m4RecoveryBucket(base, role, runID string) string {
	normalized := strings.ToLower(strings.Trim(base, "-"))
	if len(normalized) > 30 {
		normalized = normalized[:30]
	}
	return fmt.Sprintf("%s-m4-%s-%s", normalized, role, runID)
}

func dumpM4ProjectionRecoveryDatabase(t *testing.T, ctx context.Context, dsn, path string) string {
	t.Helper()
	command := exec.CommandContext(ctx, "pg_dump", "--dbname="+dsn, "--format=custom", "--no-owner", "--no-acl",
		"--exclude-table-data=public.document_version_search_indexes", "--file="+path)
	if _, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create isolated M4 PostgreSQL backup: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func restoreM4ProjectionRecoveryDatabase(t *testing.T, ctx context.Context, dsn, path string) {
	t.Helper()
	render := exec.CommandContext(ctx, "pg_restore", "--no-owner", "--no-acl", "--file=-", path)
	var portableSQL, renderErrors bytes.Buffer
	render.Stdout = &portableSQL
	render.Stderr = &renderErrors
	if err := render.Run(); err != nil {
		t.Fatalf("render isolated M4 PostgreSQL restore: %v: %s", err, strings.TrimSpace(renderErrors.String()))
	}
	// Newer pg_dump clients emit this session setting even when the source
	// server and restore target predate PostgreSQL 17. It is not a schema fact,
	// so remove it to keep the recovery drill portable across supported client
	// and server minor/major combinations.
	restoreSQL := bytes.ReplaceAll(portableSQL.Bytes(), []byte("SET transaction_timeout = 0;\n"), nil)
	restore := exec.CommandContext(ctx, "psql", "--dbname="+dsn, "--set=ON_ERROR_STOP=1", "--quiet")
	restore.Stdin = bytes.NewReader(restoreSQL)
	if output, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("restore isolated M4 PostgreSQL copy: %v: %s", err, strings.TrimSpace(string(output)))
	}
}

func copyM4VaultTree(t *testing.T, sourceRoot, targetRoot string) {
	t.Helper()
	if err := filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil || relative == "." {
			return err
		}
		target := filepath.Join(targetRoot, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("source Vault copy contains a symlink")
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if !entry.Type().IsRegular() {
			return errors.New("source Vault copy contains a non-regular file")
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err = file.Write(payload); err == nil {
			err = file.Sync()
		}
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		return err
	}); err != nil {
		t.Fatalf("copy independent Vault tree: %v", err)
	}
}

func assertM4SourceCopyStayedIntact(
	t *testing.T, sourceRuntime *platformdatabase.Runtime, sourceVaultRoot, knowledgeRelativePath string, documentVersionID int64,
) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(sourceVaultRoot, knowledgeRelativePath)); err != nil {
		t.Fatalf("source Vault was changed while clearing restored copy: %v", err)
	}
	var sourceSearchRows int64
	if err := sourceRuntime.SQL.QueryRow(`SELECT count(*) FROM document_version_search_indexes WHERE document_version_id=$1`, documentVersionID).Scan(&sourceSearchRows); err != nil || sourceSearchRows != 1 {
		t.Fatalf("source search projection changed = %d/%v", sourceSearchRows, err)
	}
}

func clearM4ProjectionRecoveryEphemeralState(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	options, err := goredis.ParseURL(strings.TrimSpace(os.Getenv("HOTKEY_TEST_REDIS_URL")))
	if err != nil || options.DB != 15 {
		t.Fatalf("M4 recovery requires isolated Redis DB 15: %v", err)
	}
	client := goredis.NewClient(options)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.FlushDB(cleanupCtx).Err()
		_ = client.Close()
	})
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, "m4:session:fixture", "1", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, "m4:client-cache:fixture", "1", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	before, err := client.DBSize(ctx).Result()
	if err != nil || before != 2 {
		t.Fatalf("isolated Redis ephemeral keys = %d/%v, want 2", before, err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	after, err := client.DBSize(ctx).Result()
	if err != nil || after != 0 {
		t.Fatalf("isolated Redis cleanup = %d/%v, want 0", after, err)
	}
	return before
}

func readM4ProjectionRecoveryFactIDs(t *testing.T, database *sql.DB) m4ProjectionRecoveryFactIDs {
	t.Helper()
	return m4ProjectionRecoveryFactIDs{
		NotificationOutboxIDs: readM4IDs(t, database, "notification_outbox_events"),
		UserNotificationIDs:   readM4IDs(t, database, "user_notifications"),
		ReadReceiptIDs:        readM4IDs(t, database, "notification_read_receipts"),
		DeliveryAttemptIDs:    readM4IDs(t, database, "notification_delivery_attempts"),
	}
}

func readM4IDs(t *testing.T, database *sql.DB, table string) []int64 {
	t.Helper()
	allowed := map[string]bool{
		"notification_outbox_events": true, "user_notifications": true,
		"notification_read_receipts": true, "notification_delivery_attempts": true,
	}
	if !allowed[table] {
		t.Fatalf("invalid fact table %q", table)
	}
	rows, err := database.Query(`SELECT id FROM ` + table + ` ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func m4ProjectionRecoveryFacts(snapshot operationsapplication.ProjectionRecoveryFactSnapshotDTO) m4ProjectionRecoveryFactEvidence {
	return m4ProjectionRecoveryFactEvidence{
		NotificationOutboxCount: snapshot.NotificationOutboxCount,
		UserNotificationCount:   snapshot.UserNotificationCount,
		ReadReceiptCount:        snapshot.ReadReceiptCount,
		DeliveryAttemptCount:    snapshot.DeliveryAttemptCount,
		MaxUserNotificationID:   snapshot.MaxUserNotificationID,
		MaxReadReceiptID:        snapshot.MaxReadReceiptID,
		MaxDeliveryAttemptID:    snapshot.MaxDeliveryAttemptID,
		FingerprintSHA256:       snapshot.FingerprintSHA256,
	}
}

func writeM4ProjectionRecoveryEvidence(t *testing.T, path string, report m4ProjectionRecoveryReport) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create exclusive M4 recovery evidence: %v", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("write M4 recovery evidence: %v", err)
	}
}

func m4SHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
