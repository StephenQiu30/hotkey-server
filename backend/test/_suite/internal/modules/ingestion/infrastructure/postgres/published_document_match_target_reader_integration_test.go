package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestPublishedMatchTargetReaderSelectsExactActiveSourceAndHighestSafetyProfile(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "published-match-target", 91)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	projection := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, t.TempDir()), fixture.documentVersions)
	plaintext := []byte("authorized normalized document body")
	projected, err := projection.Project(context.Background(), ingestionapplication.ProjectDocumentCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, ExpectedDocumentVersion: fixture.persisted.DocumentVersion.Version,
		ArtifactType: ingestionapplication.DocumentProjectionPlaintext, TransformerProfileSHA256: fmt.Sprintf("%064x", 91),
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID, ProjectionBytes: plaintext,
	})
	if err != nil {
		t.Fatalf("Project(plaintext): %v", err)
	}
	writer, _ := ingestionpostgres.NewDocumentRecallProjectionWriter(runtime)
	searches, _ := ingestionapplication.NewDocumentRecallProjectionService(writer)
	indexedAt := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := searches.PersistSearchProjection(context.Background(), ingestionapplication.PersistDocumentSearchProjectionCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, DerivedArtifactID: projected.Artifact.ID,
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		NormalizationProfileVersion: ingestionapplication.CanonicalDocumentSearchNormalizationProfileVersion,
		NormalizedTextSHA256:        fixture.persisted.DocumentVersion.ContentSHA256, Plaintext: string(plaintext), IndexedAt: indexedAt,
		EntityKeys: []string{}, ActionKeys: []string{}, LocationKeys: []string{}, RegionKeys: []string{},
	}); err != nil {
		t.Fatalf("PersistSearchProjection(): %v", err)
	}

	monitorID, monitorVersionID, compiledProfileID, userID := createPublishedMatchTargetMonitor(t, runtime, fixture.sourceID, "active")
	uncalibratedID := insertPublishedMatchTargetProfile(t, runtime, userID, "uncalibrated", "uncalibrated-target-v1", "target-reranker-v1")
	shadowID := insertPublishedMatchTargetProfile(t, runtime, userID, "shadow", "shadow-target-v1", "target-reranker-v2")
	activeID := insertPublishedMatchTargetActiveProfile(t, runtime, userID)
	reader, err := ingestionpostgres.NewPublishedMatchTargetReader(runtime)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.ReadPublishedMatchTargets(context.Background(), ingestionapplication.PublishedMatchTargetsQuery{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID,
	})
	if err != nil {
		t.Fatalf("ReadPublishedMatchTargets(): %v", err)
	}
	if len(result.Targets) != 1 {
		t.Fatalf("targets = %#v, want one", result.Targets)
	}
	target := result.Targets[0]
	if target.MonitorID != monitorID || target.MonitorVersionID != monitorVersionID || target.CompiledProfileID != compiledProfileID ||
		target.RelevanceProfileID != activeID || target.RelevanceProfileID == shadowID || target.RelevanceProfileID == uncalibratedID {
		t.Fatalf("target = %#v", target)
	}
	trigger, err := reader.ReadPublishedMonitorTrigger(context.Background(), ingestionapplication.ReadPublishedMonitorTriggerQuery{
		MonitorID: monitorID, MonitorVersionID: monitorVersionID, CompiledProfileID: compiledProfileID,
	})
	if err != nil || !trigger.Exists || trigger.DocumentVersionID != fixture.persisted.DocumentVersion.ID {
		t.Fatalf("published monitor trigger/error = %#v/%v", trigger, err)
	}

	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='paused' WHERE id=$1`, monitorID); err != nil {
		t.Fatal(err)
	}
	paused, err := reader.ReadPublishedMatchTargets(context.Background(), ingestionapplication.PublishedMatchTargetsQuery{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID,
	})
	if err != nil || len(paused.Targets) != 0 {
		t.Fatalf("paused targets/error = %#v/%v", paused.Targets, err)
	}
	pausedTrigger, err := reader.ReadPublishedMonitorTrigger(context.Background(), ingestionapplication.ReadPublishedMonitorTriggerQuery{
		MonitorID: monitorID, MonitorVersionID: monitorVersionID, CompiledProfileID: compiledProfileID,
	})
	if err != nil || pausedTrigger.Exists {
		t.Fatalf("paused trigger/error = %#v/%v", pausedTrigger, err)
	}
}

func insertPublishedMatchTargetActiveProfile(t *testing.T, runtime *database.Runtime, userID int64) int64 {
	t.Helper()
	var evaluationRunID, id int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO relevance_evaluation_runs (
	 dataset_version,dataset_hash,family_isolation_hash,time_boundary,sample_window_start,sample_window_end,
	 annotation_protocol_version,annotation_guideline_sha256,split_strategy_version,annotator_count,agreement_metric,agreement_score,
	 matching_algorithm_version,reranker_version,calibration_version,calibration_slope,calibration_intercept,reject_threshold,accept_threshold,
 sample_count,positive_count,negative_count,recall_at_100,precision_score,recall_score,
 expected_calibration_error,brier_score,precision_wilson_lower,hard_negative_count,hard_negative_passed,
 status,evaluated_by_user_id,evaluated_at
	) VALUES ('target-time-split-v1',repeat('4',64),repeat('5',64),CURRENT_TIMESTAMP-interval '30 days',CURRENT_TIMESTAMP-interval '20 days',CURRENT_TIMESTAMP-interval '1 day',
	          'independent-reference-v1',repeat('6',64),'time-family-source-v1',2,'cohen_kappa',0.95,
	          'rrf-k60-v1',$1,$2,1,0,0.4,0.8,400,200,200,0.97,0.95,0.90,0.04,0.03,0.91,100,true,
          'passed',$3,CURRENT_TIMESTAMP) RETURNING id`, ingestionapplication.CanonicalDocumentMatchRerankerVersion,
		ingestionapplication.CanonicalDocumentMatchCalibrationVersion, userID).Scan(&evaluationRunID); err != nil {
		t.Fatalf("insert active relevance evaluation: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO relevance_evaluation_slices (
 evaluation_run_id,dimension,value,sample_count,positive_count,negative_count,precision_score,recall_score,passed
) VALUES ($1,'source','rss',400,200,200,0.95,0.90,true),
         ($1,'language','en',200,100,100,0.95,0.90,true)`, evaluationRunID); err != nil {
		t.Fatalf("insert active relevance slice: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO relevance_decision_profiles (
 profile_name,matching_algorithm_version,reranker_version,calibration_version,status,
	 reject_threshold,accept_threshold,calibration_slope,calibration_intercept,evaluation_sample_count,evaluation_run_id,created_by_user_id,activated_by_user_id,activated_at
	) VALUES ($1,'rrf-k60-v1',$2,$3,'active',0.4,0.8,1,0,400,$4,$5,$5,CURRENT_TIMESTAMP) RETURNING id`,
		"published-target-active", ingestionapplication.CanonicalDocumentMatchRerankerVersion,
		ingestionapplication.CanonicalDocumentMatchCalibrationVersion, evaluationRunID, userID).Scan(&id); err != nil {
		t.Fatalf("insert active relevance profile: %v", err)
	}
	return id
}

