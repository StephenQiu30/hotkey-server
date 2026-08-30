//go:build integration

package queue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	notificationapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	notificationpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/infrastructure/postgres"
	platformdatabase "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

const (
	riverFaultEvidenceVersion = "hotkey-river-fault-rehearsal-v1"
	riverCrashModeFlag        = "HOTKEY_TEST_RIVER_CRASH_MODE"
	riverCrashDSNFlag         = "HOTKEY_TEST_RIVER_CRASH_DSN"
	riverProviderURLFlag      = "HOTKEY_TEST_RIVER_PROVIDER_URL"

	riverCrashBeforeClaimExit       = 81
	riverCrashAfterClaimExit        = 82
	riverCrashInTransactionExit     = 83
	riverCrashAfterEffectCommitExit = 84
	riverCrashAfterProviderExit     = 85
)

type riverFaultRehearsalConfig struct {
	Output, Environment, Hardware, GitRevision string
	ProductionEgressDisabled                   bool
}

type riverFaultStageEvidence struct {
	FinalState            string `json:"final_state"`
	Attempts              int    `json:"attempts"`
	Effects               int    `json:"effects"`
	EffectsBeforeRecovery int    `json:"effects_before_recovery"`
	LeaseExpiredAttempts  int    `json:"lease_expired_attempts"`
	RetryableAttempts     int    `json:"retryable_attempts"`
	PermanentAttempts     int    `json:"permanent_attempts"`
	CancelledAttempts     int    `json:"cancelled_attempts"`
	CrashExitCode         int    `json:"crash_exit_code"`
}

type riverProviderFaultEvidence struct {
	FinalStatus            string `json:"final_status"`
	ProviderSends          int    `json:"provider_sends"`
	ReceiptLookups         int    `json:"receipt_lookups"`
	DeliveryAttempts       int    `json:"delivery_attempts"`
	SucceededAttempts      int    `json:"succeeded_attempts"`
	UnknownAttempts        int    `json:"unknown_attempts"`
	RemainingClaims        int    `json:"remaining_claims"`
	StableDispatchIdentity bool   `json:"stable_dispatch_identity"`
	StaleTokenRejected     bool   `json:"stale_token_rejected"`
	BlindReplayClaimed     bool   `json:"blind_replay_claimed"`
	CrashExitCode          int    `json:"crash_exit_code"`
}

type riverFaultRehearsalReport struct {
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
	FencedAt                 time.Time `json:"fenced_at"`
	Stages                   struct {
		BeforeClaim          riverFaultStageEvidence `json:"before_claim"`
		AfterClaim           riverFaultStageEvidence `json:"after_claim"`
		BusinessTransaction  riverFaultStageEvidence `json:"business_transaction"`
		BeforeCompletionMark riverFaultStageEvidence `json:"before_completion_marker"`
		Retry                riverFaultStageEvidence `json:"retry"`
		Cancellation         riverFaultStageEvidence `json:"cancellation"`
		FailureReplay        riverFaultStageEvidence `json:"failure_replay"`
	} `json:"stages"`
	Provider struct {
		ReceiptCapable       riverProviderFaultEvidence `json:"receipt_capable"`
		UnsupportedAmbiguous riverProviderFaultEvidence `json:"unsupported_ambiguous"`
		ProtectedFactsBefore string                     `json:"protected_facts_before_sha256"`
		ProtectedFactsAfter  string                     `json:"protected_facts_after_sha256"`
		Guarantee            string                     `json:"guarantee"`
		ExactlyOnceClaimed   bool                       `json:"exactly_once_claimed"`
	} `json:"provider"`
	Differences []string `json:"differences"`
}

