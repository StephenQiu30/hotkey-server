package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestRelevanceCalibrationRepositoryPersistsExactActiveEvaluationAndReplays(t *testing.T) {
	runtime := openDocumentMatchRuntime(t)
	defer runtime.Close()
	adminID := insertDocumentMatchUser(t, runtime, "calibration-admin", "admin")
	repository, err := NewDocumentMatchRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	command := passingCalibrationPersistenceCommand(adminID, 'a')
	first, err := repository.PersistRelevanceCalibration(context.Background(), command)
	if err != nil {
		t.Fatalf("PersistRelevanceCalibration() error = %v", err)
	}
	if first.ID <= 0 || first.EvaluationRunID <= 0 || first.Status != "active" || first.EvaluationSampleCount != 400 {
		t.Fatalf("profile = %#v", first)
	}
	replayed, err := repository.PersistRelevanceCalibration(context.Background(), command)
	if err != nil || replayed.ID != first.ID || replayed.EvaluationRunID != first.EvaluationRunID {
		t.Fatalf("replay = %#v / %v", replayed, err)
	}
	conflict := command
	conflict.DatasetVersion = "different-dataset-v1"
	if _, err := repository.PersistRelevanceCalibration(context.Background(), conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
	successor, err := repository.PersistRelevanceCalibration(context.Background(), passingCalibrationPersistenceCommand(adminID, 'c'))
	if err != nil || successor.ID == first.ID || successor.EvaluationRunID == first.EvaluationRunID {
		t.Fatalf("independent calibrated dataset profile = %#v / %v", successor, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE relevance_evaluation_runs SET precision_score=0 WHERE id=$1`, first.EvaluationRunID); err == nil {
		t.Fatal("append-only evaluation run was mutated")
	}
	if _, err := runtime.SQL.Exec(`UPDATE relevance_decision_profiles SET accept_threshold=0.9 WHERE id=$1`, first.ID); err == nil {
		t.Fatal("immutable active relevance profile was mutated")
	}
}

func TestRelevanceCalibrationRepositoryRejectsNonAdminAndUngatedActiveProfile(t *testing.T) {
	runtime := openDocumentMatchRuntime(t)
	defer runtime.Close()
	viewerID := insertDocumentMatchUser(t, runtime, "calibration-viewer", "viewer")
	repository, _ := NewDocumentMatchRepository(runtime)
	if _, err := repository.PersistRelevanceCalibration(context.Background(), passingCalibrationPersistenceCommand(viewerID, 'b')); !errors.Is(err, ingestionapplication.ErrDocumentMatchAuthorizationDenied) {
		t.Fatalf("viewer calibration error = %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO relevance_decision_profiles (
 profile_name,matching_algorithm_version,reranker_version,calibration_version,status,reject_threshold,accept_threshold,
 evaluation_sample_count,created_by_user_id,activated_by_user_id,activated_at
) VALUES ('ungated','rrf-k60-v1','rank-signal-logit-v1','time-split-platt-v1','active',0.4,0.8,400,$1,$1,CURRENT_TIMESTAMP)`, viewerID); err == nil {
		t.Fatal("active profile without an evaluation run succeeded")
	}
}

func passingCalibrationPersistenceCommand(actorID int64, hashCharacter byte) ingestionapplication.PersistRelevanceCalibrationCommand {
	evaluatedAt := time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC)
	return ingestionapplication.PersistRelevanceCalibrationCommand{
		ActorUserID: actorID, ProfileName: "passing calibration " + string(hashCharacter),
		DatasetVersion: "relevance-time-split-2026-08-v1", DatasetHash: strings.Repeat(string(hashCharacter), 64),
		FamilyIsolationHash: strings.Repeat("f", 64), TimeBoundary: evaluatedAt.Add(-30 * 24 * time.Hour),
		AnnotationProtocolVersion: "dual-review-relevance-v1", AnnotationGuidelineSHA256: strings.Repeat("e", 64),
		SplitStrategyVersion: "time-family-event-isolated-v1", AnnotatorCount: 2,
		AgreementMetric: "cohen_kappa", AgreementScore: .96,
		SampleWindowStart: evaluatedAt.Add(-29 * 24 * time.Hour), SampleWindowEnd: evaluatedAt.Add(-24 * time.Hour),
		MatchingAlgorithmVersion: ingestionapplication.HybridRecallMatchingAlgorithmVersion,
		RerankerVersion:          ingestionapplication.CanonicalDocumentMatchRerankerVersion,
		CalibrationVersion:       ingestionapplication.CanonicalDocumentMatchCalibrationVersion + ":" + string(hashCharacter),
		RejectThreshold:          .4, AcceptThreshold: .8, Status: "active",
		CalibrationSlope: 1.25, CalibrationIntercept: -.1,
		Metrics: ingestionapplication.RelevanceEvaluationMetricsDTO{
			SampleCount: 400, PositiveCount: 200, NegativeCount: 200, RecallAt100: .97,
			Precision: .95, Recall: .90, ECE: .04, Brier: .03, PrecisionWilsonLower: .91,
			HardNegativeCount: 100, HardNegativePassed: true, Passed: true,
		},
		Slices: []ingestionapplication.RelevanceEvaluationSliceResultDTO{
			{Dimension: "language", Value: "en", SampleCount: 200, PositiveCount: 100, NegativeCount: 100, Precision: .95, Recall: .90, Passed: true},
			{Dimension: "language", Value: "zh", SampleCount: 200, PositiveCount: 100, NegativeCount: 100, Precision: .94, Recall: .88, Passed: true},
			{Dimension: "source", Value: "rss", SampleCount: 400, PositiveCount: 200, NegativeCount: 200, Precision: .95, Recall: .90, Passed: true},
		},
		EvaluatedAt: evaluatedAt.Add(time.Duration(hashCharacter-'a') * time.Minute),
	}
}
