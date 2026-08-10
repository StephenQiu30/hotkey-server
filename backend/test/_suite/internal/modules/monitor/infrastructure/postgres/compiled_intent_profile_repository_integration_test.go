package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
)

func TestIntentRepositoryPersistsExactPreviewCompiledProfileIdempotently(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, false)
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	reserved, err := repository.ReserveAndEnqueue(context.Background(), intentPreviewReservation(fixture, now))
	if err != nil {
		t.Fatalf("ReserveAndEnqueue(): %v", err)
	}
	running := reserved.Run
	startedAt := now.Add(time.Second)
	running.Status, running.StartedAt = "running", &startedAt
	if _, err := repository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: reserved.Run, Next: running}); err != nil {
		t.Fatalf("start preview run: %v", err)
	}
	command := monitorapplication.PersistPreviewCompiledProfileDTO{
		Task: monitorapplication.IntentAnalysisTaskDTO{
			Run: monitorapplication.IntentRunReferenceDTO{
				RunID: running.ID, Kind: running.Kind, MonitorID: running.MonitorID, DraftID: running.DraftID,
				DraftResourceVersion: running.DraftResourceVersion, InputHash: running.InputHash,
			}, AnalysisProfile: "preview-v1", SampleLimit: 25,
		},
		CompilerVersion:          "monitor-intent-compiler-v1",
		MatchingAlgorithmVersion: "rrf-k60-v1", LexicalAlgorithmVersion: "fts-trgm-dice-v1",
		SemanticAlgorithmVersion: "halfvec-cosine-v1", StructuredAlgorithmVersion: "entity-hard-rule-v1",
		SearchNormalizationProfileVersion: "canonical-nfc-plaintext-v1",
		SemanticState:                     "unavailable", SemanticUnavailableReason: "semantic_generation_unavailable",
		ProfileHash: strings.Repeat("d", 64), ReadyAt: startedAt,
		Clauses: []monitorapplication.CompiledIntentClauseDTO{
			{Operator: "must", Field: "action", Value: "launch", NormalizedValue: "launch", Origin: "intent_clause"},
			{Operator: "should", Field: "phrase", Value: "Track launch disruption", NormalizedValue: "track launch disruption", Origin: "objective_derived"},
		},
		Entities: []monitorapplication.CompiledIntentEntityDTO{{
			CanonicalID: "product:hotkey", Aliases: []string{"Hot Key", "HotKey"}, NormalizedAliases: []string{"hot key", "hotkey"},
		}},
	}

	first, err := repository.PersistPreviewCompiledProfile(context.Background(), command)
	if err != nil {
		t.Fatalf("PersistPreviewCompiledProfile(): %v", err)
	}
	if first.CompiledProfileID <= 0 || first.ConfigVersionID != fixture.configID || first.IntentRevisionID <= 0 || first.Reused || first.Status != "building" {
		t.Fatalf("first receipt = %#v", first)
	}
	replayed, err := repository.PersistPreviewCompiledProfile(context.Background(), command)
	if err != nil || replayed.CompiledProfileID != first.CompiledProfileID || !replayed.Reused {
		t.Fatalf("replayed receipt = %#v / %v", replayed, err)
	}
	completed, err := repository.CompletePreviewCompiledProfile(context.Background(), monitorapplication.CompletePreviewCompiledProfileDTO{
		CompiledProfileID: first.CompiledProfileID, ConfigVersionID: fixture.configID,
		IntentRevisionID: first.IntentRevisionID, ProfileHash: command.ProfileHash,
		SemanticState:             monitorapplication.IntentSemanticStateUnavailable,
		SemanticUnavailableReason: monitorapplication.IntentSemanticModelUnavailable, ReadyAt: command.ReadyAt,
	})
	if err != nil || completed.Status != "ready" || completed.SemanticUnavailableReason != monitorapplication.IntentSemanticModelUnavailable {
		t.Fatalf("CompletePreviewCompiledProfile() = %#v / %v", completed, err)
	}
	var status, hash string
	var clauses, entities, aliases int
	if err := runtime.SQL.QueryRow(`SELECT status,btrim(profile_hash) FROM monitor_compiled_profiles WHERE id=$1`, first.CompiledProfileID).Scan(&status, &hash); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_compiled_clauses WHERE compiled_profile_id=$1`, first.CompiledProfileID).Scan(&clauses); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_compiled_entities WHERE compiled_profile_id=$1`, first.CompiledProfileID).Scan(&entities); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_compiled_entity_aliases WHERE compiled_profile_id=$1`, first.CompiledProfileID).Scan(&aliases); err != nil {
		t.Fatal(err)
	}
	if status != "ready" || hash != command.ProfileHash || clauses != 2 || entities != 1 || aliases != 2 {
		t.Fatalf("stored profile facts = status:%s hash:%s clauses:%d entities:%d aliases:%d", status, hash, clauses, entities, aliases)
	}
	profileReader, err := monitorpostgres.NewCompiledRecallProfileReader(runtime)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := profileReader.ReadReadyRecallProfile(context.Background(), ingestionapplication.ReadyRecallProfileQuery{
		MonitorID: fixture.monitorID, Purpose: "preview", ConfigVersionID: fixture.configID,
		PreviewRunID: running.ID, DraftID: fixture.draftID, DraftResourceVersion: 1,
		CompiledProfileID: first.CompiledProfileID,
	})
	if err != nil {
		t.Fatalf("ReadReadyRecallProfile(): %v", err)
	}
	if len(ready.Clauses) != 2 || len(ready.Entities) != 1 || len(ready.Entities[0].Aliases) != 2 || ready.SemanticUnavailableReason != monitorapplication.IntentSemanticModelUnavailable {
		t.Fatalf("ready recall profile = %#v", ready)
	}

	conflict := command
	conflict.ProfileHash = strings.Repeat("e", 64)
	if _, err := repository.PersistPreviewCompiledProfile(context.Background(), conflict); !errors.Is(err, monitorapplication.ErrCompiledIntentProfileConflict) {
		t.Fatalf("same preview owner different profile error = %v", err)
	}
}