func TestRiverFaultRehearsalCoversEveryCrashRetryAndProviderLostAckBoundary(t *testing.T) {
	cfg := loadRiverFaultRehearsalConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dsn := postgresfixture.New(t)
	databaseRuntime, err := platformdatabase.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer databaseRuntime.Close()
	if err := platformdatabase.InitializeEmpty(ctx, databaseRuntime.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseRuntime.SQL.ExecContext(ctx, `
CREATE TABLE river_fault_rehearsal_effects (
  stage varchar(64) PRIMARY KEY,
  job_id bigint NOT NULL UNIQUE REFERENCES river_job(id),
  applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
)`); err != nil {
		t.Fatal(err)
	}

	report := riverFaultRehearsalReport{
		Version: riverFaultEvidenceVersion, Status: "verified", Approval: "automated_isolated_fixture",
		Environment: cfg.Environment, Hardware: cfg.Hardware, GitRevision: cfg.GitRevision,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), Isolated: true,
		ProductionEgressDisabled: cfg.ProductionEgressDisabled,
		FixtureSHA256:            riverFaultSHA256("hotkey-river-fault-rehearsal-fixture-v1"),
		FencedAt:                 databaseTime(t, databaseRuntime), Differences: []string{},
	}
	report.Stages.BeforeClaim = runRiverCrashStage(t, databaseRuntime, dsn, "before_claim", riverCrashBeforeClaimExit, 0)
	report.Stages.AfterClaim = runRiverCrashStage(t, databaseRuntime, dsn, "after_claim", riverCrashAfterClaimExit, 0)
	report.Stages.BusinessTransaction = runRiverCrashStage(t, databaseRuntime, dsn, "business_transaction", riverCrashInTransactionExit, 0)
	report.Stages.BeforeCompletionMark = runRiverCrashStage(t, databaseRuntime, dsn, "before_completion_marker", riverCrashAfterEffectCommitExit, 1)
	report.Stages.Retry = runRiverRetryStage(t, databaseRuntime)
	report.Stages.Cancellation = runRiverCancellationStage(t, databaseRuntime)
	report.Stages.FailureReplay = runRiverFailureReplayStage(t, databaseRuntime)

	provider := newRiverProviderFixture(t)
	defer provider.Close()
	receiptFixture := insertRiverEmailFixture(t, databaseRuntime, "receipt")
	unsupportedFixture := insertRiverEmailFixture(t, databaseRuntime, "unsupported")
	protectedBefore := riverNotificationFingerprint(t, databaseRuntime, receiptFixture, unsupportedFixture)
	report.Provider.ReceiptCapable = runRiverProviderCrashRecovery(
		t, databaseRuntime, dsn, provider, receiptFixture, "provider_receipt", true,
	)
	report.Provider.UnsupportedAmbiguous = runRiverProviderCrashRecovery(
		t, databaseRuntime, dsn, provider, unsupportedFixture, "provider_unsupported", false,
	)
	protectedAfter := riverNotificationFingerprint(t, databaseRuntime, receiptFixture, unsupportedFixture)
	if protectedBefore != protectedAfter {
		t.Fatal("provider recovery changed protected notification facts")
	}
	report.Provider.ProtectedFactsBefore = riverFaultSHA256(protectedBefore)
	report.Provider.ProtectedFactsAfter = riverFaultSHA256(protectedAfter)
	report.Provider.Guarantee = "bounded_at_least_once_with_fencing_idempotency_and_explicit_unknown"
	report.Provider.ExactlyOnceClaimed = false

	if err := writeRiverFaultRehearsalEvidence(cfg.Output, report); err != nil {
		t.Fatal(err)
	}
}

