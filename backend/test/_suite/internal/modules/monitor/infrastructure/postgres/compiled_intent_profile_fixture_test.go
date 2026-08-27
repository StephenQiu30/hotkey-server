package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func createUnavailablePreviewCompiledProfile(t *testing.T, runtime *database.Runtime, fixture intentRepositoryFixture, now time.Time, clauses []monitorapplication.CompiledIntentClauseDTO, entities []monitorapplication.CompiledIntentEntityDTO) (int64, int64) {
	t.Helper()
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	reservation := intentPreviewReservation(fixture, now)
	reservation.IdempotencyKey = fmt.Sprintf("preview.compiled.%d.%d", fixture.monitorID, fixture.draftID)
	reserved, err := repository.ReserveAndEnqueue(context.Background(), reservation)
	if err != nil {
		t.Fatalf("reserve preview: %v", err)
	}
	running := reserved.Run
	startedAt := now.Add(time.Second)
	running.Status, running.StartedAt = "running", &startedAt
	if _, err := repository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: reserved.Run, Next: running}); err != nil {
		t.Fatalf("start preview: %v", err)
	}
	command := monitorapplication.PersistPreviewCompiledProfileDTO{
		Task: monitorapplication.IntentAnalysisTaskDTO{
			Run: monitorapplication.IntentRunReferenceDTO{
				RunID: running.ID, Kind: "preview", MonitorID: fixture.monitorID, DraftID: fixture.draftID,
				DraftResourceVersion: 1, InputHash: running.InputHash,
			}, AnalysisProfile: "preview-v1", SampleLimit: 25,
		},
		CompilerVersion:          monitorapplication.IntentCompilerVersion,
		MatchingAlgorithmVersion: "rrf-k60-v1", LexicalAlgorithmVersion: "fts-trgm-dice-v1",
		SemanticAlgorithmVersion: "halfvec-cosine-v1", StructuredAlgorithmVersion: "entity-hard-rule-v1",
		SearchNormalizationProfileVersion: monitorapplication.IntentSearchNormalizationProfileVersion,
		SemanticState:                     monitorapplication.IntentSemanticStateUnavailable,
		SemanticUnavailableReason:         monitorapplication.IntentSemanticGenerationUnavailable,
		ProfileHash:                       strings.Repeat("9", 64), ReadyAt: startedAt, Clauses: clauses, Entities: entities,
	}
	receipt, err := repository.PersistPreviewCompiledProfile(context.Background(), command)
	if err != nil {
		t.Fatalf("stage preview profile: %v", err)
	}
	if _, err := repository.CompletePreviewCompiledProfile(context.Background(), monitorapplication.CompletePreviewCompiledProfileDTO{
		CompiledProfileID: receipt.CompiledProfileID, ConfigVersionID: fixture.configID,
		IntentRevisionID: receipt.IntentRevisionID, ProfileHash: command.ProfileHash,
		SemanticState:             monitorapplication.IntentSemanticStateUnavailable,
		SemanticUnavailableReason: monitorapplication.IntentSemanticModelUnavailable, ReadyAt: command.ReadyAt,
	}); err != nil {
		t.Fatalf("complete preview profile: %v", err)
	}
	return receipt.CompiledProfileID, receipt.IntentRevisionID
}
