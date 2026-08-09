package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestIntentRepositoryPersistsNormalizedDraftAndIdempotentCandidateReview(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, true)
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := repository.Find(context.Background(), monitorapplication.ReadIntentDraftQuery{MonitorID: fixture.monitorID, DraftID: fixture.draftID})
	if err != nil {
		t.Fatalf("Find(): %v", err)
	}
	if loaded.ResourceVersion != 1 || loaded.Objective != "Track launch disruption" || len(loaded.Clauses) != 2 ||
		len(loaded.Entities) != 1 || len(loaded.Entities[0].Aliases) != 2 || len(loaded.Examples) != 2 || len(loaded.Candidates) != 1 {
		t.Fatalf("loaded normalized draft = %#v", loaded)
	}
	reviewedAt := time.Now().UTC().Truncate(time.Microsecond)
	next := loaded
	next.ResourceVersion = 2
	next.Candidates = append([]monitorapplication.ExpansionCandidateDTO(nil), loaded.Candidates...)
	next.Candidates[0].ApprovalStatus = "approved"
	next.Candidates[0].ReviewerUserID = &fixture.actorID
	next.Candidates[0].ReviewedAt = &reviewedAt
	next.Candidates[0].ReviewNote = "approved after operator review"
	mutation := monitorapplication.IntentDraftMutationDTO{
		Kind: monitorapplication.IntentDraftMutationCandidateReview, ExpectedDraftID: fixture.draftID,
		ExpectedResourceVersion: 1, Next: next, InvalidatedAt: reviewedAt,
		IdempotencyKey: "review.launch-synonym", CommandFingerprint: strings.Repeat("c", 64),
	}
	first, err := repository.SaveAndInvalidateRuns(context.Background(), mutation)
	if err != nil {
		t.Fatalf("SaveAndInvalidateRuns(): %v", err)
	}
	if !first.Created || first.Draft.ResourceVersion != 2 || first.Draft.Candidates[0].ApprovalStatus != "approved" {
		t.Fatalf("first receipt = %#v", first)
	}
	replay, err := repository.SaveAndInvalidateRuns(context.Background(), mutation)
	if err != nil || replay.Created || replay.Draft.ResourceVersion != 2 || replay.CommandFingerprint != mutation.CommandFingerprint {
		t.Fatalf("replay = %#v / %v", replay, err)
	}
	conflict := mutation
	conflict.CommandFingerprint = strings.Repeat("d", 64)
	if _, err := repository.SaveAndInvalidateRuns(context.Background(), conflict); !errors.Is(err, monitorapplication.ErrIntentIdempotencyConflict) {
		t.Fatalf("same key different input error = %v", err)
	}

	var revisions, receipts int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_intent_draft_revisions WHERE draft_id=$1`, fixture.draftID).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_intent_mutation_receipts WHERE draft_id=$1`, fixture.draftID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 || receipts != 1 {
		t.Fatalf("revision/receipt counts = %d/%d", revisions, receipts)
	}
	historical, err := repository.FindIntentDraftRevision(context.Background(), monitorapplication.ReadIntentDraftRevisionQuery{
		MonitorID: fixture.monitorID, DraftID: fixture.draftID, ResourceVersion: 1,
	})
	if err != nil || historical.ResourceVersion != 1 || historical.Candidates[0].ApprovalStatus != "pending" {
		t.Fatalf("immutable historical revision = %#v / %v", historical, err)
	}
}