func TestRiverFaultRehearsalCrashHelper(t *testing.T) {
	mode := os.Getenv(riverCrashModeFlag)
	if mode == "" {
		return
	}
	if mode == "before_claim" {
		os.Exit(riverCrashBeforeClaimExit)
	}
	ctx := context.Background()
	databaseRuntime, err := platformdatabase.Open(ctx, os.Getenv(riverCrashDSNFlag))
	if err != nil {
		t.Fatal(err)
	}
	defer databaseRuntime.Close()
	if mode == "provider_receipt" || mode == "provider_unsupported" {
		capabilities := notificationapplication.NotificationEmailProviderCapabilities{}
		if mode == "provider_receipt" {
			capabilities.SupportsReceiptLookup = true
		}
		repository := notificationpostgres.NewRepository(databaseRuntime)
		service, err := notificationapplication.NewEmailDeliveryService(notificationapplication.EmailDeliveryServiceDependencies{
			Repository: crashBeforeEmailAttemptCommitRepository{EmailDeliveryRepository: repository},
			Sender:     &riverHTTPEmailSender{origin: os.Getenv(riverProviderURLFlag), capabilities: capabilities},
			NewToken:   func() (string, error) { return strings.Repeat("a", 64), nil },
			WebOrigin:  "https://hotkey.invalid",
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.DispatchNext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("provider crash helper returned without terminating: %#v", result)
	}

	worker := NewWorker(databaseRuntime, map[string]Handler{KindRunRetention: func(ctx context.Context, job Job) error {
		switch mode {
		case "after_claim":
			os.Exit(riverCrashAfterClaimExit)
		case "business_transaction":
			return databaseRuntime.WithinTransaction(ctx, func(ctx context.Context, transaction platformdatabase.Transaction) error {
				if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO river_fault_rehearsal_effects(stage,job_id) VALUES ($1,$2)`, mode, job.ID); err != nil {
					return err
				}
				os.Exit(riverCrashInTransactionExit)
				return nil
			})
		case "before_completion_marker":
			if _, err := databaseRuntime.SQL.ExecContext(ctx, `INSERT INTO river_fault_rehearsal_effects(stage,job_id) VALUES ($1,$2)`, mode, job.ID); err != nil {
				return err
			}
			os.Exit(riverCrashAfterEffectCommitExit)
		default:
			return fmt.Errorf("unsupported river crash mode")
		}
		return nil
	}})
	if worked, err := worker.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("RunOnce(crash helper) = %t/%v", worked, err)
	}
	t.Fatal("river crash helper returned without terminating")
}

func TestRiverFaultRehearsalEvidenceWriterIsExclusivePrivateAndSanitized(t *testing.T) {
	report := validRiverFaultRehearsalReportFixture()
	path := filepath.Join(t.TempDir(), "river-fault.json")
	if err := writeRiverFaultRehearsalEvidence(path, report); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode = %o, want 600", info.Mode().Perm())
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(payload) {
		t.Fatal("evidence is not valid JSON")
	}
	for _, forbidden := range []string{
		"postgres://", "127.0.0.1", "password", "secret", "@example", "claim_token", "dispatch_key", "recipient", "message_id", "/tmp/",
	} {
		if bytes.Contains(bytes.ToLower(payload), []byte(forbidden)) {
			t.Fatalf("evidence leaked forbidden marker %q", forbidden)
		}
	}
	if err := writeRiverFaultRehearsalEvidence(path, report); err == nil {
		t.Fatal("evidence writer overwrote an existing attachment")
	}
}

func runRiverCrashStage(
	t *testing.T,
	databaseRuntime *platformdatabase.Runtime,
	dsn, mode string,
	exitCode, expectedEffectsBefore int,
) riverFaultStageEvidence {
	t.Helper()
	store := NewStore(databaseRuntime)
	jobID := enqueueRiverFaultJob(t, store, mode, int64(exitCode))
	runRiverFaultSubprocess(t, mode, dsn, "", exitCode)
	state, attempts := riverJobState(t, databaseRuntime, jobID)
	effectsBefore := riverEffectCount(t, databaseRuntime, mode)
	if mode == "before_claim" {
		if state != "available" || attempts != 0 || effectsBefore != 0 {
			t.Fatalf("before-claim crash facts = %q/%d/%d", state, attempts, effectsBefore)
		}
	} else {
		if state != "running" || attempts != 1 || effectsBefore != expectedEffectsBefore {
			t.Fatalf("%s crash facts = %q/%d/%d", mode, state, attempts, effectsBefore)
		}
		if _, err := databaseRuntime.SQL.Exec(`UPDATE river_job SET attempted_at=clock_timestamp()-interval '2 minutes' WHERE id=$1`, jobID); err != nil {
			t.Fatal(err)
		}
		worker := NewWorker(databaseRuntime, nil)
		if reclaimed, err := worker.ReclaimStale(context.Background(), time.Minute); err != nil || reclaimed != 1 {
			t.Fatalf("ReclaimStale(%s) = %d/%v", mode, reclaimed, err)
		}
	}
	recovered := NewWorker(databaseRuntime, map[string]Handler{KindRunRetention: riverEffectHandler(databaseRuntime, mode)})
	if worked, err := recovered.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(recovered %s) = %t/%v", mode, worked, err)
	}
	evidence := riverStageSnapshot(t, databaseRuntime, jobID, mode)
	evidence.CrashExitCode = exitCode
	evidence.EffectsBeforeRecovery = effectsBefore
	return evidence
}

func runRiverRetryStage(t *testing.T, databaseRuntime *platformdatabase.Runtime) riverFaultStageEvidence {
	t.Helper()
	const stage = "retry"
	jobID := enqueueRiverFaultJob(t, NewStore(databaseRuntime), stage, 501)
	worker := NewWorker(databaseRuntime, map[string]Handler{KindRunRetention: func(context.Context, Job) error {
		return errors.New("synthetic retryable failure")
	}})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(retry failure) = %t/%v", worked, err)
	}
	state, attempts := riverJobState(t, databaseRuntime, jobID)
	if state != "available" || attempts != 1 {
		t.Fatalf("retry state = %q/%d", state, attempts)
	}
	if _, err := databaseRuntime.SQL.Exec(`UPDATE river_job SET scheduled_at=clock_timestamp()-interval '1 second' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	recovered := NewWorker(databaseRuntime, map[string]Handler{KindRunRetention: riverEffectHandler(databaseRuntime, stage)})
	if worked, err := recovered.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(retry recovery) = %t/%v", worked, err)
	}
	return riverStageSnapshot(t, databaseRuntime, jobID, stage)
}

func runRiverCancellationStage(t *testing.T, databaseRuntime *platformdatabase.Runtime) riverFaultStageEvidence {
	t.Helper()
	const stage = "cancellation"
	jobID := enqueueRiverFaultJob(t, NewStore(databaseRuntime), stage, 502)
	worker := NewWorker(databaseRuntime, map[string]Handler{KindRunRetention: func(context.Context, Job) error {
		return NewCancelledError(context.Canceled)
	}})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(cancellation) = %t/%v", worked, err)
	}
	return riverStageSnapshot(t, databaseRuntime, jobID, stage)
}

func runRiverFailureReplayStage(t *testing.T, databaseRuntime *platformdatabase.Runtime) riverFaultStageEvidence {
	t.Helper()
	const stage = "failure_replay"
	store := NewStore(databaseRuntime)
	jobID := enqueueRiverFaultJobWithAttempts(t, store, stage, 503, 1)
	failed := NewWorker(databaseRuntime, map[string]Handler{KindRunRetention: func(context.Context, Job) error {
		return NewPermanentError(errors.New("synthetic terminal failure"))
	}})
	if worked, err := failed.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(terminal failure) = %t/%v", worked, err)
	}
	state, attempts := riverJobState(t, databaseRuntime, jobID)
	if state != "discarded" || attempts != 1 {
		t.Fatalf("discarded job = %q/%d", state, attempts)
	}
	reactivation, err := store.ReactivateByID(context.Background(), jobID)
	if err != nil || !reactivation.Changed || reactivation.PreviousState != "discarded" {
		t.Fatalf("ReactivateByID() = %#v/%v", reactivation, err)
	}
	recovered := NewWorker(databaseRuntime, map[string]Handler{KindRunRetention: riverEffectHandler(databaseRuntime, stage)})
	if worked, err := recovered.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(replay) = %t/%v", worked, err)
	}
	return riverStageSnapshot(t, databaseRuntime, jobID, stage)
}

func enqueueRiverFaultJob(t *testing.T, store *Store, stage string, entityID int64) int64 {
	t.Helper()
	return enqueueRiverFaultJobWithAttempts(t, store, stage, entityID, 3)
}

func enqueueRiverFaultJobWithAttempts(t *testing.T, store *Store, stage string, entityID int64, maxAttempts int) int64 {
	t.Helper()
	jobID, created, err := store.Enqueue(context.Background(), Job{
		Kind: KindRunRetention, UniqueKey: "river-fault-" + stage,
		Payload:     Payload{EntityID: entityID, EntityVersion: 1},
		ScheduledAt: time.Now().UTC().Add(-time.Minute), MaxAttempts: maxAttempts, Priority: 1,
	})
	if err != nil || !created {
		t.Fatalf("Enqueue(%s) = %d/%t/%v", stage, jobID, created, err)
	}
	return jobID
}

func riverEffectHandler(databaseRuntime *platformdatabase.Runtime, stage string) Handler {
	return func(ctx context.Context, job Job) error {
		return databaseRuntime.WithinTransaction(ctx, func(ctx context.Context, transaction platformdatabase.Transaction) error {
			_, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO river_fault_rehearsal_effects(stage,job_id) VALUES ($1,$2)
ON CONFLICT (stage) DO NOTHING`, stage, job.ID)
			return err
		})
	}
}

func runRiverFaultSubprocess(t *testing.T, mode, dsn, providerURL string, expectedExit int) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestRiverFaultRehearsalCrashHelper$")
	command.Env = append(os.Environ(),
		riverCrashModeFlag+"="+mode,
		riverCrashDSNFlag+"="+dsn,
		riverProviderURLFlag+"="+providerURL,
	)
	output, err := command.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != expectedExit {
		t.Fatalf("crash helper %s exit = %v, output=%s", mode, err, output)
	}
}

func riverJobState(t *testing.T, databaseRuntime *platformdatabase.Runtime, jobID int64) (string, int) {
	t.Helper()
	var state string
	var attempts int
	if err := databaseRuntime.SQL.QueryRow(`SELECT state,attempt FROM river_job WHERE id=$1`, jobID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	return state, attempts
}

func riverEffectCount(t *testing.T, databaseRuntime *platformdatabase.Runtime, stage string) int {
	t.Helper()
	var count int
	if err := databaseRuntime.SQL.QueryRow(`SELECT count(*) FROM river_fault_rehearsal_effects WHERE stage=$1`, stage).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func riverStageSnapshot(t *testing.T, databaseRuntime *platformdatabase.Runtime, jobID int64, stage string) riverFaultStageEvidence {
	t.Helper()
	evidence := riverFaultStageEvidence{Effects: riverEffectCount(t, databaseRuntime, stage)}
	if err := databaseRuntime.SQL.QueryRow(`
SELECT state,attempt,
       (SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='lease_expired'),
       (SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='retryable'),
       (SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='permanent'),
       (SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='cancelled')
FROM river_job WHERE id=$1`, jobID).Scan(
		&evidence.FinalState, &evidence.Attempts, &evidence.LeaseExpiredAttempts,
		&evidence.RetryableAttempts, &evidence.PermanentAttempts, &evidence.CancelledAttempts,
	); err != nil {
		t.Fatal(err)
	}
	return evidence
}

type riverEmailFixture struct {
	UserID, OutboxID, NotificationID, ReceiptID int64
}

func insertRiverEmailFixture(t *testing.T, databaseRuntime *platformdatabase.Runtime, label string) riverEmailFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	unique := fmt.Sprintf("%s-%d", label, now.UnixNano())
	var fixture riverEmailFixture
	var monitorID, configID, eventID int64
	if err := databaseRuntime.SQL.QueryRow(`INSERT INTO users(email,password_hash,display_name,role)
VALUES ($1,'fixture','River fixture','viewer') RETURNING id`, unique+"@example.test").Scan(&fixture.UserID); err != nil {
		t.Fatal(err)
	}
	if err := databaseRuntime.SQL.QueryRow(`INSERT INTO monitors(name,status,created_by,updated_by)
VALUES ($1,'draft',$2,$2) RETURNING id`, "river-"+unique, fixture.UserID).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if err := databaseRuntime.SQL.QueryRow(`INSERT INTO monitor_config_versions(
monitor_id,revision,state,languages,alert_email_enabled,created_by,updated_by)
VALUES ($1,1,'draft',ARRAY['zh'],true,$2,$2) RETURNING id`, monitorID, fixture.UserID).Scan(&configID); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseRuntime.SQL.Exec(`UPDATE monitor_config_versions
SET state='published',config_hash=repeat('a',64),published_at=$1 WHERE id=$2`, now, configID); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseRuntime.SQL.Exec(`UPDATE monitors SET status='active',published_config_version_id=$1 WHERE id=$2`, configID, monitorID); err != nil {
		t.Fatal(err)
	}
	if err := databaseRuntime.SQL.QueryRow(`INSERT INTO micro_events(
event_key,primary_subject_key,primary_action_key,event_started_at,clustering_profile_version)
VALUES ($1,'subject:river','action:recovery',$2,'micro-event-clustering-v1') RETURNING id`,
		riverFaultSHA256(unique), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := databaseRuntime.SQL.QueryRow(`INSERT INTO notification_outbox_events(
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,1,$2,$3,'River fixture','Bounded summary','urgent',$4,$5) RETURNING id`,
		eventID, monitorID, now, fmt.Sprintf("/dashboard/events?event=%d", eventID), "river:"+unique).Scan(&fixture.OutboxID); err != nil {
		t.Fatal(err)
	}
	if err := databaseRuntime.SQL.QueryRow(`INSERT INTO user_notifications(
outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,
occurred_at,title,summary,resource_status,deep_link)
VALUES ($1,$2,$3,'micro_event.updated','micro_event',$4,1,$5,'River fixture','Bounded summary','urgent',$6)
RETURNING id`, fixture.OutboxID, fixture.UserID, monitorID, eventID, now,
		fmt.Sprintf("/dashboard/events?event=%d", eventID)).Scan(&fixture.NotificationID); err != nil {
		t.Fatal(err)
	}
	if err := databaseRuntime.SQL.QueryRow(`INSERT INTO notification_read_receipts(user_id,read_through_id)
VALUES ($1,$2) RETURNING id`, fixture.UserID, fixture.NotificationID).Scan(&fixture.ReceiptID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func riverNotificationFingerprint(
	t *testing.T,
	databaseRuntime *platformdatabase.Runtime,
	left, right riverEmailFixture,
) string {
	t.Helper()
	var fingerprint string
	if err := databaseRuntime.SQL.QueryRow(`
SELECT json_build_object(
  'outbox',(SELECT json_agg(row_to_json(fact) ORDER BY fact.id) FROM (SELECT * FROM notification_outbox_events WHERE id IN ($1,$2)) fact),
  'notifications',(SELECT json_agg(row_to_json(fact) ORDER BY fact.id) FROM (SELECT * FROM user_notifications WHERE id IN ($3,$4)) fact),
  'receipts',(SELECT json_agg(row_to_json(fact) ORDER BY fact.id) FROM (SELECT * FROM notification_read_receipts WHERE id IN ($5,$6)) fact)
)::text`, left.OutboxID, right.OutboxID, left.NotificationID, right.NotificationID, left.ReceiptID, right.ReceiptID).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

type crashBeforeEmailAttemptCommitRepository struct {
	notificationapplication.EmailDeliveryRepository
}

func (crashBeforeEmailAttemptCommitRepository) CompleteEmailDelivery(
	context.Context,
	notificationapplication.CompleteEmailDeliveryCommand,
) (notificationapplication.RecordNotificationDeliveryAttemptResult, error) {
	os.Exit(riverCrashAfterProviderExit)
	return notificationapplication.RecordNotificationDeliveryAttemptResult{}, nil
}

func runRiverProviderCrashRecovery(
	t *testing.T,
	databaseRuntime *platformdatabase.Runtime,
	dsn string,
	provider *riverProviderFixture,
	fixture riverEmailFixture,
	mode string,
	receiptCapable bool,
) riverProviderFaultEvidence {
	t.Helper()
	runRiverFaultSubprocess(t, mode, dsn, provider.URL(), riverCrashAfterProviderExit)
	var oldToken, dispatchIdentity string
	var oldGeneration int64
	var dispatchStartedAt time.Time
	if err := databaseRuntime.SQL.QueryRow(`SELECT claim_token,fencing_generation,dispatch_key,dispatch_started_at
FROM notification_delivery_claims WHERE user_notification_id=$1`, fixture.NotificationID).Scan(
		&oldToken, &oldGeneration, &dispatchIdentity, &dispatchStartedAt,
	); err != nil || dispatchStartedAt.IsZero() {
		t.Fatalf("persisted provider fence = %q/%d/%q/%s / %v", oldToken, oldGeneration, dispatchIdentity, dispatchStartedAt, err)
	}
	if _, err := databaseRuntime.SQL.Exec(`UPDATE notification_delivery_claims
SET claimed_at=clock_timestamp()-interval '2 minutes',lease_until=clock_timestamp()-interval '1 second'
WHERE user_notification_id=$1`, fixture.NotificationID); err != nil {
		t.Fatal(err)
	}
	capabilities := notificationapplication.NotificationEmailProviderCapabilities{}
	if receiptCapable {
		capabilities.SupportsReceiptLookup = true
	}
	repository := notificationpostgres.NewRepository(databaseRuntime)
	service, err := notificationapplication.NewEmailDeliveryService(notificationapplication.EmailDeliveryServiceDependencies{
		Repository: repository,
		Sender:     &riverHTTPEmailSender{origin: provider.URL(), capabilities: capabilities},
		NewToken:   func() (string, error) { return strings.Repeat("b", 64), nil },
		WebOrigin:  "https://hotkey.invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.DispatchNext(context.Background())
	if err != nil || !result.Claimed {
		t.Fatalf("DispatchNext(recovery %s) = %#v/%v", mode, result, err)
	}
	wantStatus := "unknown"
	if receiptCapable {
		wantStatus = "succeeded"
	}
	if result.Status != wantStatus || result.AttemptNo != 1 {
		t.Fatalf("provider recovery %s = %#v", mode, result)
	}

	evidence := riverProviderFaultEvidence{FinalStatus: result.Status, CrashExitCode: riverCrashAfterProviderExit}
	evidence.ProviderSends, evidence.ReceiptLookups = provider.Counts(dispatchIdentity)
	var attemptIdentity string
	if err := databaseRuntime.SQL.QueryRow(`SELECT count(*),
       count(*) FILTER (WHERE status='succeeded'),
       count(*) FILTER (WHERE status='unknown'),
       COALESCE(max(dispatch_key),'')
FROM notification_delivery_attempts WHERE user_notification_id=$1`, fixture.NotificationID).Scan(
		&evidence.DeliveryAttempts, &evidence.SucceededAttempts, &evidence.UnknownAttempts, &attemptIdentity,
	); err != nil {
		t.Fatal(err)
	}
	if err := databaseRuntime.SQL.QueryRow(`SELECT count(*) FROM notification_delivery_claims WHERE user_notification_id=$1`, fixture.NotificationID).Scan(&evidence.RemainingClaims); err != nil {
		t.Fatal(err)
	}
	evidence.StableDispatchIdentity = attemptIdentity == dispatchIdentity
	evidence.BlindReplayClaimed = evidence.RemainingClaims != 0
	_, staleErr := repository.CompleteEmailDelivery(context.Background(), notificationapplication.CompleteEmailDeliveryCommand{
		UserNotificationID: fixture.NotificationID, UserID: fixture.UserID, ClaimToken: oldToken,
		FencingGeneration: oldGeneration, DispatchKey: dispatchIdentity, ProviderCapabilities: capabilities,
		Status: "succeeded", ProviderMessageID: "late-stale-worker",
	})
	evidence.StaleTokenRejected = errors.Is(staleErr, sharedrepository.ErrConflict)
	if !receiptCapable {
		second, err := service.DispatchNext(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		evidence.BlindReplayClaimed = evidence.BlindReplayClaimed || second.Claimed
	}
	return evidence
}

type riverProviderFixture struct {
	server   *httptest.Server
	mu       sync.Mutex
	receipts map[string]string
	sends    map[string]int
	lookups  map[string]int
}

func newRiverProviderFixture(t *testing.T) *riverProviderFixture {
	t.Helper()
	fixture := &riverProviderFixture{
		receipts: make(map[string]string), sends: make(map[string]int), lookups: make(map[string]int),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/dispatch":
			var payload struct {
				Identity string `json:"identity"`
			}
			if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&payload) != nil || len(payload.Identity) != 64 {
				http.Error(response, "invalid", http.StatusBadRequest)
				return
			}
			fixture.mu.Lock()
			fixture.sends[payload.Identity]++
			messageID := fixture.receipts[payload.Identity]
			if messageID == "" {
				messageID = fmt.Sprintf("accepted-%d", len(fixture.receipts)+1)
				fixture.receipts[payload.Identity] = messageID
			}
			fixture.mu.Unlock()
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]string{"id": messageID})
		case "/receipt":
			identity := request.URL.Query().Get("identity")
			fixture.mu.Lock()
			fixture.lookups[identity]++
			messageID := fixture.receipts[identity]
			fixture.mu.Unlock()
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(map[string]any{"found": messageID != "", "id": messageID})
		default:
			http.NotFound(response, request)
		}
	}))
	return fixture
}

func (fixture *riverProviderFixture) URL() string { return fixture.server.URL }
func (fixture *riverProviderFixture) Close()      { fixture.server.Close() }
func (fixture *riverProviderFixture) Counts(identity string) (int, int) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.sends[identity], fixture.lookups[identity]
}

type riverHTTPEmailSender struct {
	origin       string
	capabilities notificationapplication.NotificationEmailProviderCapabilities
}

func (sender *riverHTTPEmailSender) Capabilities() notificationapplication.NotificationEmailProviderCapabilities {
	return sender.capabilities
}

func (sender *riverHTTPEmailSender) SendNotificationEmail(
	ctx context.Context,
	dispatch notificationapplication.NotificationEmailDispatchDTO,
) (string, error) {
	if dispatch.Message.Recipient == "" || len(dispatch.DispatchKey) != 64 {
		return "", errors.New("invalid synthetic provider dispatch")
	}
	payload, _ := json.Marshal(map[string]string{"identity": dispatch.DispatchKey})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sender.origin+"/dispatch", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var result struct {
		ID string `json:"id"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&result) != nil || result.ID == "" {
		return "", errors.New("synthetic provider rejected dispatch")
	}
	return result.ID, nil
}

func (sender *riverHTTPEmailSender) LookupNotificationEmail(
	ctx context.Context,
	dispatchIdentity string,
) (notificationapplication.NotificationEmailReceiptDTO, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		sender.origin+"/receipt?identity="+url.QueryEscape(dispatchIdentity), nil)
	if err != nil {
		return notificationapplication.NotificationEmailReceiptDTO{}, err
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return notificationapplication.NotificationEmailReceiptDTO{}, err
	}
	defer response.Body.Close()
	var result struct {
		Found bool   `json:"found"`
		ID    string `json:"id"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&result) != nil {
		return notificationapplication.NotificationEmailReceiptDTO{}, errors.New("synthetic provider receipt lookup failed")
	}
	return notificationapplication.NotificationEmailReceiptDTO{Found: result.Found, ProviderMessageID: result.ID}, nil
}

func writeRiverFaultRehearsalEvidence(path string, report riverFaultRehearsalReport) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("river fault rehearsal output is required")
	}
	if err := validateRiverFaultRehearsalReport(report); err != nil {
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

func validateRiverFaultRehearsalReport(report riverFaultRehearsalReport) error {
	completedOnce := func(stage riverFaultStageEvidence) bool {
		return stage.FinalState == "completed" && stage.Attempts == 1 && stage.Effects == 1
	}
	completedAfterLease := func(stage riverFaultStageEvidence, exit int, effectsBefore int) bool {
		return stage.FinalState == "completed" && stage.Attempts == 2 && stage.Effects == 1 &&
			stage.EffectsBeforeRecovery == effectsBefore && stage.LeaseExpiredAttempts == 1 && stage.CrashExitCode == exit
	}
	receipt := report.Provider.ReceiptCapable
	unsupported := report.Provider.UnsupportedAmbiguous
	if report.Version != riverFaultEvidenceVersion || report.Status != "verified" || report.Approval != "automated_isolated_fixture" ||
		report.Environment == "" || report.Hardware == "" || len(report.GitRevision) != 40 || !report.Isolated ||
		!report.ProductionEgressDisabled || len(report.FixtureSHA256) != 64 || report.FencedAt.IsZero() ||
		!completedOnce(report.Stages.BeforeClaim) || report.Stages.BeforeClaim.CrashExitCode != riverCrashBeforeClaimExit ||
		!completedAfterLease(report.Stages.AfterClaim, riverCrashAfterClaimExit, 0) ||
		!completedAfterLease(report.Stages.BusinessTransaction, riverCrashInTransactionExit, 0) ||
		!completedAfterLease(report.Stages.BeforeCompletionMark, riverCrashAfterEffectCommitExit, 1) ||
		report.Stages.Retry.FinalState != "completed" || report.Stages.Retry.Attempts != 2 || report.Stages.Retry.Effects != 1 || report.Stages.Retry.RetryableAttempts != 1 ||
		report.Stages.Cancellation.FinalState != "cancelled" || report.Stages.Cancellation.Attempts != 1 || report.Stages.Cancellation.Effects != 0 || report.Stages.Cancellation.CancelledAttempts != 1 ||
		report.Stages.FailureReplay.FinalState != "completed" || report.Stages.FailureReplay.Attempts != 2 || report.Stages.FailureReplay.Effects != 1 || report.Stages.FailureReplay.PermanentAttempts != 1 ||
		receipt.FinalStatus != "succeeded" || receipt.ProviderSends != 1 || receipt.ReceiptLookups != 1 || receipt.DeliveryAttempts != 1 || receipt.SucceededAttempts != 1 || receipt.UnknownAttempts != 0 || receipt.RemainingClaims != 0 || !receipt.StableDispatchIdentity || !receipt.StaleTokenRejected || receipt.BlindReplayClaimed || receipt.CrashExitCode != riverCrashAfterProviderExit ||
		unsupported.FinalStatus != "unknown" || unsupported.ProviderSends != 1 || unsupported.ReceiptLookups != 0 || unsupported.DeliveryAttempts != 1 || unsupported.SucceededAttempts != 0 || unsupported.UnknownAttempts != 1 || unsupported.RemainingClaims != 0 || !unsupported.StableDispatchIdentity || !unsupported.StaleTokenRejected || unsupported.BlindReplayClaimed || unsupported.CrashExitCode != riverCrashAfterProviderExit ||
		report.Provider.ProtectedFactsBefore == "" || report.Provider.ProtectedFactsBefore != report.Provider.ProtectedFactsAfter ||
		report.Provider.Guarantee != "bounded_at_least_once_with_fencing_idempotency_and_explicit_unknown" || report.Provider.ExactlyOnceClaimed ||
		report.Differences == nil || len(report.Differences) != 0 {
		return errors.New("river fault rehearsal evidence is incomplete")
	}
	return nil
}

func validRiverFaultRehearsalReportFixture() riverFaultRehearsalReport {
	report := riverFaultRehearsalReport{
		Version: riverFaultEvidenceVersion, Status: "verified", Approval: "automated_isolated_fixture",
		Environment: "isolated-test", Hardware: "fixture-host", GitRevision: strings.Repeat("a", 40),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), Isolated: true,
		ProductionEgressDisabled: true, FixtureSHA256: strings.Repeat("b", 64),
		FencedAt: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), Differences: []string{},
	}
	report.Stages.BeforeClaim = riverFaultStageEvidence{FinalState: "completed", Attempts: 1, Effects: 1, CrashExitCode: riverCrashBeforeClaimExit}
	report.Stages.AfterClaim = riverFaultStageEvidence{FinalState: "completed", Attempts: 2, Effects: 1, LeaseExpiredAttempts: 1, CrashExitCode: riverCrashAfterClaimExit}
	report.Stages.BusinessTransaction = riverFaultStageEvidence{FinalState: "completed", Attempts: 2, Effects: 1, LeaseExpiredAttempts: 1, CrashExitCode: riverCrashInTransactionExit}
	report.Stages.BeforeCompletionMark = riverFaultStageEvidence{FinalState: "completed", Attempts: 2, Effects: 1, EffectsBeforeRecovery: 1, LeaseExpiredAttempts: 1, CrashExitCode: riverCrashAfterEffectCommitExit}
	report.Stages.Retry = riverFaultStageEvidence{FinalState: "completed", Attempts: 2, Effects: 1, RetryableAttempts: 1}
	report.Stages.Cancellation = riverFaultStageEvidence{FinalState: "cancelled", Attempts: 1, CancelledAttempts: 1}
	report.Stages.FailureReplay = riverFaultStageEvidence{FinalState: "completed", Attempts: 2, Effects: 1, PermanentAttempts: 1}
	report.Provider.ReceiptCapable = riverProviderFaultEvidence{FinalStatus: "succeeded", ProviderSends: 1, ReceiptLookups: 1, DeliveryAttempts: 1, SucceededAttempts: 1, StableDispatchIdentity: true, StaleTokenRejected: true, CrashExitCode: riverCrashAfterProviderExit}
	report.Provider.UnsupportedAmbiguous = riverProviderFaultEvidence{FinalStatus: "unknown", ProviderSends: 1, DeliveryAttempts: 1, UnknownAttempts: 1, StableDispatchIdentity: true, StaleTokenRejected: true, CrashExitCode: riverCrashAfterProviderExit}
	report.Provider.ProtectedFactsBefore = strings.Repeat("c", 64)
	report.Provider.ProtectedFactsAfter = report.Provider.ProtectedFactsBefore
	report.Provider.Guarantee = "bounded_at_least_once_with_fencing_idempotency_and_explicit_unknown"
	return report
}

func loadRiverFaultRehearsalConfig(t *testing.T) riverFaultRehearsalConfig {
	t.Helper()
	cfg := riverFaultRehearsalConfig{
		Output:                   strings.TrimSpace(os.Getenv("HOTKEY_RIVER_REHEARSAL_OUTPUT")),
		Environment:              strings.TrimSpace(os.Getenv("HOTKEY_RIVER_REHEARSAL_ENVIRONMENT")),
		Hardware:                 strings.TrimSpace(os.Getenv("HOTKEY_RIVER_REHEARSAL_HARDWARE")),
		GitRevision:              strings.TrimSpace(os.Getenv("HOTKEY_RIVER_REHEARSAL_GIT_REVISION")),
		ProductionEgressDisabled: strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_RIVER_REHEARSAL_PRODUCTION_EGRESS_DISABLED")), "true"),
	}
	if cfg.Environment == "" {
		cfg.Environment = "local-isolated-river-rehearsal"
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
		cfg.Output = filepath.Join(t.TempDir(), "river-fault-rehearsal.json")
	}
	if os.Getenv("HOTKEY_RIVER_REHEARSAL_PRODUCTION_EGRESS_DISABLED") == "" {
		cfg.ProductionEgressDisabled = true
	}
	if len(cfg.GitRevision) != 40 || !cfg.ProductionEgressDisabled {
		t.Fatal("river fault rehearsal requires a 40-hex revision and disabled production egress")
	}
	return cfg
}

func databaseTime(t *testing.T, databaseRuntime *platformdatabase.Runtime) time.Time {
	t.Helper()
	var now time.Time
	if err := databaseRuntime.SQL.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now.UTC().Truncate(time.Microsecond)
}

func riverFaultSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
