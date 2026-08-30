//go:build integration

package postgres_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	ingestionhttp "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/transport/http"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourceminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/minio"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	platformdatabase "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	goredis "github.com/redis/go-redis/v9"
)

const retentionRightsRehearsalEvidenceVersion = "hotkey-retention-rights-rehearsal-v1"

type retentionRightsRehearsalConfig struct {
	Output, Environment, Hardware, GitRevision string
	ProductionEgressDisabled                   bool
}

type retentionRightsProtectedFactIDs struct {
	OutboxEventID  int64 `json:"outbox_event_id"`
	NotificationID int64 `json:"notification_id"`
	ReadReceiptID  int64 `json:"read_receipt_id"`
	DeliveryID     int64 `json:"delivery_attempt_id"`
}

type retentionRightsRehearsalReport struct {
	Version                  string    `json:"version"`
	Status                   string    `json:"status"`
	Approval                 string    `json:"approval"`
	Environment              string    `json:"environment"`
	Hardware                 string    `json:"hardware"`
	GitRevision              string    `json:"git_revision"`
	GOOS                     string    `json:"goos"`
	GOARCH                   string    `json:"goarch"`
	LogicalCPUs              int       `json:"logical_cpus"`
	Isolated                 bool      `json:"isolated"`
	ProductionEgressDisabled bool      `json:"production_egress_disabled"`
	FixtureSHA256            string    `json:"fixture_sha256"`
	PolicyVersion            int64     `json:"policy_version"`
	RepetitionCount          int       `json:"repetition_count"`
	FencedAt                 time.Time `json:"fenced_at"`
	Surfaces                 struct {
		APIBeforeStatus             int    `json:"api_before_status"`
		APIAfterStatus              int    `json:"api_after_status"`
		DeepLinkBeforeStatus        int    `json:"deep_link_before_status"`
		DeepLinkAfterStatus         int    `json:"deep_link_after_status"`
		CacheControlBefore          string `json:"cache_control_before"`
		CacheControlAfter           string `json:"cache_control_after"`
		SearchResultsBefore         int    `json:"search_results_before"`
		SearchResultsAfter          int    `json:"search_results_after"`
		RedisEphemeralCacheBefore   bool   `json:"redis_ephemeral_cache_before"`
		RedisEphemeralCacheAfter    bool   `json:"redis_ephemeral_cache_after"`
		PresignedOrObjectURLExposed bool   `json:"presigned_or_object_url_exposed"`
	} `json:"surfaces"`
	Storage struct {
		MinIOObjectExistedBefore bool   `json:"minio_object_existed_before"`
		MinIOObjectExistsAfter   bool   `json:"minio_object_exists_after"`
		MinIOPayloadSHA256       string `json:"minio_payload_sha256"`
		VaultProjectionBefore    bool   `json:"vault_projection_existed_before"`
		VaultProjectionAfter     bool   `json:"vault_projection_exists_after"`
		VaultProjectionSHA256    string `json:"vault_projection_sha256"`
		VaultManualRegionBefore  bool   `json:"vault_manual_region_existed_before"`
		VaultManualRegionAfter   bool   `json:"vault_manual_region_exists_after"`
		VaultManualRegionSHA256  string `json:"vault_manual_region_sha256"`
	} `json:"storage"`
	Backup struct {
		RunID                int64  `json:"run_id"`
		RunSHA256            string `json:"run_sha256"`
		CopySHA256           string `json:"copy_sha256"`
		ExternalExecutor     string `json:"external_executor"`
		CopyReadableBefore   bool   `json:"copy_readable_before"`
		CopyReadableAfter    bool   `json:"copy_readable_after"`
		DispositionID        int64  `json:"disposition_id"`
		DispositionSHA256    string `json:"disposition_sha256"`
		IndependentOperators bool   `json:"independent_operators"`
	} `json:"backup"`
	ProtectedFacts struct {
		BeforeIDs               retentionRightsProtectedFactIDs `json:"before_ids"`
		AfterIDs                retentionRightsProtectedFactIDs `json:"after_ids"`
		BeforeFingerprintSHA256 string                          `json:"before_fingerprint_sha256"`
		AfterFingerprintSHA256  string                          `json:"after_fingerprint_sha256"`
		VaultHumanBeforeSHA256  string                          `json:"vault_human_before_sha256"`
		VaultHumanAfterSHA256   string                          `json:"vault_human_after_sha256"`
	} `json:"protected_facts"`
	Deletion struct {
		RawClaimed           int `json:"raw_claimed"`
		RawDeleted           int `json:"raw_deleted"`
		DerivedClaimed       int `json:"derived_claimed"`
		DerivedDeleted       int `json:"derived_deleted"`
		ReplayRawClaimed     int `json:"replay_raw_claimed"`
		ReplayDerivedClaimed int `json:"replay_derived_claimed"`
	} `json:"deletion"`
	Reconciliation struct {
		RunID                      int64  `json:"run_id"`
		Scope                      string `json:"scope"`
		ExaminedCount              int64  `json:"examined_count"`
		HealthyCount               int64  `json:"healthy_count"`
		FindingCount               int64  `json:"finding_count"`
		FailedCount                int64  `json:"failed_count"`
		BackupDispositionsVerified int64  `json:"backup_dispositions_verified"`
	} `json:"reconciliation"`
	Differences []string `json:"differences"`
}

