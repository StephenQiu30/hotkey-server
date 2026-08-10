package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestCompiledRecallProfileReaderServesExplicitSemanticUnavailabilityAndRetires(t *testing.T) {
	runtime := openCompiledRecallProfileRuntime(t)
	defer func() { _ = runtime.Close() }()
	monitorID, configID, revisionID, sourcePreviewID := createPublishedCompiledProfileOwner(t, runtime, "unavailable")
	var profileID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
  monitor_id,purpose,config_version_id,monitor_version_id,source_preview_compiled_profile_id,intent_revision_id,
  compiler_version,matching_algorithm_version,lexical_algorithm_version,
  semantic_algorithm_version,structured_algorithm_version,search_normalization_profile_version,
  semantic_state,semantic_unavailable_reason
) VALUES ($1,'published',$2,$2,$3,$4,'intent-compiler-v1',$5,$6,$7,$8,'canonical-search-v1',
  'unavailable','semantic_model_unavailable') RETURNING id`, monitorID, configID, sourcePreviewID,
		revisionID,
		ingestionapplication.HybridRecallMatchingAlgorithmVersion, ingestionapplication.LexicalRecallAlgorithmVersion,
		ingestionapplication.SemanticRecallAlgorithmVersion, ingestionapplication.StructuredRecallAlgorithmVersion).Scan(&profileID); err != nil {
		t.Fatalf("insert unavailable compiled profile: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=$2,ready_at=now() WHERE id=$1`, profileID, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("ready unavailable compiled profile: %v", err)
	}
	reader, _ := monitorpostgres.NewCompiledRecallProfileReader(runtime)
	query := ingestionapplication.ReadyRecallProfileQuery{
		MonitorID: monitorID, Purpose: "published", ConfigVersionID: configID,
		MonitorVersionID: configID, CompiledProfileID: profileID,
	}
	profile, err := reader.ReadReadyRecallProfile(context.Background(), query)
	if err != nil {
		t.Fatalf("ReadReadyRecallProfile() error = %v", err)
	}
	if profile.SemanticState != ingestionapplication.SemanticRecallStateUnavailable ||
		profile.SemanticUnavailableReason != "semantic_model_unavailable" || profile.Semantic != nil || len(profile.Clauses) != 0 {
		t.Fatalf("profile = %#v", profile)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='retired',retired_at=now() WHERE id=$1`, profileID); err != nil {
		t.Fatalf("retire compiled profile: %v", err)
	}
	if _, err := reader.ReadReadyRecallProfile(context.Background(), query); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("retired profile error = %v", err)
	}
}

func TestCompiledRecallProfileRejectsReadySemanticWithoutExactEmbedding(t *testing.T) {
	runtime := openCompiledRecallProfileRuntime(t)
	defer func() { _ = runtime.Close() }()
	monitorID, configID, revisionID, sourcePreviewID := createPublishedCompiledProfileOwner(t, runtime, "missing-embedding")
	var profileID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
  monitor_id,purpose,config_version_id,monitor_version_id,source_preview_compiled_profile_id,intent_revision_id,
  compiler_version,matching_algorithm_version,lexical_algorithm_version,
  semantic_algorithm_version,structured_algorithm_version,search_normalization_profile_version,
  semantic_state
) VALUES ($1,'published',$2,$2,$3,$4,'intent-compiler-v1',$5,$6,$7,$8,'canonical-search-v1','ready')
RETURNING id`, monitorID, configID, sourcePreviewID,
		revisionID,
		ingestionapplication.HybridRecallMatchingAlgorithmVersion, ingestionapplication.LexicalRecallAlgorithmVersion,
		ingestionapplication.SemanticRecallAlgorithmVersion, ingestionapplication.StructuredRecallAlgorithmVersion).Scan(&profileID); err != nil {
		t.Fatalf("insert ready semantic compiled profile: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_compiled_profiles SET status='ready',profile_hash=$2,ready_at=now() WHERE id=$1`, profileID, strings.Repeat("b", 64)); err == nil {
		t.Fatal("semantic ready profile without exact embedding was accepted")
	}
}

func openCompiledRecallProfileRuntime(t *testing.T) *database.Runtime {
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

func createPublishedCompiledProfileOwner(t *testing.T, runtime *database.Runtime, suffix string) (int64, int64, int64, int64) {
	t.Helper()
	var monitorID, configID, draftID, revisionID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name,status) VALUES ($1,'active') RETURNING id`,
		fmt.Sprintf("compiled-profile-%s-%d", suffix, time.Now().UnixNano())).Scan(&monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id,revision) VALUES ($1,1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("insert draft config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=$2 WHERE id=$1`, monitorID, configID); err != nil {
		t.Fatalf("bind draft config: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_intent_drafts (monitor_id,config_version_id) VALUES ($1,$2) RETURNING id`, monitorID, configID).Scan(&draftID); err != nil {
		t.Fatalf("insert intent draft: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective)
VALUES ($1,$2,$3,1,'track exact published event intent') RETURNING id`, draftID, monitorID, configID).Scan(&revisionID); err != nil {
		t.Fatalf("insert intent revision: %v", err)
	}
	sourcePreviewID, _ := createUnavailablePreviewCompiledProfile(t, runtime, intentRepositoryFixture{
		monitorID: monitorID, configID: configID, draftID: draftID,
	}, time.Now().UTC().Add(-time.Hour), nil, nil)
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state='published',config_hash=$2,published_at=now() WHERE id=$1`, configID, strings.Repeat("c", 64)); err != nil {
		t.Fatalf("publish config: %v", err)
	}
	return monitorID, configID, revisionID, sourcePreviewID
}
