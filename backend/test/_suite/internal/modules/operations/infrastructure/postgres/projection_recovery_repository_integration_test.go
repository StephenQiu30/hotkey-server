package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	knowledgejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/jobs"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/postgres"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	platformdatabase "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type emptyProjectionRecoveryVault struct{}

func (emptyProjectionRecoveryVault) Inspect(context.Context, int64) (knowledgeapplication.VaultRecoveryInspection, error) {
	return knowledgeapplication.VaultRecoveryInspection{}, fmt.Errorf("unexpected Vault inspection")
}

type recoverableProjectionVault struct {
	content string
	missing bool
	writes  int
}

func (vault *recoverableProjectionVault) Read(string, string) ([]byte, string, error) {
	if vault.missing {
		return nil, "", os.ErrNotExist
	}
	return []byte(vault.content), "events/9701.md", nil
}

func (vault *recoverableProjectionVault) CompareAndSwap(_, _ string, expectedHash, replacement string) (string, error) {
	currentHash := knowledgedomain.HashContent("", vault.content)
	if vault.missing {
		currentHash = knowledgedomain.HashContent("", "")
	}
	if currentHash != expectedHash {
		return "", knowledgedomain.ErrVaultConflict
	}
	vault.content = replacement
	vault.missing = false
	vault.writes++
	return knowledgedomain.HashContent("", replacement), nil
}

type projectionRecoverySnapshot struct{ content string }

func (snapshot projectionRecoverySnapshot) ReadVaultSnapshot(context.Context, string, int64) (string, error) {
	return snapshot.content, nil
}

