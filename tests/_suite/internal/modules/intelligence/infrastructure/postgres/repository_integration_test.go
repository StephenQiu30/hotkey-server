package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestRepositoryClaimsOneReusableRunAndReservesBudget(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	claim := testClaim(profile)
	first, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim() first error = %v", err)
	}
	if first.Reused || first.Run.Status != intelligencedomain.RunStatusQueued || first.Run.ReservedCost != "1.0000" || first.Run.LeaseExpiresAt == nil {
		t.Fatalf("Claim() first = %#v, want a leased reserved queued run", first)
	}
	if _, err := repository.Claim(context.Background(), claim); err == nil {
		t.Fatal("Claim() duplicate in-flight error = nil, want 70007")
	} else if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIRunInProgress {
		t.Fatalf("Claim() duplicate code = %d/%t, want %d", code, ok, intelligencedomain.CodeAIRunInProgress)
	}

	var reserved, settled string
	if err := runtime.SQL.QueryRow(`SELECT reserved_cost::text, settled_cost::text FROM ai_budget_ledgers WHERE model_profile_id = $1 AND budget_day = DATE '2026-07-17'`, profile.ID).Scan(&reserved, &settled); err != nil {
		t.Fatalf("read reserved ledger: %v", err)
	}
	if reserved != "1.0000" || settled != "0.0000" {
		t.Fatalf("ledger reserved/settled = %s/%s, want 1.0000/0.0000", reserved, settled)
	}
	settledRun, err := repository.Settle(context.Background(), first.Run.ID, "0.7500", claim.Now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Settle() error = %v", err)
	}
	if settledRun.Status != intelligencedomain.RunStatusSucceeded || settledRun.ErrorCode != nil {
		t.Fatalf("Settle() = %#v, want successful terminal run", settledRun)
	}
	reused, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim() succeeded reuse error = %v", err)
	}
	if !reused.Reused || reused.Run.ID != first.Run.ID {
		t.Fatalf("Claim() succeeded reuse = %#v, want run %d", reused, first.Run.ID)
	}

	if err := runtime.SQL.QueryRow(`SELECT reserved_cost::text, settled_cost::text FROM ai_budget_ledgers WHERE model_profile_id = $1 AND budget_day = DATE '2026-07-17'`, profile.ID).Scan(&reserved, &settled); err != nil {
		t.Fatalf("read settled ledger: %v", err)
	}
	if reserved != "0.0000" || settled != "0.7500" {
		t.Fatalf("settled ledger reserved/settled = %s/%s, want 0.0000/0.7500", reserved, settled)
	}
}

func TestRepositoryRecordsActualOverageAndBlocksTheProfileDay(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	claim := testClaim(profile)
	first, err := repository.Claim(context.Background(), claim)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	settled, err := repository.Settle(context.Background(), first.Run.ID, "1.2500", claim.Now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Settle(overage) error = %v", err)
	}
	if settled.Status != intelligencedomain.RunStatusFailed || settled.ErrorCode == nil || *settled.ErrorCode != intelligencedomain.CodeAIBudgetExhausted {
		t.Fatalf("Settle(overage) = %#v, want failed 70002", settled)
	}
	if _, err := repository.Claim(context.Background(), testClaim(profile)); err == nil {
		t.Fatal("Claim() after overage error = nil, want 70002")
	} else if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIBudgetExhausted {
		t.Fatalf("Claim() after overage code = %d/%t, want %d", code, ok, intelligencedomain.CodeAIBudgetExhausted)
	}
	nextDay := testClaim(profile)
	nextDay.Now = nextDay.Now.AddDate(0, 0, 1)
	nextDay.InputHash = strings.Repeat("c", 64)
	if _, err := repository.Claim(context.Background(), nextDay); err != nil {
		t.Fatalf("Claim() next UTC day after overage error = %v", err)
	}
	var reserved, settledCost string
	var blocked bool
	if err := runtime.SQL.QueryRow(`SELECT reserved_cost::text, settled_cost::text, overage_blocked FROM ai_budget_ledgers WHERE model_profile_id = $1 AND budget_day = DATE '2026-07-17'`, profile.ID).Scan(&reserved, &settledCost, &blocked); err != nil {
		t.Fatalf("read overage ledger: %v", err)
	}
	if reserved != "0.0000" || settledCost != "1.2500" || !blocked {
		t.Fatalf("overage ledger = %s/%s/%t, want 0.0000/1.2500/true", reserved, settledCost, blocked)
	}
}

func openIntelligenceRuntime(t *testing.T) *database.Runtime {
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

func testEmbeddingProfile() intelligencedomain.ModelProfile {
	credential := intelligencedomain.OpenAICredentialReference
	dimensions := intelligencedomain.EmbeddingDimensions
	dailyBudget := "2.0000"
	return intelligencedomain.ModelProfile{
		Name:                "repository-embedding-profile",
		TaskType:            intelligencedomain.TaskTypeEmbedding,
		Provider:            intelligencedomain.ProviderOpenAI,
		ModelName:           "text-embedding-3-large",
		ModelVersion:        "2026-07",
		CredentialRef:       &credential,
		EmbeddingDimensions: &dimensions,
		TimeoutSeconds:      30,
		MaxAttempts:         2,
		MaxCost:             "1.0000",
		DailyBudget:         &dailyBudget,
		FallbackPriority:    100,
		Enabled:             true,
	}
}

func testClaim(profile intelligencedomain.ModelProfile) intelligencepostgres.ClaimInput {
	return intelligencepostgres.ClaimInput{
		TaskType:           intelligencedomain.TaskTypeEmbedding,
		TargetType:         "content",
		TargetID:           1,
		ModelProfileID:     profile.ID,
		PromptVersion:      "prompt-v1",
		InputSchemaVersion: "v1",
		SchemaVersion:      "v1",
		ParametersVersion:  "parameters-v1",
		InputHash:          strings.Repeat("a", 64),
		EvidenceSetHash:    strings.Repeat("b", 64),
		Now:                time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC),
	}
}