func TestIntentRepositoryAtomicallyQueuesAndCompletesPreviewWithHistoricalVisibility(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, false)
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	request := intentPreviewReservation(fixture, now)
	reserved, err := repository.ReserveAndEnqueue(context.Background(), request)
	if err != nil {
		t.Fatalf("ReserveAndEnqueue(): %v", err)
	}
	if !reserved.Created || reserved.Run.Status != "queued" || reserved.Run.ID <= 0 {
		t.Fatalf("reservation = %#v", reserved)
	}
	task, err := repository.FindIntentAnalysisTask(context.Background(), monitorapplication.ReadIntentAnalysisTaskQuery{
		RunID: reserved.Run.ID, DraftID: fixture.draftID, DraftResourceVersion: 1,
	})
	if err != nil || task.Run.MonitorID != fixture.monitorID || task.Run.InputHash != request.Task.InputHash || task.AnalysisProfile != request.Task.AnalysisProfile || task.SampleLimit != request.Task.SampleLimit {
		t.Fatalf("durable analysis task = %#v / %v", task, err)
	}
	if _, err := repository.FindIntentAnalysisTask(context.Background(), monitorapplication.ReadIntentAnalysisTaskQuery{
		RunID: reserved.Run.ID, DraftID: fixture.draftID + 1, DraftResourceVersion: 1,
	}); !errors.Is(err, monitorapplication.ErrIntentRunNotFound) {
		t.Fatalf("cross-draft durable task error = %v", err)
	}
	replay, err := repository.ReserveAndEnqueue(context.Background(), request)
	if err != nil || replay.Created || replay.Run.ID != reserved.Run.ID {
		t.Fatalf("reservation replay = %#v / %v", replay, err)
	}
	changedRequest := request
	changedRequest.RequestHash = strings.Repeat("9", 64)
	if _, err := repository.ReserveAndEnqueue(context.Background(), changedRequest); !errors.Is(err, monitorapplication.ErrIntentIdempotencyConflict) {
		t.Fatalf("reservation conflict error = %v", err)
	}

	startedAt := now.Add(time.Second)
	running := reserved.Run
	running.Status, running.StartedAt = "running", &startedAt
	started, err := repository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: reserved.Run, Next: running})
	if err != nil || !started.Changed {
		t.Fatalf("SaveTransition(start) = %#v / %v", started, err)
	}
	completedAt := startedAt.Add(time.Second)
	succeeded := running
	succeeded.Status, succeeded.CompletedAt = "succeeded", &completedAt
	previewMutation := monitorapplication.CompletePreviewRunMutationDTO{
		Transition:        monitorapplication.IntentRunTransitionDTO{Expected: running, Next: succeeded},
		Preview:           monitorapplication.IntentPreviewDTO{EstimatedAlertCount: 3, Warnings: []string{"sparse historical sample"}},
		ResultFingerprint: strings.Repeat("e", 64),
	}
	completed, err := repository.CompletePreview(context.Background(), previewMutation)
	if err != nil {
		t.Fatalf("CompletePreview(): %v", err)
	}
	if !completed.Changed || completed.Preview.Preview == nil || completed.Preview.Preview.EstimatedAlertCount != 3 {
		t.Fatalf("preview completion = %#v", completed)
	}
	duplicate, err := repository.CompletePreview(context.Background(), previewMutation)
	if err != nil || duplicate.Changed || duplicate.Preview.Preview == nil {
		t.Fatalf("preview replay = %#v / %v", duplicate, err)
	}

	current, err := repository.Find(context.Background(), monitorapplication.ReadIntentDraftQuery{MonitorID: fixture.monitorID, DraftID: fixture.draftID})
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.ResourceVersion++
	next.Objective = "Track launch disruption and recovery"
	_, err = repository.SaveAndInvalidateRuns(context.Background(), monitorapplication.IntentDraftMutationDTO{
		Kind: monitorapplication.IntentDraftMutationReplace, ExpectedDraftID: fixture.draftID,
		ExpectedResourceVersion: current.ResourceVersion, Next: next, InvalidatedAt: completedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("replace draft: %v", err)
	}
	historical, err := repository.FindPreview(context.Background(), monitorapplication.ReadPreviewRunQuery{
		MonitorID: fixture.monitorID, DraftID: fixture.draftID, DraftResourceVersion: 1, RunID: reserved.Run.ID,
	})
	if err != nil || historical.Run.Status != "invalidated" || historical.Preview == nil || historical.Preview.EstimatedAlertCount != 3 {
		t.Fatalf("historical preview = %#v / %v", historical, err)
	}
	staleRequest := request
	staleRequest.IdempotencyKey, staleRequest.RequestHash = "preview.stale-version", strings.Repeat("7", 64)
	if _, err := repository.ReserveAndEnqueue(context.Background(), staleRequest); !errors.Is(err, monitorapplication.ErrIntentVersionConflict) {
		t.Fatalf("stale draft reservation error = %v", err)
	}
	var jobs, runs int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM river_job WHERE kind='analyze_monitor_intent'`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_intent_analysis_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || runs != 1 {
		t.Fatalf("job/run counts = %d/%d", jobs, runs)
	}
}

func TestIntentRepositoryCompletesExpansionAndInvalidatesOtherOldRunsInOneTransaction(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, false)
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	inputHash := strings.Repeat("a", 64)
	expansionRequest := monitorapplication.ReserveIntentRunDTO{
		IdempotencyKey: "expand.launch", RequestHash: strings.Repeat("1", 64), RequestedAt: now,
		Task: monitorapplication.IntentRunTaskDTO{Kind: "expansion", MonitorID: fixture.monitorID, DraftID: fixture.draftID, DraftResourceVersion: 1, InputHash: inputHash, AnalysisProfile: monitorapplication.IntentExpansionProfile},
	}
	expansion, err := repository.ReserveAndEnqueue(context.Background(), expansionRequest)
	if err != nil {
		t.Fatal(err)
	}
	previewRequest := intentPreviewReservation(fixture, now)
	previewRequest.IdempotencyKey, previewRequest.RequestHash = "preview.old", strings.Repeat("2", 64)
	oldPreview, err := repository.ReserveAndEnqueue(context.Background(), previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := now.Add(time.Second)
	running := expansion.Run
	running.Status, running.StartedAt = "running", &startedAt
	if _, err := repository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: expansion.Run, Next: running}); err != nil {
		t.Fatal(err)
	}
	completedAt, invalidatedAt := startedAt.Add(time.Second), startedAt.Add(2*time.Second)
	finished := running
	finished.Status, finished.CompletedAt, finished.InvalidatedAt = "invalidated", &completedAt, &invalidatedAt
	current, err := repository.Find(context.Background(), monitorapplication.ReadIntentDraftQuery{MonitorID: fixture.monitorID, DraftID: fixture.draftID})
	if err != nil {
		t.Fatal(err)
	}
	candidate := monitorapplication.ExpansionCandidateDTO{
		ID: "launch-disruption", Value: "launch outage", Source: "llm", Reason: "semantic expansion",
		ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: inputHash,
		Similarity: 0.82, Risk: "medium", ApprovalStatus: "pending",
	}
	next := current
	next.ResourceVersion = 2
	next.Candidates = []monitorapplication.ExpansionCandidateDTO{candidate}
	mutation := monitorapplication.CompleteExpansionRunMutationDTO{
		Transition: monitorapplication.IntentRunTransitionDTO{Expected: running, Next: finished},
		DraftMutation: monitorapplication.IntentDraftMutationDTO{
			Kind: monitorapplication.IntentDraftMutationExpansionResult, ExpectedDraftID: fixture.draftID,
			ExpectedResourceVersion: 1, Next: next, InvalidatedAt: invalidatedAt,
		},
		Candidates: []monitorapplication.ExpansionCandidateDTO{candidate}, ResultFingerprint: strings.Repeat("f", 64),
	}
	completed, err := repository.CompleteExpansion(context.Background(), mutation)
	if err != nil {
		t.Fatalf("CompleteExpansion(): %v", err)
	}
	if !completed.Changed || completed.Draft.ResourceVersion != 2 || completed.Expansion.Run.Status != "invalidated" || len(completed.Expansion.Candidates) != 1 {
		t.Fatalf("expansion completion = %#v", completed)
	}
	replay, err := repository.CompleteExpansion(context.Background(), mutation)
	if err != nil || replay.Changed || replay.Draft.ResourceVersion != 2 {
		t.Fatalf("expansion replay = %#v / %v", replay, err)
	}
	stalePreview, err := repository.FindPreview(context.Background(), monitorapplication.ReadPreviewRunQuery{
		MonitorID: fixture.monitorID, DraftID: fixture.draftID, DraftResourceVersion: 1, RunID: oldPreview.Run.ID,
	})
	if err != nil || stalePreview.Run.Status != "invalidated" || stalePreview.Preview != nil {
		t.Fatalf("stale preview = %#v / %v", stalePreview, err)
	}
}

func TestIntentRepositoryConcurrentReservationAndCompletionRollbackRemainConsistent(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, false)
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	request := intentPreviewReservation(fixture, now)

	const callers = 6
	results := make(chan monitorapplication.IntentRunReservationDTO, callers)
	errorsChannel := make(chan error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, reserveErr := repository.ReserveAndEnqueue(context.Background(), request)
			results <- result
			errorsChannel <- reserveErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	for reserveErr := range errorsChannel {
		if reserveErr != nil {
			t.Fatalf("concurrent ReserveAndEnqueue(): %v", reserveErr)
		}
	}
	created, runID := 0, int64(0)
	var reserved monitorapplication.IntentRunDTO
	for result := range results {
		if result.Created {
			created++
		}
		if runID == 0 {
			runID, reserved = result.Run.ID, result.Run
		} else if result.Run.ID != runID {
			t.Fatalf("concurrent reservations returned run IDs %d and %d", runID, result.Run.ID)
		}
	}
	if created != 1 {
		t.Fatalf("created reservation count = %d, want 1", created)
	}
	var durableArgs string
	if err := runtime.SQL.QueryRow(`SELECT args::text FROM river_job WHERE kind='analyze_monitor_intent'`).Scan(&durableArgs); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"body", "raw", "markdown", "objective", "candidate"} {
		if strings.Contains(strings.ToLower(durableArgs), forbidden) {
			t.Fatalf("durable args leaked %q: %s", forbidden, durableArgs)
		}
	}
	var identity map[string]json.RawMessage
	if err := json.Unmarshal([]byte(durableArgs), &identity); err != nil || len(identity) != 3 || identity["run_id"] == nil || identity["draft_id"] == nil || identity["draft_resource_version"] == nil {
		t.Fatalf("durable args identity = %v / %v", identity, err)
	}
	for _, forbidden := range []string{"kind", "monitor_id", "input_hash", "profile_version", "analysis_profile", "sample_limit"} {
		if identity[forbidden] != nil {
			t.Fatalf("durable args trusted repository fact %q: %s", forbidden, durableArgs)
		}
	}

	startedAt := now.Add(time.Second)
	running := reserved
	running.Status, running.StartedAt = "running", &startedAt
	if _, err := repository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: reserved, Next: running}); err != nil {
		t.Fatal(err)
	}
	completedAt := startedAt.Add(time.Second)
	commitErr := runtime.WithinTransaction(context.Background(), func(ctx context.Context, transaction database.Transaction) error {
		_, updateErr := transaction.SQL.ExecContext(ctx, `
UPDATE monitor_intent_analysis_runs
SET status='succeeded',completed_at=$2,result_fingerprint=$3
WHERE id=$1`, runID, completedAt, strings.Repeat("4", 64))
		return updateErr
	})
	if commitErr == nil {
		t.Fatal("successful preview without normalized result committed")
	}
	failedCompletion := monitorapplication.CompletePreviewRunMutationDTO{
		Transition: monitorapplication.IntentRunTransitionDTO{Expected: running, Next: func() monitorapplication.IntentRunDTO {
			next := running
			next.Status, next.CompletedAt = "succeeded", &completedAt
			return next
		}()},
		Preview: monitorapplication.IntentPreviewDTO{Samples: []monitorapplication.PreviewSampleDTO{
			{DocumentVersionID: 999999999, Title: "missing immutable document", Decision: "accepted"},
		}},
		ResultFingerprint: strings.Repeat("5", 64),
	}
	if _, err := repository.CompletePreview(context.Background(), failedCompletion); err == nil {
		t.Fatal("preview completion with missing document version succeeded")
	}
	stored, err := repository.FindPreview(context.Background(), monitorapplication.ReadPreviewRunQuery{
		MonitorID: fixture.monitorID, DraftID: fixture.draftID, DraftResourceVersion: 1, RunID: runID,
	})
	if err != nil || stored.Run.Status != "running" || stored.Preview != nil {
		t.Fatalf("run after failed completion = %#v / %v", stored, err)
	}

	failureRequest := monitorapplication.ReserveIntentRunDTO{
		IdempotencyKey: "expand.failure", RequestHash: strings.Repeat("6", 64), RequestedAt: now,
		Task: monitorapplication.IntentRunTaskDTO{Kind: "expansion", MonitorID: fixture.monitorID, DraftID: fixture.draftID, DraftResourceVersion: 1, InputHash: strings.Repeat("a", 64), AnalysisProfile: monitorapplication.IntentExpansionProfile},
	}
	failureRun, err := repository.ReserveAndEnqueue(context.Background(), failureRequest)
	if err != nil {
		t.Fatal(err)
	}
	failed := failureRun.Run
	failed.Status, failed.CompletedAt, failed.FailureReason = "failed", &completedAt, "provider unavailable"
	transition := monitorapplication.IntentRunTransitionDTO{Expected: failureRun.Run, Next: failed}
	firstFailure, err := repository.SaveTransition(context.Background(), transition)
	if err != nil || !firstFailure.Changed {
		t.Fatalf("first failure transition = %#v / %v", firstFailure, err)
	}
	replayedFailure, err := repository.SaveTransition(context.Background(), transition)
	if err != nil || replayedFailure.Changed || replayedFailure.Run.FailureReason != failed.FailureReason {
		t.Fatalf("replayed failure transition = %#v / %v", replayedFailure, err)
	}
}

type intentRepositoryFixture struct {
	actorID, monitorID, configID, draftID int64
}

func intentRepositoryRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("InitializeEmpty(): %v", err)
	}
	return runtime
}

func insertIntentRepositoryDraft(t *testing.T, runtime *database.Runtime, withCandidate bool) intentRepositoryFixture {
	t.Helper()
	var fixture intentRepositoryFixture
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role,status)
VALUES ($1,'not-a-credential','Intent reviewer','admin','active') RETURNING id`,
		"intent-reviewer-"+strings.Repeat("x", 8)+"@example.test").Scan(&fixture.actorID); err != nil {
		t.Fatalf("insert actor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name,status) VALUES ($1,'draft') RETURNING id`, "intent repository monitor").Scan(&fixture.monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_config_versions (monitor_id,revision,state) VALUES ($1,1,'draft') RETURNING id`, fixture.monitorID).Scan(&fixture.configID); err != nil {
		t.Fatalf("insert config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=$2 WHERE id=$1`, fixture.monitorID, fixture.configID); err != nil {
		t.Fatalf("bind config: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_drafts (monitor_id,config_version_id) VALUES ($1,$2) RETURNING id`, fixture.monitorID, fixture.configID).Scan(&fixture.draftID); err != nil {
		t.Fatalf("insert intent draft: %v", err)
	}
	var revisionID, entityID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective)
VALUES ($1,$2,$3,1,'Track launch disruption') RETURNING id`, fixture.draftID, fixture.monitorID, fixture.configID).Scan(&revisionID); err != nil {
		t.Fatalf("insert revision: %v", err)
	}
	for ordinal, clause := range []monitorapplication.IntentClauseDTO{
		{Operator: "must", Field: "action", Value: "launch"},
		{Operator: "must_not", Field: "location", Value: "test environment"},
	} {
		if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_intent_clauses (revision_id,draft_id,resource_version,ordinal,operator,field,value)
VALUES ($1,$2,1,$3,$4,$5,$6)`, revisionID, fixture.draftID, ordinal, clause.Operator, clause.Field, clause.Value); err != nil {
			t.Fatalf("insert clause: %v", err)
		}
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_entities (revision_id,draft_id,resource_version,ordinal,canonical_id,display_name,ambiguity_note)
VALUES ($1,$2,1,0,'product:hotkey','HotKey','product, not keyboard shortcut') RETURNING id`, revisionID, fixture.draftID).Scan(&entityID); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	for ordinal, alias := range []string{"Hot Key", "热点键"} {
		if _, err := runtime.SQL.Exec(`INSERT INTO monitor_intent_entity_aliases (entity_id,draft_id,resource_version,ordinal,alias) VALUES ($1,$2,1,$3,$4)`, entityID, fixture.draftID, ordinal, alias); err != nil {
			t.Fatalf("insert alias: %v", err)
		}
	}
	for ordinal, example := range []monitorapplication.IntentExampleDTO{{Label: "positive", Text: "The product launch is unavailable"}, {Label: "negative", Text: "A keyboard shortcut tutorial"}} {
		if _, err := runtime.SQL.Exec(`INSERT INTO monitor_intent_examples (revision_id,draft_id,resource_version,ordinal,label,example_text) VALUES ($1,$2,1,$3,$4,$5)`, revisionID, fixture.draftID, ordinal, example.Label, example.Text); err != nil {
			t.Fatalf("insert example: %v", err)
		}
	}
	if withCandidate {
		var candidateRecordID int64
		if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_expansion_candidates (
  draft_id,introduced_resource_version,candidate_id,candidate_value,source,reason,model_version,prompt_version,input_hash,similarity,risk
) VALUES ($1,1,'launch-synonym','release outage','user_input','operator synonym','','',$2,0.9,'low') RETURNING id`,
			fixture.draftID, strings.Repeat("a", 64)).Scan(&candidateRecordID); err != nil {
			t.Fatalf("insert candidate: %v", err)
		}
		if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_intent_draft_candidates (revision_id,draft_id,resource_version,candidate_record_id,ordinal,approval_status)
VALUES ($1,$2,1,$3,0,'pending')`, revisionID, fixture.draftID, candidateRecordID); err != nil {
			t.Fatalf("attach candidate: %v", err)
		}
	}
	return fixture
}

func intentPreviewReservation(fixture intentRepositoryFixture, now time.Time) monitorapplication.ReserveIntentRunDTO {
	return monitorapplication.ReserveIntentRunDTO{
		IdempotencyKey: "preview.launch", RequestHash: strings.Repeat("8", 64), RequestedAt: now,
		Task: monitorapplication.IntentRunTaskDTO{
			Kind: "preview", MonitorID: fixture.monitorID, DraftID: fixture.draftID, DraftResourceVersion: 1,
			InputHash: strings.Repeat("a", 64), AnalysisProfile: "preview-v1", SampleLimit: 25,
		},
	}
}