func TestProjectionRecoverySchedulesMissingVaultAndWorkerConvergesToZero(t *testing.T) {
	ctx := context.Background()
	runtime := openOperationsRuntime(t)
	defer func() { _ = runtime.Close() }()
	fact, protected := insertRecoverableKnowledgeProjection(t, runtime)
	vault := &recoverableProjectionVault{missing: true}
	recovery := knowledgeapplication.NewVaultRecoveryService(
		knowledgepostgres.NewRepository(runtime), vault, projectionRecoverySnapshot{content: protected}, nil,
	)
	if err := knowledgedomain.ValidateVaultLocation("events", "9701"); err != nil {
		t.Fatalf("validate recoverable Vault location: %v", err)
	}
	if fact.Document.VaultPath != "events/9701.md" {
		t.Fatalf("recoverable Vault path=%q", fact.Document.VaultPath)
	}
	if _, err := recovery.Inspect(ctx, fact.Document.ID); err != nil {
		t.Fatalf("inspect recoverable Vault fixture %+v: %v", fact, err)
	}
	repository, err := operationspostgres.NewProjectionRecoveryRepository(runtime, recovery, queue.NewStore(runtime))
	if err != nil {
		t.Fatal(err)
	}
	service, err := operationsapplication.NewProjectionRecoveryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.Recover(ctx, operationsapplication.ProjectionRecoveryCommand{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if before.Inspection.MissingVaultProjectionCount != 1 || len(before.Inspection.Blockers) != 0 || vault.writes != 0 {
		t.Fatalf("preflight=%+v vault=%+v", before.Inspection, vault)
	}

	result, err := service.Recover(ctx, operationsapplication.ProjectionRecoveryCommand{
		Apply: true, ConfirmIsolated: true, ProductionEgressDisabled: true,
		OperatorID: "operator-a", ReviewerID: "reviewer-b",
		RunSHA256: strings.Repeat("1", 64), BackupEvidenceSHA256: strings.Repeat("2", 64),
		RehearsalEvidenceSHA256: strings.Repeat("3", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.ScheduledVaultRecoveryCount != 1 || result.Receipt.ScheduledSearchRebuildCount != 0 || vault.writes != 0 {
		t.Fatalf("scheduled receipt=%+v vault=%+v", result.Receipt, vault)
	}

	handler, err := knowledgejobs.NewHandler(queue.KindProjectKnowledge, func(ctx context.Context, documentID int64) error {
		_, recoverErr := recovery.Recover(ctx, documentID)
		return recoverErr
	})
	if err != nil {
		t.Fatal(err)
	}
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindProjectKnowledge: handler.Handle})
	if claimed, runErr := worker.RunOnce(ctx); runErr != nil || !claimed {
		t.Fatalf("recovery Worker RunOnce() = %t/%v", claimed, runErr)
	}
	if vault.writes != 1 || vault.content != protected {
		t.Fatalf("recovered Vault=%+v", vault)
	}

	after, err := service.Recover(ctx, operationsapplication.ProjectionRecoveryCommand{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if after.Inspection.MissingVaultProjectionCount != 0 || after.Inspection.Facts != before.Inspection.Facts ||
		after.Inspection.VaultManualRegionFingerprintSHA256 != before.Inspection.VaultManualRegionFingerprintSHA256 {
		t.Fatalf("post-recovery inspection=%+v, preflight=%+v", after.Inspection, before.Inspection)
	}
	var completed int64
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE kind='project_knowledge' AND state='completed'`).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 1 || fact.Document.ID <= 0 {
		t.Fatalf("completed recovery jobs=%d fact=%+v", completed, fact)
	}
}

func TestProjectionRecoveryRepositoryPreservesNotificationFactsAndUnknownAttempts(t *testing.T) {
	ctx := context.Background()
	runtime := openOperationsRuntime(t)
	defer func() { _ = runtime.Close() }()
	now := time.Now().UTC().Truncate(time.Microsecond)
	notificationIDs := insertProjectionRecoveryNotificationFixture(t, runtime.SQL, now)

	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO notification_delivery_attempts(
  user_notification_id,channel,delivery_target_key,attempt_no,status,dispatch_key,fencing_generation,
  provider_supports_idempotency,provider_supports_receipt_lookup,error_code,attempted_at
) VALUES ($1,'email','primary',1,'unknown',$2,1,false,false,'provider_outcome_unconfirmed',$3)`,
		notificationIDs[0], strings.Repeat("a", 64), now); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO notification_delivery_claims(
  user_notification_id,channel,delivery_target_key,claim_token,fencing_generation,dispatch_key,
  provider_supports_idempotency,provider_supports_receipt_lookup,claimed_at,lease_until
) VALUES ($1,'email','primary',$2,1,$3,false,false,$4::timestamptz,$4::timestamptz+interval '5 minutes')`,
		notificationIDs[1], strings.Repeat("b", 64), strings.Repeat("c", 64), now); err != nil {
		t.Fatal(err)
	}

	repository, err := operationspostgres.NewProjectionRecoveryRepository(runtime, emptyProjectionRecoveryVault{}, queue.NewStore(runtime))
	if err != nil {
		t.Fatal(err)
	}
	service, err := operationsapplication.NewProjectionRecoveryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := service.Recover(ctx, operationsapplication.ProjectionRecoveryCommand{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Inspection.Facts.NotificationOutboxCount != 2 || inspection.Inspection.Facts.UserNotificationCount != 2 ||
		inspection.Inspection.Facts.ReadReceiptCount != 1 || inspection.Inspection.Facts.DeliveryAttemptCount != 1 ||
		inspection.Inspection.DisposableDeliveryClaimCount != 1 || inspection.Inspection.UnknownDeliveryAttemptCount != 1 ||
		len(inspection.Inspection.Facts.FingerprintSHA256) != 64 || len(inspection.Inspection.VaultManualRegionFingerprintSHA256) != 64 {
		t.Fatalf("inspection=%+v", inspection.Inspection)
	}

	command := operationsapplication.ProjectionRecoveryCommand{
		Apply: true, ConfirmIsolated: true, ProductionEgressDisabled: true,
		OperatorID: "operator-a", ReviewerID: "reviewer-b",
		RunSHA256: strings.Repeat("d", 64), BackupEvidenceSHA256: strings.Repeat("e", 64),
		RehearsalEvidenceSHA256: strings.Repeat("f", 64),
	}
	result, err := service.Recover(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.RunID <= 0 || result.Receipt.RemovedDeliveryClaimCount != 1 || result.Receipt.PreservedUnknownAttemptCount != 1 ||
		result.Receipt.BeforeFacts != result.Receipt.AfterFacts ||
		result.Receipt.BeforeVaultManualRegionFingerprintSHA256 != result.Receipt.AfterVaultManualRegionFingerprintSHA256 {
		t.Fatalf("receipt=%+v", result.Receipt)
	}
	var claims, attempts int64
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM notification_delivery_claims`).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM notification_delivery_attempts WHERE status='unknown'`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if claims != 0 || attempts != 1 {
		t.Fatalf("claims=%d unknown_attempts=%d", claims, attempts)
	}
	var recordedRuns int64
	if err := runtime.SQL.QueryRowContext(ctx, `
SELECT count(*)
FROM projection_recovery_runs
WHERE id=$1 AND run_sha256=$2 AND status='scheduled'
  AND operator_record_id=$3 AND reviewer_record_id=$4
  AND backup_evidence_sha256=$5 AND rehearsal_evidence_sha256=$6
  AND notification_facts_before_sha256=notification_facts_after_sha256
  AND vault_manual_before_sha256=vault_manual_after_sha256
  AND notification_outbox_count=2 AND user_notification_count=2
  AND read_receipt_count=1 AND delivery_attempt_count=1
  AND removed_delivery_claim_count=1 AND scheduled_vault_recovery_count=0
  AND scheduled_search_rebuild_count=0 AND preserved_started_claim_count=0
  AND preserved_unknown_attempt_count=1`,
		result.Receipt.RunID, command.RunSHA256, command.OperatorID, command.ReviewerID,
		command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256,
	).Scan(&recordedRuns); err != nil {
		t.Fatal(err)
	}
	if recordedRuns != 1 {
		t.Fatalf("durable projection recovery run records=%d", recordedRuns)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE projection_recovery_runs SET status='scheduled' WHERE id=$1`, result.Receipt.RunID); err == nil {
		t.Fatal("projection recovery run evidence must be append-only")
	}
}

func insertRecoverableKnowledgeProjection(t *testing.T, runtime *platformdatabase.Runtime) (knowledgeapplication.VaultRebuildFact, string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var eventID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events(
event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at)
VALUES ($1,'Recovery knowledge','safe summary','active',$2,$2) RETURNING id`,
		fmt.Sprintf("projection-recovery-%d", now.UnixNano()), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	repository := knowledgepostgres.NewRepository(runtime)
	emptyHash := knowledgedomain.HashContent("", "")
	document := knowledgedomain.Document{
		ID: 9701, Version: 1, RevisionNo: 0, Type: knowledgedomain.DocumentEvent, EventID: &eventID,
		VaultPath: "events/9701.md", ContentHash: emptyHash, GeneratedHash: emptyHash,
		Status: knowledgedomain.DocumentPlanned,
	}
	if err := repository.SaveDocument(ctx, document); err != nil {
		t.Fatal(err)
	}
	frontmatter := `{"title":"Recovery knowledge"}`
	body := "approved recovery body"
	proposal := knowledgedomain.Proposal{
		ID: 9801, Version: 1, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: emptyHash,
		ProposedFrontmatter: frontmatter, ProposedBody: body, Reason: "recovery fixture",
		Status: knowledgedomain.ProposalPending,
	}
	if err := repository.SaveProposal(context.Background(), proposal); err != nil {
		t.Fatal(err)
	}
	approved, err := repository.UpdateProposalStatus(ctx, proposal.ID, proposal.Version, knowledgedomain.ProposalApproved)
	if err != nil {
		t.Fatal(err)
	}
	renderInput := knowledgedomain.VaultDocumentRenderInput{
		DocumentID: document.ID, RevisionNo: 1, Type: document.Type, SourceID: eventID,
		Title: "Recovery knowledge", Generated: body,
	}
	protected, err := knowledgedomain.RenderVaultDocument(renderInput)
	if err != nil {
		t.Fatal(err)
	}
	next := document
	next.Version = 2
	next.RevisionNo = 1
	next.ContentHash = knowledgedomain.HashContent("", protected)
	next.GeneratedHash = knowledgedomain.HashContent(approved.ProposedFrontmatter, body)
	next.Status = knowledgedomain.DocumentActive
	revision := knowledgedomain.Revision{
		DocumentID: document.ID, RevisionNo: 1, ProposalID: proposal.ID, Source: "proposal",
		PreviousHash: emptyHash, NewHash: next.ContentHash,
		SnapshotObjectKey: "knowledge/v1/recovery/1.md", Frontmatter: approved.ProposedFrontmatter,
	}
	if _, err := repository.ApplyProposal(ctx, proposal.ID, approved.Version, next, revision); err != nil {
		t.Fatal(err)
	}
	fact, err := repository.LoadVaultRebuildFact(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	return fact, protected
}

func insertProjectionRecoveryNotificationFixture(t *testing.T, database *sql.DB, now time.Time) []int64 {
	t.Helper()
	var userID, monitorID int64
	if err := database.QueryRow(`INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','Recovery operator','viewer') RETURNING id`, fmt.Sprintf("recovery-%d@example.test", now.UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`INSERT INTO monitors(name,status,created_by,updated_by)
VALUES ($1,'draft',$2,$2) RETURNING id`, fmt.Sprintf("recovery-monitor-%d", now.UnixNano()), userID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	notificationIDs := make([]int64, 0, 2)
	for index := 1; index <= 2; index++ {
		var outboxID, notificationID int64
		if err := database.QueryRow(`INSERT INTO notification_outbox_events(
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,1,$2,$3,'Recovery event','safe summary','urgent',$4,$5) RETURNING id`,
			index, monitorID, now, fmt.Sprintf("/dashboard/contents/%d", index), fmt.Sprintf("recovery:%d:%d", now.UnixNano(), index)).Scan(&outboxID); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRow(`INSERT INTO user_notifications(
outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,
occurred_at,title,summary,resource_status,deep_link)
VALUES ($1,$2,$3,'micro_event.updated','micro_event',$4,1,$5,'Recovery event','safe summary','urgent',$6)
RETURNING id`, outboxID, userID, monitorID, index, now, fmt.Sprintf("/dashboard/contents/%d", index)).Scan(&notificationID); err != nil {
			t.Fatal(err)
		}
		notificationIDs = append(notificationIDs, notificationID)
	}
	if _, err := database.ExecContext(context.Background(), `
INSERT INTO notification_read_receipts(user_id,read_through_id) VALUES ($1,$2)`, userID, notificationIDs[1]); err != nil {
		t.Fatal(err)
	}
	return notificationIDs
}
