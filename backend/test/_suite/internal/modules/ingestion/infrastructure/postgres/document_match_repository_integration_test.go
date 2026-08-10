package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestDocumentMatchRepositoryPersistsAtomicExactVersionBatchAndReplays(t *testing.T) {
	runtime := openDocumentMatchRuntime(t)
	defer runtime.Close()
	fixture := createDocumentMatchFixture(t, runtime, "batch")
	repository, err := NewDocumentMatchRepository(runtime)
	if err != nil {
		t.Fatalf("NewDocumentMatchRepository(): %v", err)
	}

	first := documentMatchCommand(fixture, fixture.firstDocumentVersionID, 'a')
	second := documentMatchCommand(fixture, fixture.secondDocumentVersionID, 'b')
	stored, err := repository.PersistAutomaticDocumentMatches(context.Background(), []ingestionapplication.PersistAutomaticDocumentMatchCommand{first, second})
	if err != nil {
		t.Fatalf("PersistAutomaticDocumentMatches(): %v", err)
	}
	if len(stored) != 2 || stored[0].DocumentVersionID != first.DocumentVersionID || stored[1].DocumentVersionID != second.DocumentVersionID {
		t.Fatalf("stored decisions = %#v", stored)
	}
	if stored[0].Decision != "review" || !stored[0].Degraded || stored[0].RelevanceProbability != nil || len(stored[0].Signals) != 1 {
		t.Fatalf("stored review projection = %#v", stored[0])
	}

	replayed := first
	replayed.DecidedAt = first.DecidedAt.Add(time.Hour)
	again, err := repository.PersistAutomaticDocumentMatches(context.Background(), []ingestionapplication.PersistAutomaticDocumentMatchCommand{replayed})
	if err != nil {
		t.Fatalf("PersistAutomaticDocumentMatches(replay): %v", err)
	}
	if len(again) != 1 || again[0].ID != stored[0].ID || !again[0].CreatedAt.Equal(stored[0].CreatedAt) {
		t.Fatalf("replay = %#v, want first immutable decision %#v", again, stored[0])
	}
	conflict := first
	conflict.InputHash = strings.Repeat("c", 64)
	if _, err := repository.PersistAutomaticDocumentMatches(context.Background(), []ingestionapplication.PersistAutomaticDocumentMatchCommand{conflict}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("conflicting replay error = %v, want conflict", err)
	}

	var decisionCount, signalCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_match_decisions WHERE monitor_version_id=$1`, fixture.monitorVersionID).Scan(&decisionCount); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_match_recall_signals`).Scan(&signalCount); err != nil {
		t.Fatalf("count signals: %v", err)
	}
	if decisionCount != 2 || signalCount != 2 {
		t.Fatalf("decision/signal count = %d/%d, want 2/2", decisionCount, signalCount)
	}
	firstPage, err := repository.ListDocumentMatches(context.Background(), ingestionapplication.ListDocumentMatchesQuery{
		ActorUserID: fixture.viewerUserID, MonitorID: fixture.monitorID, EffectiveDecision: "review", Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListDocumentMatches(first page): %v", err)
	}
	if len(firstPage.Items) != 1 || firstPage.NextCursor == "" || firstPage.Items[0].EffectiveDecision != "review" ||
		len(firstPage.Items[0].Automatic.Signals) != 1 {
		t.Fatalf("first document match page = %#v", firstPage)
	}
	secondPage, err := repository.ListDocumentMatches(context.Background(), ingestionapplication.ListDocumentMatchesQuery{
		ActorUserID: fixture.viewerUserID, MonitorID: fixture.monitorID, EffectiveDecision: "review", Cursor: firstPage.NextCursor, Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListDocumentMatches(second page): %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.NextCursor != "" || secondPage.Items[0].Automatic.ID == firstPage.Items[0].Automatic.ID {
		t.Fatalf("second document match page = %#v", secondPage)
	}
}

func TestDocumentMatchRepositoryRollsBackBatchAndSchemaRejectsMutableOrPrematureAcceptance(t *testing.T) {
	runtime := openDocumentMatchRuntime(t)
	defer runtime.Close()
	fixture := createDocumentMatchFixture(t, runtime, "rollback")
	repository, err := NewDocumentMatchRepository(runtime)
	if err != nil {
		t.Fatalf("NewDocumentMatchRepository(): %v", err)
	}
	valid := documentMatchCommand(fixture, fixture.firstDocumentVersionID, 'd')
	missing := documentMatchCommand(fixture, fixture.secondDocumentVersionID+999999, 'e')
	if _, err := repository.PersistAutomaticDocumentMatches(context.Background(), []ingestionapplication.PersistAutomaticDocumentMatchCommand{valid, missing}); err == nil {
		t.Fatal("batch with missing DocumentVersion succeeded")
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_match_decisions WHERE monitor_version_id=$1`, fixture.monitorVersionID).Scan(&count); err != nil {
		t.Fatalf("count rolled-back decisions: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back decision count = %d, want 0", count)
	}

	stored, err := repository.PersistAutomaticDocumentMatches(context.Background(), []ingestionapplication.PersistAutomaticDocumentMatchCommand{valid})
	if err != nil {
		t.Fatalf("persist valid decision: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE document_match_decisions SET decision='rejected' WHERE id=$1`, stored[0].ID); err == nil {
		t.Fatal("append-only automatic decision was updated")
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO document_match_decisions (
 monitor_id,monitor_version_id,compiled_profile_id,document_version_id,relevance_profile_id,
 matching_algorithm_version,reranker_version,calibration_version,rrf_score,relevance_probability,
 decision,degraded,reason_codes,input_hash,decided_at
) VALUES ($1,$2,$3,$4,$5,'rrf-k60-v1','cross-encoder-v1','uncalibrated-v1',0.02,0.99,
          'accepted',false,ARRAY['calibrated_accept'],repeat('f',64),now())`,
		fixture.monitorID, fixture.monitorVersionID, fixture.compiledProfileID,
		fixture.secondDocumentVersionID, fixture.relevanceProfileID); err == nil {
		t.Fatal("uncalibrated profile inserted accepted automatic decision")
	}
}

func TestDocumentMatchRepositoryAppendsIdempotentOverrideChainAndAuthorizesDurableActor(t *testing.T) {
	runtime := openDocumentMatchRuntime(t)
	defer runtime.Close()
	fixture := createDocumentMatchFixture(t, runtime, "override")
	repository, err := NewDocumentMatchRepository(runtime)
	if err != nil {
		t.Fatalf("NewDocumentMatchRepository(): %v", err)
	}
	stored, err := repository.PersistAutomaticDocumentMatches(context.Background(), []ingestionapplication.PersistAutomaticDocumentMatchCommand{
		documentMatchCommand(fixture, fixture.firstDocumentVersionID, '1'),
	})
	if err != nil {
		t.Fatalf("persist automatic decision: %v", err)
	}
	decisionID := stored[0].ID
	if err := repository.AuthorizeDocumentMatchReview(context.Background(), ingestionapplication.AuthorizeDocumentMatchReviewQuery{
		ActorUserID: fixture.editorUserID, MonitorID: fixture.monitorID, MatchDecisionID: decisionID,
	}); err != nil {
		t.Fatalf("AuthorizeDocumentMatchReview(editor): %v", err)
	}
	if err := repository.AuthorizeDocumentMatchReview(context.Background(), ingestionapplication.AuthorizeDocumentMatchReviewQuery{
		ActorUserID: fixture.viewerUserID, MonitorID: fixture.monitorID, MatchDecisionID: decisionID,
	}); !errors.Is(err, ingestionapplication.ErrDocumentMatchAuthorizationDenied) {
		t.Fatalf("viewer authorization error = %v", err)
	}
	if _, _, err := repository.AppendDocumentMatchOverride(context.Background(), ingestionapplication.AppendDocumentMatchOverrideCommand{
		ActorUserID: fixture.viewerUserID, MonitorID: fixture.monitorID, MatchDecisionID: decisionID,
		Decision: "accepted", ReasonCode: "manual_relevant", IdempotencyKey: "viewer-must-not-write",
		CommandFingerprint: strings.Repeat("5", 64), DecidedAt: time.Now().UTC().Truncate(time.Microsecond),
	}); !errors.Is(err, ingestionapplication.ErrDocumentMatchAuthorizationDenied) {
		t.Fatalf("direct viewer append error = %v, want authorization denied", err)
	}

	first := ingestionapplication.AppendDocumentMatchOverrideCommand{
		ActorUserID: fixture.editorUserID, MonitorID: fixture.monitorID, MatchDecisionID: decisionID,
		Decision: "accepted", ReasonCode: "manual_relevant", Note: "matches the published intent",
		IdempotencyKey:     "document-match-override-first-" + fmt.Sprint(decisionID),
		CommandFingerprint: strings.Repeat("2", 64), DecidedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	override, reused, err := repository.AppendDocumentMatchOverride(context.Background(), first)
	if err != nil || reused {
		t.Fatalf("AppendDocumentMatchOverride(first) = %#v reused=%t err=%v", override, reused, err)
	}
	if override.Sequence != 1 || override.PreviousEffectiveDecision != "review" || override.Decision != "accepted" {
		t.Fatalf("first override = %#v", override)
	}
	replay, reused, err := repository.AppendDocumentMatchOverride(context.Background(), first)
	if err != nil || !reused || replay.ID != override.ID {
		t.Fatalf("AppendDocumentMatchOverride(replay) = %#v reused=%t err=%v", replay, reused, err)
	}
	conflict := first
	conflict.CommandFingerprint = strings.Repeat("3", 64)
	if _, _, err := repository.AppendDocumentMatchOverride(context.Background(), conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("override idempotency conflict error = %v", err)
	}
	stale := first
	stale.Decision = "rejected"
	stale.ReasonCode = "manual_irrelevant"
	stale.IdempotencyKey = "document-match-override-stale-" + fmt.Sprint(decisionID)
	stale.CommandFingerprint = strings.Repeat("6", 64)
	if _, _, err := repository.AppendDocumentMatchOverride(context.Background(), stale); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale override sequence error = %v, want conflict", err)
	}
	second := first
	second.ExpectedSequence = 1
	second.Decision = "rejected"
	second.ReasonCode = "manual_irrelevant"
	second.IdempotencyKey = "document-match-override-second-" + fmt.Sprint(decisionID)
	second.CommandFingerprint = strings.Repeat("4", 64)
	second.DecidedAt = first.DecidedAt.Add(time.Minute)
	next, reused, err := repository.AppendDocumentMatchOverride(context.Background(), second)
	if err != nil || reused || next.Sequence != 2 || next.PreviousEffectiveDecision != "accepted" {
		t.Fatalf("AppendDocumentMatchOverride(second) = %#v reused=%t err=%v", next, reused, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE document_match_overrides SET note='mutated' WHERE id=$1`, next.ID); err == nil {
		t.Fatal("append-only override was updated")
	}
	page, err := repository.ListDocumentMatches(context.Background(), ingestionapplication.ListDocumentMatchesQuery{
		ActorUserID: fixture.viewerUserID, MonitorID: fixture.monitorID, EffectiveDecision: "rejected", Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListDocumentMatches(effective rejected): %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Automatic.ID != decisionID || page.Items[0].OverrideSequence != 2 ||
		page.Items[0].EffectiveDecision != "rejected" {
		t.Fatalf("effective rejected page = %#v", page)
	}
	if _, err := runtime.SQL.Exec(`UPDATE users SET status='disabled' WHERE id=$1`, fixture.viewerUserID); err != nil {
		t.Fatalf("disable viewer: %v", err)
	}
	if _, err := repository.ListDocumentMatches(context.Background(), ingestionapplication.ListDocumentMatchesQuery{
		ActorUserID: fixture.viewerUserID, MonitorID: fixture.monitorID, Limit: 10,
	}); !errors.Is(err, ingestionapplication.ErrDocumentMatchAuthorizationDenied) {
		t.Fatalf("disabled viewer list error = %v, want authorization denied", err)
	}
}

func TestDocumentMatchRepositorySchedulesAcceptedProjectionAtomically(t *testing.T) {
	runtime := openDocumentMatchRuntime(t)
	defer runtime.Close()
	fixture := createDocumentMatchFixture(t, runtime, "accepted-projection")
	scheduler := &documentMatchProjectionSchedulerFake{err: errors.New("queue unavailable")}
	repository, err := NewDocumentMatchRepository(runtime, scheduler)
	if err != nil {
		t.Fatalf("NewDocumentMatchRepository(): %v", err)
	}
	stored, err := repository.PersistAutomaticDocumentMatches(context.Background(), []ingestionapplication.PersistAutomaticDocumentMatchCommand{
		documentMatchCommand(fixture, fixture.firstDocumentVersionID, '7'),
	})
	if err != nil {
		t.Fatalf("persist automatic decision: %v", err)
	}
	command := ingestionapplication.AppendDocumentMatchOverrideCommand{
		ActorUserID: fixture.editorUserID, MonitorID: fixture.monitorID, MatchDecisionID: stored[0].ID,
		Decision: "accepted", ReasonCode: "manual_relevant", IdempotencyKey: "accepted-projection-atomic",
		CommandFingerprint: strings.Repeat("8", 64), DecidedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if _, _, err := repository.AppendDocumentMatchOverride(context.Background(), command); err == nil {
		t.Fatal("accepted override succeeded while its durable projection could not be scheduled")
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_match_overrides WHERE match_decision_id=$1`, stored[0].ID).Scan(&count); err != nil {
		t.Fatalf("count rolled-back override: %v", err)
	}
	if count != 0 || !scheduler.sawTransaction {
		t.Fatalf("override count/transaction = %d/%t, want 0/true", count, scheduler.sawTransaction)
	}

	scheduler.err = nil
	created, reused, err := repository.AppendDocumentMatchOverride(context.Background(), command)
	if err != nil || reused || created.Sequence != 1 {
		t.Fatalf("AppendDocumentMatchOverride(retry) = %#v reused=%t err=%v", created, reused, err)
	}
	if scheduler.command != (ingestionapplication.ScheduleAcceptedDocumentMatchProjectionCommand{
		DocumentMatchDecisionID: stored[0].ID, DocumentVersionID: fixture.firstDocumentVersionID, EffectiveSequence: 1,
	}) {
		t.Fatalf("scheduled command = %#v", scheduler.command)
	}
	if _, reused, err := repository.AppendDocumentMatchOverride(context.Background(), command); err != nil || !reused {
		t.Fatalf("AppendDocumentMatchOverride(replay) reused=%t err=%v", reused, err)
	}
	if scheduler.calls != 3 {
		t.Fatalf("scheduler calls = %d, want failed + created + exact replay", scheduler.calls)
	}
}

type documentMatchFixture struct {
	monitorID, monitorVersionID, compiledProfileID, relevanceProfileID int64
	firstDocumentVersionID, secondDocumentVersionID                    int64
	adminUserID, editorUserID, viewerUserID                            int64
}

func createDocumentMatchFixture(t *testing.T, runtime *database.Runtime, suffix string) documentMatchFixture {
	t.Helper()
	fixture := documentMatchFixture{}
	fixture.adminUserID = insertDocumentMatchUser(t, runtime, suffix+"-admin", "admin")
	fixture.editorUserID = insertDocumentMatchUser(t, runtime, suffix+"-editor", "editor")
	fixture.viewerUserID = insertDocumentMatchUser(t, runtime, suffix+"-viewer", "viewer")
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name,status,created_by,updated_by) VALUES ($1,'draft',$2,$2) RETURNING id`,
		"document-match-"+suffix, fixture.adminUserID).Scan(&fixture.monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id,revision,created_by,updated_by) VALUES ($1,1,$2,$2) RETURNING id`,
		fixture.monitorID, fixture.adminUserID).Scan(&fixture.monitorVersionID); err != nil {
		t.Fatalf("insert monitor version: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=$2 WHERE id=$1`, fixture.monitorID, fixture.monitorVersionID); err != nil {
		t.Fatalf("attach draft monitor version: %v", err)
	}
	var draftID, revisionID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_intent_drafts (monitor_id,config_version_id) VALUES ($1,$2) RETURNING id`,
		fixture.monitorID, fixture.monitorVersionID).Scan(&draftID); err != nil {
		t.Fatalf("insert intent draft: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective)
VALUES ($1,$2,$3,1,'track semantic document matches') RETURNING id`, draftID, fixture.monitorID, fixture.monitorVersionID).Scan(&revisionID); err != nil {
		t.Fatalf("insert intent revision: %v", err)
	}
	var riverJobID, previewRunID, previewProfileID int64
	previewKey := "document-match-preview-" + suffix
	if err := runtime.SQL.QueryRow(`
INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,finalized_at,unique_key)
VALUES ('monitor_intent_analysis','{}','completed',1,1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,$1)
RETURNING id`, []byte(previewKey)).Scan(&riverJobID); err != nil {
		t.Fatalf("insert preview job: %v", err)
	}
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatalf("begin preview result transaction: %v", err)
	}
	if err := transaction.QueryRow(`
INSERT INTO monitor_intent_analysis_runs (
 monitor_id,draft_id,draft_resource_version,kind,input_hash,profile_version,sample_limit,
 request_hash,idempotency_key,river_job_id,status,queued_at,started_at,completed_at,result_fingerprint
) VALUES ($1,$2,1,'preview',repeat('1',64),'preview-v1',25,repeat('2',64),$3,$4,'succeeded',
          CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,repeat('3',64))
RETURNING id`, fixture.monitorID, draftID, previewKey, riverJobID).Scan(&previewRunID); err != nil {
		_ = transaction.Rollback()
		t.Fatalf("insert preview run: %v", err)
	}
	if _, err := transaction.Exec(`
INSERT INTO monitor_intent_preview_results (run_id,estimated_alert_count) VALUES ($1,0)`, previewRunID); err != nil {
		_ = transaction.Rollback()
		t.Fatalf("insert preview result: %v", err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("commit preview result: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
 monitor_id,purpose,config_version_id,preview_run_id,draft_id,draft_resource_version,intent_revision_id,
 compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
 structured_algorithm_version,search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'preview',$2,$3,$4,1,$5,'monitor-intent-compiler-v1','rrf-k60-v1','fts-trgm-dice-v1',
          'halfvec-cosine-v1','entity-hard-rule-v1','canonical-nfc-plaintext-v1','unavailable','semantic_generation_unavailable')
RETURNING id`, fixture.monitorID, fixture.monitorVersionID, previewRunID, draftID, revisionID).Scan(&previewProfileID); err != nil {
		t.Fatalf("insert preview compiled profile: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('7',64),ready_at=CURRENT_TIMESTAMP WHERE id=$1`, previewProfileID); err != nil {
		t.Fatalf("ready preview compiled profile: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
 monitor_id,purpose,config_version_id,monitor_version_id,source_preview_compiled_profile_id,intent_revision_id,
 compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
 structured_algorithm_version,search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'published',$2,$2,$3,$4,'monitor-intent-compiler-v1','rrf-k60-v1','fts-trgm-dice-v1',
          'halfvec-cosine-v1','entity-hard-rule-v1','canonical-nfc-plaintext-v1','unavailable','semantic_generation_unavailable')
RETURNING id`, fixture.monitorID, fixture.monitorVersionID, previewProfileID, revisionID).Scan(&fixture.compiledProfileID); err != nil {
		t.Fatalf("insert compiled profile: %v", err)
	}
	publishedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state='published',config_hash=repeat('9',64),published_at=$2 WHERE id=$1`,
		fixture.monitorVersionID, publishedAt); err != nil {
		t.Fatalf("publish monitor version: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='active',draft_config_version_id=NULL,published_config_version_id=$2 WHERE id=$1`,
		fixture.monitorID, fixture.monitorVersionID); err != nil {
		t.Fatalf("publish monitor pointer: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('8',64),ready_at=$2 WHERE id=$1`,
		fixture.compiledProfileID, publishedAt); err != nil {
		t.Fatalf("ready compiled profile: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO relevance_decision_profiles (
 profile_name,matching_algorithm_version,reranker_version,calibration_version,status,
 reject_threshold,accept_threshold,created_by_user_id
) VALUES ($1,'rrf-k60-v1','cross-encoder-v1','uncalibrated-v1','uncalibrated',0.4,0.8,$2)
RETURNING id`, "uncalibrated-"+suffix, fixture.adminUserID).Scan(&fixture.relevanceProfileID); err != nil {
		t.Fatalf("insert relevance profile: %v", err)
	}
	fixture.firstDocumentVersionID = insertDocumentMatchVersion(t, runtime, suffix+"-first", 1)
	fixture.secondDocumentVersionID = insertDocumentMatchVersion(t, runtime, suffix+"-second", 2)
	return fixture
}

func openDocumentMatchRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	return runtime
}

func insertDocumentMatchVersion(t *testing.T, runtime *database.Runtime, suffix string, index int) int64 {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	var sourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss',$1,$2) RETURNING id`, "match-source-"+suffix, "https://feed.example.test/"+suffix).Scan(&sourceID); err != nil {
		t.Fatalf("insert source connection: %v", err)
	}
	var observationID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_observations (
 source_connection_id,external_id,upstream_identity,source_code,content_type,title,language,
 source_record_url,canonical_url,body_origin,completeness,discovered_at,captured_at
) VALUES ($1,$2,$3,'rss','article',$4,'en',$5,$6,'feed_content','full',$7,$7)
RETURNING id`, sourceID, "work-"+suffix, fmt.Sprintf("%064x", index), "document "+suffix,
		"https://feed.example.test/records/"+suffix, "https://publisher.example.test/articles/"+suffix, now).Scan(&observationID); err != nil {
		t.Fatalf("insert source observation: %v", err)
	}
	var documentID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO documents (source_connection_id,document_key,external_work_id)
VALUES ($1,$2,$3) RETURNING id`, sourceID, strings.Repeat(fmt.Sprintf("%x", index), 64), "work-"+suffix).Scan(&documentID); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	var versionID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO document_versions (
 document_id,source_observation_id,revision_no,version_key,body_origin,completeness,word_count,
 language,content_sha256,extractor_version,extractor_profile_version,extractor_profile_sha256,
 lifecycle_state,captured_at
) VALUES ($1,$2,1,$3,'feed_content','full',4,'en',$4,'rss-entry-v2','rss-profile-v3',$5,'derived_available',$6)
RETURNING id`, documentID, observationID, strings.Repeat(fmt.Sprintf("%x", index+2), 64),
		strings.Repeat(fmt.Sprintf("%x", index+4), 64), strings.Repeat(fmt.Sprintf("%x", index+6), 64), now).Scan(&versionID); err != nil {
		t.Fatalf("insert document version: %v", err)
	}
	return versionID
}

func insertDocumentMatchUser(t *testing.T, runtime *database.Runtime, suffix, role string) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role)
VALUES ($1,'not-a-real-password-hash',$2,$3) RETURNING id`, suffix+"@example.test", suffix, role).Scan(&id); err != nil {
		t.Fatalf("insert %s user: %v", role, err)
	}
	return id
}

func documentMatchCommand(fixture documentMatchFixture, documentVersionID int64, hashCharacter byte) ingestionapplication.PersistAutomaticDocumentMatchCommand {
	return ingestionapplication.PersistAutomaticDocumentMatchCommand{
		MonitorID: fixture.monitorID, MonitorVersionID: fixture.monitorVersionID,
		CompiledProfileID: fixture.compiledProfileID, DocumentVersionID: documentVersionID,
		RelevanceProfileID: fixture.relevanceProfileID, MatchingAlgorithmVersion: "rrf-k60-v1",
		RerankerVersion: "cross-encoder-v1", CalibrationVersion: "uncalibrated-v1",
		RRFScore: 0.02, Decision: "review", Degraded: true,
		ReasonCodes: []string{"relevance_reranker_unavailable", "semantic_model_unavailable"},
		Signals: []ingestionapplication.DocumentMatchSignalDTO{{
			Channel: "lexical", Rank: 1, RawScore: 0.75, AlgorithmVersion: "fts-trgm-dice-v1",
		}},
		InputHash: strings.Repeat(string(hashCharacter), 64), DecidedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

type documentMatchProjectionSchedulerFake struct {
	command        ingestionapplication.ScheduleAcceptedDocumentMatchProjectionCommand
	calls          int
	sawTransaction bool
	err            error
}

func (fake *documentMatchProjectionSchedulerFake) ScheduleAcceptedDocumentMatchProjection(ctx context.Context,
	command ingestionapplication.ScheduleAcceptedDocumentMatchProjectionCommand) (ingestionapplication.ScheduleAcceptedDocumentMatchProjectionResult, error) {
	fake.calls++
	fake.command = command
	_, fake.sawTransaction = database.TransactionFromContext(ctx)
	return ingestionapplication.ScheduleAcceptedDocumentMatchProjectionResult{
		DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		DocumentVersionID:       command.DocumentVersionID,
		EffectiveSequence:       command.EffectiveSequence,
		JobID:                   97,
		Created:                 fake.calls == 2,
	}, fake.err
}