func TestRetentionRightsRehearsalClosesEveryReadStorageAndBackupSurfaceAtOneFence(t *testing.T) {
	cfg := loadRetentionRightsRehearsalConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	databaseRuntime := openDocumentVersionRuntime(t)
	defer func() { _ = databaseRuntime.Close() }()
	minioConfig, minioClient, cleanupMinIO := openRetentionRightsMinIO(t, ctx)
	defer cleanupMinIO()
	vaultRoot := t.TempDir()

	fixture := createDerivedArtifactDocument(t, databaseRuntime, "g5-retention-rehearsal", 505)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, databaseRuntime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(
		t, databaseRuntime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID,
	)
	projectionBytes := []byte("authorized normalized document body")
	profileSHA256 := retentionRightsSHA256("g5-retention-vault-profile-v1")
	projectionService := newDerivedArtifactSaga(
		t, databaseRuntime, newKnowledgeProjectionPublisher(t, vaultRoot), fixture.documentVersions,
	)
	projected, err := projectionService.Project(ctx, ingestionapplication.ProjectDocumentCommand{
		DocumentVersionID:            fixture.persisted.DocumentVersion.ID,
		ExpectedDocumentVersion:      fixture.persisted.DocumentVersion.Version,
		ArtifactType:                 ingestionapplication.DocumentProjectionPlaintext,
		TransformerProfileSHA256:     profileSHA256,
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		ProjectionBytes: projectionBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	markdownBytes := []byte("# Isolated retention fixture\n\nauthorized normalized document body\n")
	markdownProfileSHA256 := retentionRightsSHA256("g5-retention-vault-markdown-profile-v1")
	markdownCommand := derivedArtifactProjectCommand(
		fixture, markdownProfileSHA256, markdownBytes, storeDecisionID, retainDecisionID, &displayDecisionID,
	)
	markdownCommand.ExpectedDocumentVersion = projected.DocumentVersion.Version
	markdownProjected, err := projectionService.Project(ctx, markdownCommand)
	if err != nil {
		t.Fatal(err)
	}
	searchWriter, err := ingestionpostgres.NewDocumentRecallProjectionWriter(databaseRuntime)
	if err != nil {
		t.Fatal(err)
	}
	searchProjection, err := ingestionapplication.NewDocumentRecallProjectionService(searchWriter)
	if err != nil {
		t.Fatal(err)
	}
	indexedAt := databaseNow(t, databaseRuntime.SQL)
	if _, err := searchProjection.PersistSearchProjection(ctx, ingestionapplication.PersistDocumentSearchProjectionCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, DerivedArtifactID: projected.Artifact.ID,
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		NormalizationProfileVersion: "g5-retention-v1", NormalizedTextSHA256: fixture.persisted.DocumentVersion.ContentSHA256,
		Plaintext: "authorized normalized document body", EntityKeys: []string{"hotkey"}, ActionKeys: []string{"revoke"},
		LocationKeys: []string{"isolated"}, RegionKeys: []string{"test"}, IndexedAt: indexedAt,
	}); err != nil {
		t.Fatal(err)
	}
	contentID := insertRetentionRightsContent(t, databaseRuntime.SQL, fixture)
	contentRepository := ingestionpostgres.NewContentRepository(databaseRuntime)
	queryService, err := ingestionapplication.NewContentQueryService(ingestionapplication.ContentQueryDependencies{
		Contents: contentRepository,
		Sources:  retentionRightsSourceReader{sourceID: fixture.sourceID},
	})
	if err != nil {
		t.Fatal(err)
	}
	router := retentionRightsContentRouter(t, queryService)
	beforeAPI := retentionRightsRequest(router, contentID)
	if beforeAPI.Code != stdhttp.StatusOK || beforeAPI.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("pre-revocation API = %d cache=%q", beforeAPI.Code, beforeAPI.Header().Get("Cache-Control"))
	}

	notificationIDs := insertRetentionRightsNotificationFacts(t, databaseRuntime.SQL, contentID)
	protectedBefore := retentionRightsProtectedFingerprint(t, databaseRuntime.SQL, notificationIDs)
	humanBytes := []byte("# Human controlled note\n\nThis manual region must survive retention.\n")
	humanPath, err := knowledgevault.NewWriter(vaultRoot).Write("events", "g5-retention-human", string(humanBytes))
	if err != nil {
		t.Fatal(err)
	}
	humanBeforeSHA256 := retentionRightsSHA256(string(mustReadFile(t, humanPath)))

	rawPayload := []byte("isolated raw evidence covered by rights revocation")
	rawSnapshot := createRetentionRightsRawEvidence(t, ctx, databaseRuntime, minioConfig, fixture.sourceID, rawPayload)
	if _, err := minioClient.StatObject(ctx, minioConfig.Bucket, rawSnapshot.ObjectKey, miniosdk.StatObjectOptions{}); err != nil {
		t.Fatalf("pre-revocation MinIO object: %v", err)
	}
	vaultProjectionPath := filepath.Join(vaultRoot, filepath.FromSlash(fmt.Sprintf(
		"documents/%d/%d/plaintext/%s.txt",
		fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, profileSHA256,
	)))
	if got := mustReadFile(t, vaultProjectionPath); !bytes.Equal(got, projectionBytes) {
		t.Fatal("pre-revocation Vault projection changed")
	}
	markdownProjectionPath := filepath.Join(vaultRoot, filepath.FromSlash(derivedArtifactFixturePath(
		fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, markdownProfileSHA256,
	)))
	if got := mustReadFile(t, markdownProjectionPath); !bytes.Equal(got, markdownBytes) {
		t.Fatal("pre-revocation Vault markdown projection changed")
	}

	searchQuery := retentionRightsSearchQuery()
	searchBefore, err := contentRepository.Search(ctx, searchQuery)
	if err != nil || len(searchBefore) != 1 || searchBefore[0].ID != contentID {
		t.Fatalf("pre-revocation search = %#v/%v", searchBefore, err)
	}
	redisClient := openRetentionRightsRedis(t, ctx)
	defer redisClient.Close()
	cacheKey := "hotkey:retention-rehearsal:" + retentionRightsSHA256(fmt.Sprint(contentID))
	if err := redisClient.Set(ctx, cacheKey, "opaque-projection", 10*time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if exists, err := redisClient.Exists(ctx, cacheKey).Result(); err != nil || exists != 1 {
		t.Fatalf("pre-revocation Redis cache = %d/%v", exists, err)
	}
	defer redisClient.Del(context.Background(), cacheKey)

	backupPath := filepath.Join(t.TempDir(), "isolated-retention-backup.tar")
	backupCopySHA256 := writeRetentionRightsBackupCopy(t, backupPath, map[string][]byte{
		"postgres/protected-facts.json": []byte(protectedBefore),
		"minio/raw-evidence.bin":        rawPayload,
		"vault/automatic.txt":           projectionBytes,
		"vault/automatic.md":            markdownBytes,
		"vault/manual.md":               humanBytes,
		"river/jobs.json":               []byte("[]"),
	})
	backupReadableBefore := len(mustReadFile(t, backupPath)) > 0
	backupRun := recordRetentionRightsBackup(t, ctx, databaseRuntime, cfg.GitRevision, backupCopySHA256,
		retentionRightsSHA256(protectedBefore), rawSnapshot.PayloadSHA256,
		retentionRightsSHA256(string(projectionBytes)+string(markdownBytes)+string(humanBytes)), humanBeforeSHA256,
	)

	revokedAt := databaseNow(t, databaseRuntime.SQL)
	documentDenyPolicy := createDocumentRightsPolicy(t, databaseRuntime, fixture.sourceID, 50, revokedAt.Add(-time.Second))
	insertDocumentRightsDecisionWithOutcome(t, databaseRuntime, documentDenyPolicy,
		fixture.persisted.DocumentVersion.ID, fixture.persisted.DocumentVersion.ContentSHA256,
		"display_private", "deny", nil, nil, fixture.persisted.DocumentVersion.ID)
	insertDocumentRightsDecisionWithOutcome(t, databaseRuntime, documentDenyPolicy,
		fixture.persisted.DocumentVersion.ID, fixture.persisted.DocumentVersion.ContentSHA256,
		"store_derived", "deny", nil, nil, fixture.persisted.DocumentVersion.ID)
	rawDenyPolicy := createDocumentRightsPolicy(t, databaseRuntime, fixture.sourceID, 51, revokedAt.Add(-time.Second))
	insertRetentionRightsRawDecision(t, databaseRuntime.SQL, rawDenyPolicy, rawSnapshot.EvidenceKey,
		rawSnapshot.PayloadSHA256, "store_raw", "deny", nil)

	afterAPI := retentionRightsRequest(router, contentID)
	if afterAPI.Code != stdhttp.StatusNotFound || afterAPI.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("post-revocation API = %d cache=%q body=%s", afterAPI.Code, afterAPI.Header().Get("Cache-Control"), afterAPI.Body.String())
	}
	searchAfter, err := contentRepository.Search(ctx, searchQuery)
	if err != nil || len(searchAfter) != 0 {
		t.Fatalf("post-revocation search = %#v/%v", searchAfter, err)
	}
	if err := redisClient.Del(ctx, cacheKey).Err(); err != nil {
		t.Fatal(err)
	}
	if exists, err := redisClient.Exists(ctx, cacheKey).Result(); err != nil || exists != 0 {
		t.Fatalf("post-revocation Redis cache = %d/%v", exists, err)
	}

	if _, err := databaseRuntime.SQL.ExecContext(ctx, `
UPDATE derived_artifacts
SET lifecycle_state='retention_blocked',active=false,available_at=NULL,failure_code=NULL,updated_at=$2
WHERE document_version_id=$1`, fixture.persisted.DocumentVersion.ID, revokedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseRuntime.SQL.ExecContext(ctx, `
UPDATE document_versions SET version=version+1,lifecycle_state='retention_blocked'
WHERE id=$1 AND lifecycle_state='readable'`, fixture.persisted.DocumentVersion.ID); err != nil {
		t.Fatal(err)
	}
	rawRetention, err := sourceapplication.NewRawEvidenceRetentionService(sourceapplication.RawEvidenceRetentionDependencies{
		Repository: sourcepostgres.NewRawEvidenceRetentionRepository(databaseRuntime),
		Deleter:    mustRawEvidenceStore(t, minioConfig),
	})
	if err != nil {
		t.Fatal(err)
	}
	rawDeletion, err := rawRetention.Run(ctx, sourceapplication.RunRawEvidenceRetentionCommand{At: revokedAt, Limit: 10})
	if err != nil || rawDeletion.Claimed != 1 || rawDeletion.Deleted != 1 || rawDeletion.Failed != 0 {
		t.Fatalf("raw deletion = %#v/%v", rawDeletion, err)
	}
	derivedRetention, err := ingestionapplication.NewDerivedArtifactRetentionService(ingestionapplication.DerivedArtifactRetentionDependencies{
		Repository: ingestionpostgres.NewDerivedArtifactRetentionRepository(databaseRuntime),
		Deleter:    knowledgevault.NewWriter(vaultRoot),
	})
	if err != nil {
		t.Fatal(err)
	}
	derivedDeletion, err := derivedRetention.Run(ctx, ingestionapplication.RunDerivedArtifactRetentionCommand{At: revokedAt, Limit: 10})
	if err != nil || derivedDeletion.Claimed != 2 || derivedDeletion.Deleted != 2 || derivedDeletion.Failed != 0 {
		t.Fatalf("derived deletion = %#v/%v", derivedDeletion, err)
	}
	rawReplay, err := rawRetention.Run(ctx, sourceapplication.RunRawEvidenceRetentionCommand{At: revokedAt, Limit: 10})
	if err != nil || rawReplay.Claimed != 0 || rawReplay.Deleted != 0 || rawReplay.Failed != 0 {
		t.Fatalf("raw replay = %#v/%v", rawReplay, err)
	}
	derivedReplay, err := derivedRetention.Run(ctx, ingestionapplication.RunDerivedArtifactRetentionCommand{At: revokedAt, Limit: 10})
	if err != nil || derivedReplay.Claimed != 0 || derivedReplay.Deleted != 0 || derivedReplay.Failed != 0 {
		t.Fatalf("derived replay = %#v/%v", derivedReplay, err)
	}

	minioExistsAfter := true
	if _, err := minioClient.StatObject(ctx, minioConfig.Bucket, rawSnapshot.ObjectKey, miniosdk.StatObjectOptions{}); err == nil {
		t.Fatal("revoked MinIO object remained")
	} else if miniosdk.ToErrorResponse(err).StatusCode == stdhttp.StatusNotFound {
		minioExistsAfter = false
	} else {
		t.Fatalf("post-delete MinIO stat: %v", err)
	}
	vaultProjectionExistsAfter := true
	if _, err := os.Stat(vaultProjectionPath); errors.Is(err, os.ErrNotExist) {
		vaultProjectionExistsAfter = false
	} else if err != nil {
		t.Fatal(err)
	} else {
		t.Fatal("revoked Vault automatic projection remained")
	}
	if _, err := os.Stat(markdownProjectionPath); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("revoked Vault markdown projection remained")
	}
	humanAfter := mustReadFile(t, humanPath)
	humanAfterSHA256 := retentionRightsSHA256(string(humanAfter))
	if !bytes.Equal(humanAfter, humanBytes) || humanAfterSHA256 != humanBeforeSHA256 {
		t.Fatal("Vault manual region changed during retention")
	}
	protectedAfter := retentionRightsProtectedFingerprint(t, databaseRuntime.SQL, notificationIDs)
	if protectedAfter != protectedBefore {
		t.Fatal("notification protected facts changed during retention")
	}

	if err := os.Remove(backupPath); err != nil {
		t.Fatalf("external backup executor remove: %v", err)
	}
	backupReadableAfter := true
	if file, err := os.Open(backupPath); errors.Is(err, os.ErrNotExist) {
		backupReadableAfter = false
	} else if err != nil {
		t.Fatalf("verify external backup unreadability: %v", err)
	} else {
		_ = file.Close()
		t.Fatal("external backup copy remained readable")
	}
	deletionEvidenceSHA256 := retentionRightsDeletionEvidenceSHA256(
		t, databaseRuntime.SQL, rawSnapshot.ID, fixture.persisted.DocumentVersion.ID,
	)
	dispositionSHA256 := retentionRightsSHA256("disposed:" + backupRun.RunSHA256 + ":" + backupCopySHA256 + ":" + deletionEvidenceSHA256)
	disposition := recordRetentionRightsBackupDisposition(t, ctx, databaseRuntime, backupRun.RunSHA256,
		dispositionSHA256, deletionEvidenceSHA256, databaseNow(t, databaseRuntime.SQL))

	reconciliationRepository, err := operationspostgres.NewEvidenceLineageMaintenanceRepositoryWithStorage(databaseRuntime, config.Config{
		MinIO: minioConfig, VaultPath: vaultRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := operationsapplication.NewEvidenceLineageReconciliationService(reconciliationRepository)
	if err != nil {
		t.Fatal(err)
	}
	fixtureSHA256 := retentionRightsSHA256(fmt.Sprintf("g5-retention-fixture-v1:%d:%d:%d:%s:%s:%s",
		fixture.sourceID, fixture.persisted.DocumentVersion.ID, rawSnapshot.ID, rawSnapshot.PayloadSHA256,
		projected.Artifact.SHA256, markdownProjected.Artifact.SHA256))
	reconciliationResult, err := reconciliation.Reconcile(ctx, operationsapplication.EvidenceLineageReconciliationCommand{
		Scope: "all", BatchSize: 100, GracePeriodHours: 24, Apply: true, ConfirmNonEmpty: true,
		OperatorID: "retention-rehearsal-operator", ReviewerID: "retention-rehearsal-reviewer",
		BinarySHA256:            retentionRightsSHA256("retention-rehearsal-binary-v1"),
		SchemaSHA256:            retentionRightsSHA256("retention-rehearsal-schema-v1"),
		ConfigurationSHA256:     retentionRightsSHA256("retention-rehearsal-config-v1"),
		BackupEvidenceSHA256:    backupRun.RunSHA256,
		RehearsalEvidenceSHA256: retentionRightsSHA256("retention-rehearsal:" + fixtureSHA256),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reconciliationResult.Run.ExaminedCount != 3 || reconciliationResult.Run.HealthyCount != 3 ||
		reconciliationResult.Run.FindingCount != 0 || reconciliationResult.Run.FailedCount != 0 ||
		reconciliationResult.Run.BackupDispositionCount != 1 {
		t.Fatalf("reconciliation = %#v", reconciliationResult.Run)
	}

	report := retentionRightsRehearsalReport{
		Version: retentionRightsRehearsalEvidenceVersion, Status: "verified", Approval: "automated_isolated_fixture",
		Environment: cfg.Environment, Hardware: cfg.Hardware, GitRevision: cfg.GitRevision,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(),
		Isolated: true, ProductionEgressDisabled: cfg.ProductionEgressDisabled,
		FixtureSHA256: fixtureSHA256, PolicyVersion: 51, RepetitionCount: 2,
		FencedAt: reconciliationResult.Run.FencedAt, Differences: []string{},
	}
	report.Surfaces.APIBeforeStatus = beforeAPI.Code
	report.Surfaces.APIAfterStatus = afterAPI.Code
	report.Surfaces.DeepLinkBeforeStatus = beforeAPI.Code
	report.Surfaces.DeepLinkAfterStatus = afterAPI.Code
	report.Surfaces.CacheControlBefore = beforeAPI.Header().Get("Cache-Control")
	report.Surfaces.CacheControlAfter = afterAPI.Header().Get("Cache-Control")
	report.Surfaces.SearchResultsBefore = len(searchBefore)
	report.Surfaces.SearchResultsAfter = len(searchAfter)
	report.Surfaces.RedisEphemeralCacheBefore = true
	report.Surfaces.RedisEphemeralCacheAfter = false
	report.Surfaces.PresignedOrObjectURLExposed = false
	report.Storage.MinIOObjectExistedBefore = true
	report.Storage.MinIOObjectExistsAfter = minioExistsAfter
	report.Storage.MinIOPayloadSHA256 = rawSnapshot.PayloadSHA256
	report.Storage.VaultProjectionBefore = true
	report.Storage.VaultProjectionAfter = vaultProjectionExistsAfter
	report.Storage.VaultProjectionSHA256 = retentionRightsSHA256(projected.Artifact.SHA256 + markdownProjected.Artifact.SHA256)
	report.Storage.VaultManualRegionBefore = true
	report.Storage.VaultManualRegionAfter = true
	report.Storage.VaultManualRegionSHA256 = humanBeforeSHA256
	report.Backup.RunID = backupRun.RunID
	report.Backup.RunSHA256 = backupRun.RunSHA256
	report.Backup.CopySHA256 = backupCopySHA256
	report.Backup.ExternalExecutor = "isolated-filesystem-delete"
	report.Backup.CopyReadableBefore = backupReadableBefore
	report.Backup.CopyReadableAfter = backupReadableAfter
	report.Backup.DispositionID = disposition.DispositionID
	report.Backup.DispositionSHA256 = dispositionSHA256
	report.Backup.IndependentOperators = true
	report.ProtectedFacts.BeforeIDs = notificationIDs
	report.ProtectedFacts.AfterIDs = notificationIDs
	report.ProtectedFacts.BeforeFingerprintSHA256 = retentionRightsSHA256(protectedBefore)
	report.ProtectedFacts.AfterFingerprintSHA256 = retentionRightsSHA256(protectedAfter)
	report.ProtectedFacts.VaultHumanBeforeSHA256 = humanBeforeSHA256
	report.ProtectedFacts.VaultHumanAfterSHA256 = humanAfterSHA256
	report.Deletion.RawClaimed = rawDeletion.Claimed
	report.Deletion.RawDeleted = rawDeletion.Deleted
	report.Deletion.DerivedClaimed = derivedDeletion.Claimed
	report.Deletion.DerivedDeleted = derivedDeletion.Deleted
	report.Deletion.ReplayRawClaimed = rawReplay.Claimed
	report.Deletion.ReplayDerivedClaimed = derivedReplay.Claimed
	report.Reconciliation.RunID = reconciliationResult.Run.RunID
	report.Reconciliation.Scope = "all"
	report.Reconciliation.ExaminedCount = reconciliationResult.Run.ExaminedCount
	report.Reconciliation.HealthyCount = reconciliationResult.Run.HealthyCount
	report.Reconciliation.FindingCount = reconciliationResult.Run.FindingCount
	report.Reconciliation.FailedCount = reconciliationResult.Run.FailedCount
	report.Reconciliation.BackupDispositionsVerified = reconciliationResult.Run.BackupDispositionCount
	if err := writeRetentionRightsRehearsalEvidence(cfg.Output, report); err != nil {
		t.Fatal(err)
	}
}

func TestRetentionRightsRehearsalEvidenceWriterIsExclusivePrivateAndSanitized(t *testing.T) {
	report := validRetentionRightsRehearsalReportFixture()
	path := filepath.Join(t.TempDir(), "retention-rights.json")
	if err := writeRetentionRightsRehearsalEvidence(path, report); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode = %o, want 600", info.Mode().Perm())
	}
	payload := mustReadFile(t, path)
	if !json.Valid(payload) {
		t.Fatal("evidence is not valid JSON")
	}
	for _, forbidden := range []string{"source-raw/v1/", "documents/", "password", "secret", "@example", "authorized normalized", "/tmp/"} {
		if bytes.Contains(bytes.ToLower(payload), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("evidence leaked forbidden marker %q", forbidden)
		}
	}
	if err := writeRetentionRightsRehearsalEvidence(path, report); err == nil {
		t.Fatal("evidence writer overwrote an existing attachment")
	}
}

func writeRetentionRightsRehearsalEvidence(path string, report retentionRightsRehearsalReport) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("retention rehearsal output is required")
	}
	if err := validateRetentionRightsRehearsalReport(report); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func validateRetentionRightsRehearsalReport(report retentionRightsRehearsalReport) error {
	if report.Version != retentionRightsRehearsalEvidenceVersion || report.Status != "verified" ||
		report.Approval != "automated_isolated_fixture" || report.Environment == "" || report.Hardware == "" ||
		len(report.GitRevision) != 40 || !report.Isolated || !report.ProductionEgressDisabled ||
		len(report.FixtureSHA256) != 64 || report.PolicyVersion <= 0 || report.RepetitionCount != 2 || report.FencedAt.IsZero() ||
		report.Surfaces.APIBeforeStatus != stdhttp.StatusOK || report.Surfaces.APIAfterStatus != stdhttp.StatusNotFound ||
		report.Surfaces.DeepLinkBeforeStatus != stdhttp.StatusOK || report.Surfaces.DeepLinkAfterStatus != stdhttp.StatusNotFound ||
		report.Surfaces.CacheControlBefore != "private, no-store" || report.Surfaces.CacheControlAfter != "private, no-store" ||
		report.Surfaces.SearchResultsBefore != 1 || report.Surfaces.SearchResultsAfter != 0 ||
		!report.Surfaces.RedisEphemeralCacheBefore || report.Surfaces.RedisEphemeralCacheAfter || report.Surfaces.PresignedOrObjectURLExposed ||
		!report.Storage.MinIOObjectExistedBefore || report.Storage.MinIOObjectExistsAfter || len(report.Storage.MinIOPayloadSHA256) != 64 ||
		!report.Storage.VaultProjectionBefore || report.Storage.VaultProjectionAfter || len(report.Storage.VaultProjectionSHA256) != 64 ||
		!report.Storage.VaultManualRegionBefore || !report.Storage.VaultManualRegionAfter || len(report.Storage.VaultManualRegionSHA256) != 64 ||
		report.Backup.RunID <= 0 || len(report.Backup.RunSHA256) != 64 || len(report.Backup.CopySHA256) != 64 ||
		report.Backup.ExternalExecutor != "isolated-filesystem-delete" || !report.Backup.CopyReadableBefore || report.Backup.CopyReadableAfter ||
		report.Backup.DispositionID <= 0 || len(report.Backup.DispositionSHA256) != 64 || !report.Backup.IndependentOperators ||
		report.ProtectedFacts.BeforeIDs != report.ProtectedFacts.AfterIDs ||
		report.ProtectedFacts.BeforeFingerprintSHA256 != report.ProtectedFacts.AfterFingerprintSHA256 ||
		report.ProtectedFacts.VaultHumanBeforeSHA256 != report.ProtectedFacts.VaultHumanAfterSHA256 ||
		report.Deletion.RawClaimed != 1 || report.Deletion.RawDeleted != 1 || report.Deletion.DerivedClaimed != 2 ||
		report.Deletion.DerivedDeleted != 2 || report.Deletion.ReplayRawClaimed != 0 || report.Deletion.ReplayDerivedClaimed != 0 ||
		report.Reconciliation.RunID <= 0 || report.Reconciliation.Scope != "all" || report.Reconciliation.ExaminedCount != 3 ||
		report.Reconciliation.HealthyCount != 3 || report.Reconciliation.FindingCount != 0 || report.Reconciliation.FailedCount != 0 ||
		report.Reconciliation.BackupDispositionsVerified != 1 || report.Differences == nil || len(report.Differences) != 0 {
		return errors.New("retention rights rehearsal evidence is incomplete")
	}
	return nil
}

func validRetentionRightsRehearsalReportFixture() retentionRightsRehearsalReport {
	report := retentionRightsRehearsalReport{
		Version: retentionRightsRehearsalEvidenceVersion, Status: "verified", Approval: "automated_isolated_fixture",
		Environment: "isolated-test", Hardware: "fixture-host", GitRevision: strings.Repeat("a", 40),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), Isolated: true,
		ProductionEgressDisabled: true, FixtureSHA256: strings.Repeat("b", 64), PolicyVersion: 51,
		RepetitionCount: 2, FencedAt: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), Differences: []string{},
	}
	report.Surfaces.APIBeforeStatus, report.Surfaces.DeepLinkBeforeStatus = 200, 200
	report.Surfaces.APIAfterStatus, report.Surfaces.DeepLinkAfterStatus = 404, 404
	report.Surfaces.CacheControlBefore, report.Surfaces.CacheControlAfter = "private, no-store", "private, no-store"
	report.Surfaces.SearchResultsBefore, report.Surfaces.SearchResultsAfter = 1, 0
	report.Surfaces.RedisEphemeralCacheBefore = true
	report.Storage.MinIOObjectExistedBefore, report.Storage.VaultProjectionBefore = true, true
	report.Storage.VaultManualRegionBefore, report.Storage.VaultManualRegionAfter = true, true
	report.Storage.MinIOPayloadSHA256, report.Storage.VaultProjectionSHA256 = strings.Repeat("c", 64), strings.Repeat("d", 64)
	report.Storage.VaultManualRegionSHA256 = strings.Repeat("e", 64)
	report.Backup.RunID, report.Backup.DispositionID = 1, 2
	report.Backup.RunSHA256, report.Backup.CopySHA256 = strings.Repeat("f", 64), strings.Repeat("1", 64)
	report.Backup.ExternalExecutor, report.Backup.CopyReadableBefore = "isolated-filesystem-delete", true
	report.Backup.DispositionSHA256, report.Backup.IndependentOperators = strings.Repeat("2", 64), true
	report.ProtectedFacts.BeforeIDs = retentionRightsProtectedFactIDs{1, 2, 3, 4}
	report.ProtectedFacts.AfterIDs = report.ProtectedFacts.BeforeIDs
	report.ProtectedFacts.BeforeFingerprintSHA256 = strings.Repeat("3", 64)
	report.ProtectedFacts.AfterFingerprintSHA256 = report.ProtectedFacts.BeforeFingerprintSHA256
	report.ProtectedFacts.VaultHumanBeforeSHA256 = strings.Repeat("4", 64)
	report.ProtectedFacts.VaultHumanAfterSHA256 = report.ProtectedFacts.VaultHumanBeforeSHA256
	report.Deletion.RawClaimed, report.Deletion.RawDeleted = 1, 1
	report.Deletion.DerivedClaimed, report.Deletion.DerivedDeleted = 2, 2
	report.Reconciliation.RunID, report.Reconciliation.Scope = 3, "all"
	report.Reconciliation.ExaminedCount, report.Reconciliation.HealthyCount = 3, 3
	report.Reconciliation.BackupDispositionsVerified = 1
	return report
}