func createPublishedMatchTargetMonitor(t *testing.T, runtime *database.Runtime, sourceConnectionID int64, suffix string) (int64, int64, int64, int64) {
	t.Helper()
	var userID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role) VALUES ($1,'fixture',$2,'admin') RETURNING id`,
		"published-target-"+suffix+"@example.test", "published target "+suffix).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var monitorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name,status,created_by,updated_by) VALUES ($1,'draft',$2,$2) RETURNING id`,
		"published target "+suffix, userID).Scan(&monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	var configVersionID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id,revision,created_by,updated_by) VALUES ($1,1,$2,$2) RETURNING id`, monitorID, userID).Scan(&configVersionID); err != nil {
		t.Fatalf("insert config: %v", err)
	}
	var draftID, revisionID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_intent_drafts (monitor_id,config_version_id) VALUES ($1,$2) RETURNING id`, monitorID, configVersionID).Scan(&draftID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective) VALUES ($1,$2,$3,1,'target source events') RETURNING id`, draftID, monitorID, configVersionID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_sources (config_version_id,source_connection_id) VALUES ($1,$2)`, configVersionID, sourceConnectionID); err != nil {
		t.Fatal(err)
	}
	var riverJobID, previewRunID, previewProfileID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,finalized_at,unique_key)
