package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestAIRunRecomputeArgsRejectBodyAndUnknownFields(t *testing.T) {
	encoded, err := EncodeAIRunRecomputeJobArgs(41)
	if err != nil || string(encoded) != `{"run_id":41}` {
		t.Fatalf("EncodeAIRunRecomputeJobArgs() = %s / %v", encoded, err)
	}
	for _, raw := range []string{`{}`, `{"run_id":0}`, `{"run_id":41,"body":"secret"}`, `{"run_id":41} {}`} {
		if _, err := DecodeAIRunRecomputeJobArgs(json.RawMessage(raw)); err == nil {
			t.Fatalf("DecodeAIRunRecomputeJobArgs(%s) error = nil", raw)
		}
	}
}

func TestAIRunRecomputeWorkerReactivatesOwningJobFromRunIDOnly(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	store := queue.NewStore(runtime)
	ownerID, _, err := store.Enqueue(ctx, queue.Job{
		Kind: queue.KindNormalizeContent, UniqueKey: "AI-recompute-owner", Payload: queue.Payload{EntityID: 17, EntityVersion: 2},
		ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state='completed',attempt=1,finalized_at=now() WHERE id=$1`, ownerID); err != nil {
		t.Fatal(err)
	}
	runs := intelligencepostgres.NewRepository(runtime)
	profile := recomputeEmbeddingProfile()
	if err := runs.CreateProfile(ctx, &profile); err != nil {
		t.Fatal(err)
	}
	claim, err := runs.Claim(ctx, intelligencepostgres.ClaimInput{
		TaskType: intelligencedomain.TaskTypeEmbedding, WorkspaceKey: "default", SkillID: "content.embedding.v1",
		TargetType: "content", TargetID: 17, TargetVersion: 2, RuntimeVersion: "structured-provider-v1", ModelProfileID: profile.ID,
		PromptVersion: "prompt-v1", InputSchemaVersion: "v1", SchemaVersion: "v1", ParametersVersion: "parameters-v1",
		InputHash: strings.Repeat("a", 64), EvidenceSetHash: strings.Repeat("b", 64), OwningJobID: &ownerID, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Fail(ctx, claim.Run.ID, intelligencedomain.CodeAIOutputInvalid, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	scheduler := NewAIRunRecomputeScheduler(store)
	recomputeID, created, err := scheduler.ScheduleAIRunRecompute(ctx, claim.Run.ID)
	if err != nil || !created {
		t.Fatalf("ScheduleAIRunRecompute() = %d/%t/%v", recomputeID, created, err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET scheduled_at=now()-interval '1 second' WHERE id=$1`, recomputeID); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT args FROM river_job WHERE id=$1`, recomputeID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(stored, &fields); err != nil || len(fields) != 1 || fields["run_id"] != float64(claim.Run.ID) {
		t.Fatalf("durable recompute args = %s / %#v", stored, fields)
	}
	handler := NewAIRunRecomputeHandler(runs, store)
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRecomputeAIRun: handler.Handle})
	claimed, err := worker.RunOnce(ctx)
	if err != nil || !claimed {
		t.Fatalf("RunOnce() = %t/%v", claimed, err)
	}
	var ownerState, recomputeState string
	var ownerAttempt, ownerMaxAttempts int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state,attempt,max_attempts FROM river_job WHERE id=$1`, ownerID).Scan(&ownerState, &ownerAttempt, &ownerMaxAttempts); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE id=$1`, recomputeID).Scan(&recomputeState); err != nil {
		t.Fatal(err)
	}
	if ownerState != "available" || ownerAttempt != 1 || ownerMaxAttempts != 4 || recomputeState != "completed" {
		t.Fatalf("owner=%s attempt=%d max=%d recompute=%s", ownerState, ownerAttempt, ownerMaxAttempts, recomputeState)
	}

	secondID, secondCreated, err := scheduler.ScheduleAIRunRecompute(ctx, claim.Run.ID)
	if err != nil || secondCreated || secondID != recomputeID {
		t.Fatalf("idempotent ScheduleAIRunRecompute() = %d/%t/%v", secondID, secondCreated, err)
	}
}

func recomputeEmbeddingProfile() intelligencedomain.ModelProfile {
	credential := intelligencedomain.OpenAICredentialReference
	dimensions := intelligencedomain.EmbeddingDimensions
	return intelligencedomain.ModelProfile{
		Name: "recompute-embedding", TaskType: intelligencedomain.TaskTypeEmbedding, Provider: intelligencedomain.ProviderOpenAI,
		ModelName: "text-embedding-3-large", ModelVersion: "2026-07", CredentialRef: &credential, EmbeddingDimensions: &dimensions,
		TimeoutSeconds: 30, MaxAttempts: 2, MaxCost: "1.0000", FallbackPriority: 100, Enabled: true,
	}
}