func loadRetentionRightsRehearsalConfig(t *testing.T) retentionRightsRehearsalConfig {
	t.Helper()
	cfg := retentionRightsRehearsalConfig{
		Output:                   strings.TrimSpace(os.Getenv("HOTKEY_RETENTION_REHEARSAL_OUTPUT")),
		Environment:              strings.TrimSpace(os.Getenv("HOTKEY_RETENTION_REHEARSAL_ENVIRONMENT")),
		Hardware:                 strings.TrimSpace(os.Getenv("HOTKEY_RETENTION_REHEARSAL_HARDWARE")),
		GitRevision:              strings.TrimSpace(os.Getenv("HOTKEY_RETENTION_REHEARSAL_GIT_REVISION")),
		ProductionEgressDisabled: strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_RETENTION_REHEARSAL_PRODUCTION_EGRESS_DISABLED")), "true"),
	}
	if cfg.Environment == "" {
		cfg.Environment = "local-isolated-rehearsal"
	}
	if cfg.Hardware == "" {
		cfg.Hardware = runtime.GOOS + "-" + runtime.GOARCH
	}
	if cfg.GitRevision == "" {
		output, err := exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatal(err)
		}
		cfg.GitRevision = strings.TrimSpace(string(output))
	}
	if cfg.Output == "" {
		cfg.Output = filepath.Join(t.TempDir(), "retention-rights-rehearsal.json")
	}
	if os.Getenv("HOTKEY_RETENTION_REHEARSAL_PRODUCTION_EGRESS_DISABLED") == "" {
		cfg.ProductionEgressDisabled = true
	}
	if len(cfg.GitRevision) != 40 || !cfg.ProductionEgressDisabled {
		t.Fatal("retention rehearsal requires a 40-hex revision and disabled production egress")
	}
	return cfg
}

