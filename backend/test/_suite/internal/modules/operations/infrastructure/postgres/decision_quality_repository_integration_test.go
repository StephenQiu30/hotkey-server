//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestDecisionQualityRepositoryPersistsAppendOnlyRunsAndActivatesProfiles(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	administratorID := insertDecisionQualityAdministrator(t, runtime)
	repository := NewDecisionQualityRepository(runtime)
	command := makeDecisionQualityPersistenceCommand(administratorID, true)
	created, err := repository.PersistDecisionQualityEvaluation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if created.Reused || len(created.Runs) != 5 || len(created.Profiles) != 5 {
		t.Fatalf("created receipt = %#v", created)
	}
	for _, profile := range created.Profiles {
		if profile.Status != "active" || profile.ActivatedByUserID == nil || *profile.ActivatedByUserID != administratorID {
			t.Fatalf("active profile = %#v", profile)
		}
	}
	active, err := repository.IsDecisionQualityProfileActive(ctx, "content_family", "content-family-conservative-v1")
	if err != nil || !active {
		t.Fatalf("active profile projection = %t/%v", active, err)
	}
	inactive, err := repository.IsDecisionQualityProfileActive(ctx, "content_family", "unknown-profile")
	if err != nil || inactive {
		t.Fatalf("inactive profile projection = %t/%v", inactive, err)
	}
	replayed, err := repository.PersistDecisionQualityEvaluation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Reused || len(replayed.Runs) != 5 {
		t.Fatalf("replayed receipt = %#v", replayed)
	}
	var activeCount, runCount, sliceCount int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM decision_quality_profiles WHERE status='active'`).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM decision_quality_evaluation_runs`).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM decision_quality_evaluation_slices`).Scan(&sliceCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 5 || runCount != 5 || sliceCount != 3 {
		t.Fatalf("persisted facts active=%d runs=%d slices=%d", activeCount, runCount, sliceCount)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE decision_quality_evaluation_runs SET passed=false WHERE id=$1`, created.Runs[0].ID); err == nil {
		t.Fatal("append-only evaluation run was mutable")
	}
}

func TestDecisionQualityRepositoryRejectsUnauthorizedActorAndChangedReplay(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	administratorID := insertDecisionQualityAdministrator(t, runtime)
	repository := NewDecisionQualityRepository(runtime)
	command := makeDecisionQualityPersistenceCommand(administratorID, false)
	if _, err := repository.PersistDecisionQualityEvaluation(ctx, command); err != nil {
		t.Fatal(err)
	}
	command.Metrics[0].Passed = false
	if _, err := repository.PersistDecisionQualityEvaluation(ctx, command); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("changed replay error = %v", err)
	}
	command = makeDecisionQualityPersistenceCommand(administratorID+1, true)
	if _, err := repository.PersistDecisionQualityEvaluation(ctx, command); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("unauthorized actor error = %v", err)
	}
}

func makeDecisionQualityPersistenceCommand(actorID int64, activate bool) operationsapplication.PersistDecisionQualityEvaluationCommand {
	evaluatedAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	modules := []string{"content_family", "micro_event_clustering", "evidence_locator", "evidence_relation", "hotspot_detection"}
	profiles := []string{"content-family-conservative-v1", "same-event-cold-start-v1", "w3c-text-quote-v1", "claim-evidence-relation-v1", "event-heat-v2"}
	command := operationsapplication.PersistDecisionQualityEvaluationCommand{ActorUserID: actorID, Activate: activate, EvaluatedAt: evaluatedAt,
		Dataset: operationsapplication.DecisionQualityDatasetMetadataDTO{DatasetVersion: "decision-quality-time-isolated-v1", DatasetSHA256: repeatDecisionQualitySHA("a"),
			AnnotationProtocolVersion: "dual-review-v1", AnnotationGuidelineSHA256: repeatDecisionQualitySHA("b"), SplitStrategyVersion: "time-family-event-isolated-v1",
			FamilyIsolationSHA256: repeatDecisionQualitySHA("c"), EventIsolationSHA256: repeatDecisionQualitySHA("d"), AnnotatorCount: 2,
			AgreementMetric: "cohen_kappa", AgreementScore: .98, TimeBoundary: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)}}
	for index := range modules {
		command.Metrics = append(command.Metrics, operationsapplication.DecisionQualityMetricDTO{Module: modules[index], ProfileVersion: profiles[index],
			SampleCount: 400, PositiveCount: 200, NegativeCount: 200, Precision: 1, Recall: 1, PrecisionWilsonLower: .98,
			PairwisePrecision: 1, BCubedF1: 1, CEAFE: 1, ClusterCountRatio: 1, LocatorAccuracy: 1,
			ProvenanceCompleteness: 1, EvidenceRelationMacroF1: 1, HotspotPrecision: 1,
			MedianDiscoveryDelaySeconds: 120, Passed: true, AutomaticDecisionAllowed: true, ReasonCodes: []string{"quality_gate_passed"}})
	}
	command.Slices = []operationsapplication.DecisionQualitySliceDTO{
		{Module: "content_family", Dimension: "language", Value: "zh", SampleCount: 100, Precision: 1, Recall: 1, Passed: true},
		{Module: "micro_event_clustering", Dimension: "event_size", Value: "small", SampleCount: 100, Precision: 1, Recall: 1, Passed: true},
		{Module: "hotspot_detection", Dimension: "source_type", Value: "feed", SampleCount: 100, Precision: 1, Recall: 1, Passed: true},
	}
	return command
}

func insertDecisionQualityAdministrator(t *testing.T, runtime *database.Runtime) int64 {
	t.Helper()
	var identifier int64
	if err := runtime.SQL.QueryRowContext(t.Context(), `INSERT INTO users (email,password_hash,display_name,role,status) VALUES ('quality-admin@example.test','x','quality admin','admin','active') RETURNING id`).Scan(&identifier); err != nil {
		t.Fatal(err)
	}
	return identifier
}

func repeatDecisionQualitySHA(value string) string {
	result := ""
	for range 64 {
		result += value
	}
	return result
}