VALUES ('monitor_intent_analysis','{}','completed',1,1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,$1) RETURNING id`,
		[]byte("published-target-preview-"+suffix)).Scan(&riverJobID); err != nil {
		t.Fatal(err)
	}
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`
INSERT INTO monitor_intent_analysis_runs (
 monitor_id,draft_id,draft_resource_version,kind,input_hash,profile_version,sample_limit,
 request_hash,idempotency_key,river_job_id,status,queued_at,started_at,completed_at,result_fingerprint
) VALUES ($1,$2,1,'preview',repeat('1',64),'preview-v1',25,repeat('2',64),$3,$4,'succeeded',
          CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,repeat('3',64)) RETURNING id`,
		monitorID, draftID, "published-target-preview-"+suffix, riverJobID).Scan(&previewRunID); err != nil {
		_ = transaction.Rollback()
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`INSERT INTO monitor_intent_preview_results (run_id,estimated_alert_count) VALUES ($1,0)`, previewRunID); err != nil {
		_ = transaction.Rollback()
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
 monitor_id,purpose,config_version_id,preview_run_id,draft_id,draft_resource_version,intent_revision_id,
 compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,structured_algorithm_version,
 search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'preview',$2,$3,$4,1,$5,'monitor-intent-compiler-v1','rrf-k60-v1','fts-trgm-dice-v1',
          'halfvec-cosine-v1','entity-hard-rule-v1',$6,'unavailable','semantic_generation_unavailable')
RETURNING id`, monitorID, configVersionID, previewRunID, draftID, revisionID,
		ingestionapplication.CanonicalDocumentSearchNormalizationProfileVersion).Scan(&previewProfileID); err != nil {
		t.Fatalf("insert preview compiled profile: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('7',64),ready_at=CURRENT_TIMESTAMP WHERE id=$1`, previewProfileID); err != nil {
		t.Fatalf("ready preview compiled profile: %v", err)
	}
	var compiledProfileID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
 monitor_id,purpose,config_version_id,monitor_version_id,source_preview_compiled_profile_id,intent_revision_id,compiler_version,
 matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,structured_algorithm_version,
 search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'published',$2,$2,$3,$4,'monitor-intent-compiler-v1','rrf-k60-v1','fts-trgm-dice-v1',
 'halfvec-cosine-v1','entity-hard-rule-v1',$5,'unavailable','semantic_generation_unavailable') RETURNING id`,
		monitorID, configVersionID, previewProfileID, revisionID, ingestionapplication.CanonicalDocumentSearchNormalizationProfileVersion).Scan(&compiledProfileID); err != nil {
		t.Fatalf("insert compiled profile: %v", err)
	}
	publishedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state='published',config_hash=repeat('9',64),published_at=$2 WHERE id=$1`, configVersionID, publishedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='active',published_config_version_id=$2 WHERE id=$1`, monitorID, configVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=repeat('8',64),ready_at=$2 WHERE id=$1`, compiledProfileID, publishedAt); err != nil {
		t.Fatal(err)
	}
	return monitorID, configVersionID, compiledProfileID, userID
}

func insertPublishedMatchTargetProfile(t *testing.T, runtime *database.Runtime, userID int64, status, calibrationVersion, rerankerVersion string) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO relevance_decision_profiles (
 profile_name,matching_algorithm_version,reranker_version,calibration_version,status,reject_threshold,accept_threshold,calibration_slope,calibration_intercept,created_by_user_id
) VALUES ($1,'rrf-k60-v1',$2,$3,$4,0.4,0.8,1,0,$5) RETURNING id`,
		"published-target-"+status+"-"+calibrationVersion, rerankerVersion, calibrationVersion, status, userID).Scan(&id); err != nil {
		t.Fatalf("insert relevance profile: %v", err)
	}
	return id
}