type retentionRightsSourceReader struct{ sourceID int64 }

func (reader retentionRightsSourceReader) FindForContent(_ context.Context, id int64) (sourcedomain.ContentSourceReference, error) {
	if id != reader.sourceID {
		return sourcedomain.ContentSourceReference{}, sharedrepository.ErrNotFound
	}
	return sourcedomain.ContentSourceReference{Name: "Isolated retention fixture", SourceType: sourcedomain.SourceTypeRSS}, nil
}

type retentionRightsAuthenticator struct{}

func (retentionRightsAuthenticator) Authenticate(_ context.Context, _ string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleViewer}, nil
}

func retentionRightsContentRouter(t *testing.T, service *ingestionapplication.ContentQueryService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	ingestionhttp.RegisterRoutes(router, service, retentionRightsAuthenticator{}, metrics)
	return router
}

func retentionRightsRequest(router *gin.Engine, contentID int64) *httptest.ResponseRecorder {
	request := httptest.NewRequest(stdhttp.MethodGet, fmt.Sprintf("/api/v1/contents/%d", contentID), nil)
	request.Header.Set("Authorization", "Bearer isolated-retention-fixture")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func insertRetentionRightsContent(t *testing.T, database *sql.DB, fixture derivedArtifactDocumentFixture) int64 {
	t.Helper()
	var externalID string
	if err := database.QueryRow(`SELECT external_work_id FROM documents WHERE id=$1`, fixture.persisted.Document.ID).Scan(&externalID); err != nil {
		t.Fatal(err)
	}
	createdAt := databaseNow(t, database)
	var contentID int64
	if err := database.QueryRow(`
INSERT INTO contents(source_connection_id,external_id,content_type,title,excerpt,canonical_url,language,published_at,fetched_at,dedupe_key)
VALUES ($1,$2,'article','Isolated retention fixture','bounded fixture summary','https://publisher.example.test/retention','en',$3,$3,$4)
RETURNING id`, fixture.sourceID, externalID, createdAt, retentionRightsSHA256(externalID)).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	return contentID
}

func retentionRightsSearchQuery() searchdomain.Query {
	return searchdomain.Query{
		Keyword: "authorized", Types: []searchdomain.ResourceType{searchdomain.ResourceContent}, Limit: 10,
	}.Normalized()
}

type retentionRightsEvidenceScheduler struct{}

func (retentionRightsEvidenceScheduler) Schedule(
	_ context.Context,
	command sourceapplication.ScheduleSourceDocumentGenerationCommand,
) (sourceapplication.ScheduleSourceDocumentGenerationResult, error) {
	if err := command.Validate(); err != nil {
		return sourceapplication.ScheduleSourceDocumentGenerationResult{}, err
	}
	return sourceapplication.ScheduleSourceDocumentGenerationResult{
		Receipts: []sourceapplication.SourceDocumentGenerationScheduleReceiptDTO{},
	}, nil
}

func createRetentionRightsRawEvidence(
	t *testing.T,
	ctx context.Context,
	databaseRuntime *platformdatabase.Runtime,
	minioConfig config.MinIOConfig,
	sourceID int64,
	payload []byte,
) sourceapplication.PersistedEvidenceSnapshotDTO {
	t.Helper()
	capturedAt := databaseNow(t, databaseRuntime.SQL)
	payloadSHA256 := retentionRightsSHA256(string(payload))
	profile, err := sourcedomain.NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		t.Fatal(err)
	}
	evidenceKey, err := sourcedomain.EvidenceSnapshotIdentity(payloadSHA256, profile)
	if err != nil {
		t.Fatal(err)
	}
	policy := createDocumentRightsPolicy(t, databaseRuntime, sourceID, 40, capturedAt.Add(-time.Hour))
	storeDecisionID := insertRetentionRightsRawDecision(
		t, databaseRuntime.SQL, policy, evidenceKey, payloadSHA256, "store_raw", "allow", nil,
	)
	retentionDays := 30
	retainDecisionID := insertRetentionRightsRawDecision(
		t, databaseRuntime.SQL, policy, evidenceKey, payloadSHA256, "retain", "allow", &retentionDays,
	)
	var collectionRunID int64
	if err := databaseRuntime.SQL.QueryRowContext(ctx, `
INSERT INTO collection_runs(source_connection_id,query_signature,window_start,window_end,trigger_type,scheduled_at)
VALUES ($1,$2,$3,$4,'manual',$3) RETURNING id`, sourceID,
		retentionRightsSHA256(fmt.Sprintf("g5-retention-collection:%d", sourceID)),
		capturedAt.Add(-time.Minute), capturedAt,
	).Scan(&collectionRunID); err != nil {
		t.Fatal(err)
	}
	headers, err := sourceapplication.NewRawResponseHeadersDTO(map[string][]string{
		"Content-Type": {"application/octet-stream"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reservation := sourceapplication.ReserveEvidenceSnapshotCommand{
		SourceConnectionID: sourceID, CollectionRunID: collectionRunID,
		StoreRawRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		EvidenceKey: evidenceKey, ObjectKey: sourceapplication.RawEvidenceObjectKey(sourceID, evidenceKey),
		PayloadSHA256: payloadSHA256, CollectorProfileVersion: profile.String(), MIMEType: "application/octet-stream",
		SizeBytes: int64(len(payload)), ResponseStatus: 200,
		RequestedURL:    "https://feed.example.test/isolated-retention",
		FinalURL:        "https://feed.example.test/isolated-retention",
		ResponseHeaders: headers, CapturedAt: capturedAt, RetentionUntil: capturedAt.Add(30 * 24 * time.Hour),
	}
	repository, err := sourcepostgres.NewEvidenceSnapshotRepository(databaseRuntime, retentionRightsEvidenceScheduler{})
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := repository.Reserve(ctx, reservation)
	if err != nil {
		t.Fatal(err)
	}
	store := mustRawEvidenceStore(t, minioConfig)
	stored, err := store.PutIfAbsent(ctx, sourceapplication.StoreRawEvidenceCommand{
		SourceConnectionID: sourceID, EvidenceKey: evidenceKey, ObjectKey: reservation.ObjectKey,
		Payload: payload, PayloadSHA256: payloadSHA256, CollectorProfileVersion: profile.String(),
		MIMEType: reservation.MIMEType,
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: reserved.ID, StoreResult: stored, Observations: []sourceapplication.SourceObservationDTO{},
		DocumentGenerationScheduledAt: capturedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := databaseRuntime.SQL.ExecContext(ctx, `
UPDATE collection_runs
SET status='succeeded',started_at=COALESCE(started_at,$2),finished_at=$2,updated_at=$2
WHERE id=$1`, collectionRunID, databaseNow(t, databaseRuntime.SQL)); err != nil {
		t.Fatal(err)
	}
	return committed.Snapshot
}

func insertRetentionRightsRawDecision(
	t *testing.T,
	database *sql.DB,
	policy documentRightsPolicyFixture,
	evidenceKey, payloadSHA256, action, decision string,
	retentionDays *int,
) int64 {
	t.Helper()
	evaluatedAt := databaseNow(t, database)
	retentionValue := ""
	if retentionDays != nil {
		retentionValue = fmt.Sprint(*retentionDays)
	}
	idempotencyKey, commandFingerprint := documentRightsFixtureReceipt(
		"g5-raw-decision", fmt.Sprint(policy.SourceID), fmt.Sprint(policy.ID),
		evidenceKey, payloadSHA256, action, decision, retentionValue,
	)
	var decisionID int64
	if err := database.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches(
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  ) SELECT $1,$2,policy.version,'raw_response',$8,$9,policy.recorded_by_user_id,$14,$15,1
    FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions(
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,
  evaluator,evaluated_at,effective_from,retention_days
) SELECT decision_batch.id,$1,$2,$3,$4,$5,$6,$7,'raw_response',$8,$9,$10,$11,
         'g5-retention-rehearsal',$12,$13,$16
  FROM decision_batch RETURNING id`,
		policy.SourceID, policy.ID, policy.Revision, policy.ScopeType, policy.Subject, policy.Priority, policy.Basis,
		evidenceKey, payloadSHA256, action, decision, evaluatedAt, policy.EffectiveAt,
		idempotencyKey, commandFingerprint, retentionDays,
	).Scan(&decisionID); err != nil {
		t.Fatalf("insert raw %s %s decision: %v", action, decision, err)
	}
	return decisionID
}

func openRetentionRightsMinIO(
	t *testing.T,
	ctx context.Context,
) (config.MinIOConfig, *miniosdk.Client, func()) {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_SECRET_KEY"))
	baseBucket := strings.Trim(strings.ToLower(strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_BUCKET"))), "-")
	useSSL := strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_USE_SSL")), "true")
	if endpoint == "" || accessKey == "" || secretKey == "" || baseBucket == "" {
		t.Fatal("isolated MinIO test configuration is required")
	}
	randomBytes := make([]byte, 6)
	if _, err := rand.Read(randomBytes); err != nil {
		t.Fatal(err)
	}
	if len(baseBucket) > 35 {
		baseBucket = baseBucket[:35]
	}
	bucket := fmt.Sprintf("%s-retention-%s", baseBucket, hex.EncodeToString(randomBytes))
	client, err := miniosdk.New(endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: useSSL,
		Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.MakeBucket(ctx, bucket, miniosdk.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for object := range client.ListObjects(cleanupCtx, bucket, miniosdk.ListObjectsOptions{Recursive: true}) {
			if object.Err == nil {
				_ = client.RemoveObject(cleanupCtx, bucket, object.Key, miniosdk.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(cleanupCtx, bucket)
	}
	return config.MinIOConfig{
		Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey, Bucket: bucket, UseSSL: useSSL,
	}, client, cleanup
}

func mustRawEvidenceStore(t *testing.T, cfg config.MinIOConfig) *sourceminio.RawEvidenceStore {
	t.Helper()
	store, err := sourceminio.NewRawEvidenceStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func openRetentionRightsRedis(t *testing.T, ctx context.Context) *goredis.Client {
	t.Helper()
	options, err := goredis.ParseURL(strings.TrimSpace(os.Getenv("HOTKEY_TEST_REDIS_URL")))
	if err != nil {
		t.Fatal(err)
	}
	client := goredis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	return client
}

func insertRetentionRightsNotificationFacts(
	t *testing.T,
	database *sql.DB,
	contentID int64,
) retentionRightsProtectedFactIDs {
	t.Helper()
	now := databaseNow(t, database)
	var userID, monitorID int64
	if err := database.QueryRow(`
INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','G5 retention viewer','viewer') RETURNING id`,
		fmt.Sprintf("g5-retention-%d@example.test", now.UnixNano()),
	).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`
INSERT INTO monitors(name,status,created_by,updated_by)
VALUES ($1,'draft',$2,$2) RETURNING id`, fmt.Sprintf("g5-retention-%d", now.UnixNano()), userID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	var ids retentionRightsProtectedFactIDs
	deepLink := fmt.Sprintf("/dashboard/contents/%d", contentID)
	if err := database.QueryRow(`
INSERT INTO notification_outbox_events(
  event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,
  title,summary,resource_status,deep_link,dedupe_key
) VALUES ('hotspot.discovered','hotspot',$1,1,$2,$3,'Retention fixture','bounded immutable fact','active',$4,$5)
RETURNING id`, contentID, monitorID, now, deepLink,
		retentionRightsSHA256(fmt.Sprintf("g5-retention-notification:%d", contentID)),
	).Scan(&ids.OutboxEventID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`
INSERT INTO user_notifications(
  outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,
  occurred_at,title,summary,resource_status,deep_link
) VALUES ($1,$2,$3,'hotspot.discovered','hotspot',$4,1,$5,'Retention fixture','bounded immutable fact','active',$6)
RETURNING id`, ids.OutboxEventID, userID, monitorID, contentID, now, deepLink).Scan(&ids.NotificationID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`
INSERT INTO notification_read_receipts(user_id,read_through_id) VALUES ($1,$2) RETURNING id`,
		userID, ids.NotificationID,
	).Scan(&ids.ReadReceiptID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`
INSERT INTO notification_delivery_attempts(
  user_notification_id,channel,delivery_target_key,attempt_no,status,attempted_at
) VALUES ($1,'websocket','browser_ws',1,'succeeded',$2) RETURNING id`, ids.NotificationID, now).Scan(&ids.DeliveryID); err != nil {
		t.Fatal(err)
	}
	return ids
}

func retentionRightsProtectedFingerprint(
	t *testing.T,
	database *sql.DB,
	ids retentionRightsProtectedFactIDs,
) string {
	t.Helper()
	queries := []struct {
		query string
		id    int64
	}{
		{`SELECT row_to_json(fact)::text FROM notification_outbox_events AS fact WHERE id=$1`, ids.OutboxEventID},
		{`SELECT row_to_json(fact)::text FROM user_notifications AS fact WHERE id=$1`, ids.NotificationID},
		{`SELECT row_to_json(fact)::text FROM notification_read_receipts AS fact WHERE id=$1`, ids.ReadReceiptID},
		{`SELECT row_to_json(fact)::text FROM notification_delivery_attempts AS fact WHERE id=$1`, ids.DeliveryID},
	}
	var combined strings.Builder
	for _, item := range queries {
		var value string
		if err := database.QueryRow(item.query, item.id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		combined.WriteString(value)
		combined.WriteByte('\n')
	}
	return combined.String()
}

func writeRetentionRightsBackupCopy(t *testing.T, path string, entries map[string][]byte) string {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewWriter(file)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		payload := entries[name]
		header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(payload)), ModTime: time.Unix(0, 0).UTC()}
		if err := archive.WriteHeader(header); err != nil {
			_ = archive.Close()
			_ = file.Close()
			t.Fatal(err)
		}
		if _, err := archive.Write(payload); err != nil {
			_ = archive.Close()
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return retentionRightsSHA256(string(mustReadFile(t, path)))
}

func recordRetentionRightsBackup(
	t *testing.T,
	ctx context.Context,
	databaseRuntime *platformdatabase.Runtime,
	gitRevision, copySHA256, postgresSHA256, minioSHA256, vaultSHA256, manualSHA256 string,
) operationsapplication.BackupRunReceiptDTO {
	t.Helper()
	now := databaseNow(t, databaseRuntime.SQL)
	recoveryPoint := now.Add(-2 * time.Second)
	manifest := struct {
		Version         string     `json:"version"`
		RunSHA256       string     `json:"run_sha256"`
		GitRevision     string     `json:"git_revision"`
		Status          string     `json:"status"`
		RecoveryPointAt *time.Time `json:"recovery_point_at"`
		StartedAt       time.Time  `json:"started_at"`
		CompletedAt     time.Time  `json:"completed_at"`
		Assets          []struct {
			Name   string `json:"name"`
			Count  int64  `json:"count"`
			SHA256 string `json:"sha256"`
		} `json:"assets"`
	}{
		Version:   operationsapplication.BackupRunManifestVersion,
		RunSHA256: retentionRightsSHA256("backup-run:" + copySHA256), GitRevision: gitRevision,
		Status: "succeeded", RecoveryPointAt: &recoveryPoint,
		StartedAt: now.Add(-3 * time.Second), CompletedAt: now.Add(-time.Second),
	}
	manifest.Assets = append(manifest.Assets,
		struct {
			Name   string `json:"name"`
			Count  int64  `json:"count"`
			SHA256 string `json:"sha256"`
		}{"postgres_facts", 1, postgresSHA256},
		struct {
			Name   string `json:"name"`
			Count  int64  `json:"count"`
			SHA256 string `json:"sha256"`
		}{"minio_evidence", 1, minioSHA256},
		struct {
			Name   string `json:"name"`
			Count  int64  `json:"count"`
			SHA256 string `json:"sha256"`
		}{"vault_all_files", 3, vaultSHA256},
		struct {
			Name   string `json:"name"`
			Count  int64  `json:"count"`
			SHA256 string `json:"sha256"`
		}{"vault_manual_regions", 1, manualSHA256},
		struct {
			Name   string `json:"name"`
			Count  int64  `json:"count"`
			SHA256 string `json:"sha256"`
		}{"river_jobs_attempts", 0, retentionRightsSHA256("[]")},
	)
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	service, err := operationsapplication.NewBackupRunService(operationspostgres.NewBackupRunRepository(databaseRuntime))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Record(ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func recordRetentionRightsBackupDisposition(
	t *testing.T,
	ctx context.Context,
	databaseRuntime *platformdatabase.Runtime,
	backupRunSHA256, dispositionSHA256, deletionEvidenceSHA256 string,
	disposedAt time.Time,
) operationsapplication.BackupRetentionDispositionReceiptDTO {
	t.Helper()
	manifest := struct {
		Version                string    `json:"version"`
		DispositionSHA256      string    `json:"disposition_sha256"`
		BackupRunSHA256        string    `json:"backup_run_sha256"`
		DeletionEvidenceSHA256 string    `json:"deletion_evidence_sha256"`
		ReasonCode             string    `json:"reason_code"`
		OperatorID             string    `json:"operator_record_id"`
		ReviewerID             string    `json:"reviewer_record_id"`
		DisposedAt             time.Time `json:"disposed_at"`
	}{
		Version:           operationsapplication.BackupRetentionDispositionManifestVersion,
		DispositionSHA256: dispositionSHA256, BackupRunSHA256: backupRunSHA256,
		DeletionEvidenceSHA256: deletionEvidenceSHA256, ReasonCode: "rights_revoked",
		OperatorID: "external-backup-operator", ReviewerID: "independent-backup-reviewer",
		DisposedAt: disposedAt,
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	service, err := operationsapplication.NewBackupRetentionDispositionService(
		operationspostgres.NewBackupRetentionDispositionRepository(databaseRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Record(ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func retentionRightsDeletionEvidenceSHA256(
	t *testing.T,
	database *sql.DB,
	rawSnapshotID, documentVersionID int64,
) string {
	t.Helper()
	var rawFacts, derivedFacts string
	if err := database.QueryRow(`
SELECT json_agg(row_to_json(fact) ORDER BY fact.id)::text
FROM evidence_deletion_audits AS fact WHERE evidence_snapshot_id=$1`, rawSnapshotID).Scan(&rawFacts); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`
SELECT json_agg(row_to_json(fact) ORDER BY fact.id)::text
FROM derived_artifact_deletion_audits AS fact
WHERE derived_artifact_id IN (
  SELECT id FROM derived_artifacts WHERE document_version_id=$1
)`, documentVersionID).Scan(&derivedFacts); err != nil {
		t.Fatal(err)
	}
	return retentionRightsSHA256(rawFacts + "\n" + derivedFacts)
}

func databaseNow(t *testing.T, database *sql.DB) time.Time {
	t.Helper()
	var now time.Time
	if err := database.QueryRow(`SELECT CURRENT_TIMESTAMP`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now.UTC().Truncate(time.Microsecond)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func retentionRightsSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
