package application

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestRunServiceSettlesSafeStructuredResultAndReusesIt(t *testing.T) {
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	profile := applicationTermProfile()
	if err := runs.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile(): %v", err)
	}
	provider := &applicationFakeProvider{structured: []domain.StructuredResponse{{
		ModelVersion: profile.ModelVersion, JSON: json.RawMessage(`{"terms":[{"term":"hotkey","language":"en"}]}`), Usage: domain.Usage{InputTokens: 3, OutputTokens: 2},
	}}}
	clock := &applicationClock{value: time.Date(2026, time.July, 17, 11, 0, 0, 0, time.UTC)}
	service := newApplicationRunService(t, runs, provider, clock)
	input := applicationStructuredInput()

	first, err := service.ExecuteStructured(context.Background(), input)
	if err != nil {
		t.Fatalf("ExecuteStructured(first): %v", err)
	}
	if first.Status != "succeeded" || first.Reused || !json.Valid(first.Result) || first.Run.Tokens != 5 || first.Run.Cost != "1.0000" {
		t.Fatalf("first structured result = %#v, want persisted success", first)
	}
	second, err := service.ExecuteStructured(context.Background(), input)
	if err != nil {
		t.Fatalf("ExecuteStructured(reuse): %v", err)
	}
	if !second.Reused || second.Run.ID != first.Run.ID || string(second.Result) != string(first.Result) || provider.structuredCalls() != 1 {
		t.Fatalf("reused structured result = %#v calls=%d, want exact persisted result and one provider call", second, provider.structuredCalls())
	}
	var reserved, settled string
	if err := runtime.SQL.QueryRow(`SELECT reserved_cost::text,settled_cost::text FROM ai_budget_ledgers WHERE model_profile_id=$1`, profile.ID).Scan(&reserved, &settled); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if reserved != "0.0000" || settled != "1.0000" {
		t.Fatalf("ledger reserved/settled = %s/%s, want 0/claimed max_cost", reserved, settled)
	}
}

func TestPlan009RelevanceReviewContract(t *testing.T) {
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	profile := applicationRelevanceReviewProfile()
	if err := runs.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile(relevance_review): %v", err)
	}
	provider := &applicationFakeProvider{structured: []domain.StructuredResponse{{
		ModelVersion: profile.ModelVersion,
		JSON:         json.RawMessage(`{"decision":"review","score":70,"reason_codes":["ambiguous_context"]}`),
		Usage:        domain.Usage{InputTokens: 3, OutputTokens: 2},
	}}}
	clock := &applicationClock{value: time.Date(2026, time.July, 17, 11, 0, 0, 0, time.UTC)}
	service := newApplicationRunService(t, runs, provider, clock)
	invalidTarget := applicationRelevanceReviewInput()
	invalidTarget.TargetType = "content"
	if _, err := service.ExecuteStructured(context.Background(), invalidTarget); err == nil {
		t.Fatal("ExecuteStructured(relevance_review content target) error = nil, want rejection")
	} else if code, ok := domain.CodeOf(err); !ok || code != domain.CodeAIModelProfileInvalid {
		t.Fatalf("ExecuteStructured(relevance_review content target) code = %d/%t, want %d", code, ok, domain.CodeAIModelProfileInvalid)
	}
	if calls := provider.structuredCalls(); calls != 0 {
		t.Fatalf("provider calls after rejected relevance target = %d, want 0", calls)
	}
	result, err := service.ExecuteStructured(context.Background(), applicationRelevanceReviewInput())
	if err != nil {
		t.Fatalf("ExecuteStructured(relevance_review): %v", err)
	}
	var response struct {
		Decision    string   `json:"decision"`
		Score       float64  `json:"score"`
		ReasonCodes []string `json:"reason_codes"`
	}
	if err := json.Unmarshal(result.Result, &response); err != nil {
		t.Fatalf("decode relevance-review result: %v", err)
	}
	if result.Status != "succeeded" || result.Run.TaskType != domain.TaskTypeRelevanceReview || response.Decision != "review" || response.Score != 70 || len(response.ReasonCodes) != 1 || response.ReasonCodes[0] != "ambiguous_context" {
		t.Fatalf("relevance-review result = %#v", result)
	}
}

func TestPlan009RelevanceReviewFacadeKeepsSingleOwnerAndProfileScopedReuse(t *testing.T) {
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	profile := applicationRelevanceReviewProfile()
	if err := runs.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile(relevance_review): %v", err)
	}
	provider := &blockingStructuredProvider{
		started: make(chan struct{}), release: make(chan struct{}),
		response: domain.StructuredResponse{ModelVersion: profile.ModelVersion, JSON: json.RawMessage(`{"decision":"accepted","score":80,"reason_codes":["relevant_evidence"]}`)},
	}
	clock := &applicationClock{value: time.Date(2026, time.July, 17, 13, 30, 0, 0, time.UTC)}
	runService := newApplicationRunService(t, runs, provider, clock)
	service, err := NewRelevanceReviewService(runService)
	if err != nil {
		t.Fatalf("NewRelevanceReviewService(): %v", err)
	}
	input := applicationRelevanceReviewFacadeInput()

	firstDone := make(chan struct {
		result RelevanceReviewResult
		err    error
	}, 1)
	go func() {
		result, err := service.Review(context.Background(), input)
		firstDone <- struct {
			result RelevanceReviewResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-provider.started:
	case first := <-firstDone:
		t.Fatalf("Review(first) returned before provider started: %#v / %v", first.result, first.err)
	case <-time.After(15 * time.Second):
		t.Fatal("Review(first) did not reach the provider within 15s")
	}
	inflight, err := service.Review(context.Background(), input)
	if err != nil || inflight.Status != "degraded" || inflight.ReasonCode != "ai_in_progress" || provider.structuredCalls() != 1 {
		t.Fatalf("Review(in-flight) = %#v / %v calls=%d, want one owner", inflight, err, provider.structuredCalls())
	}
	close(provider.release)
	first := <-firstDone
	if first.err != nil || first.result.Status != "succeeded" || first.result.Reused || first.result.RunID <= 0 || first.result.Decision != "accepted" || first.result.Score != 80 {
		t.Fatalf("Review(first) = %#v / %v", first.result, first.err)
	}
	reused, err := service.Review(context.Background(), input)
	if err != nil || reused.Status != "succeeded" || !reused.Reused || reused.RunID != first.result.RunID || provider.structuredCalls() != 1 {
		t.Fatalf("Review(reuse) = %#v / %v calls=%d", reused, err, provider.structuredCalls())
	}

	profile.TimeoutSeconds = 11
	if _, err := runs.UpdateProfile(context.Background(), profile, profile.Version); err != nil {
		t.Fatalf("UpdateProfile(): %v", err)
	}
	updated, err := service.Review(context.Background(), input)
	if err != nil || updated.Status != "succeeded" || updated.Reused || updated.RunID == first.result.RunID || provider.structuredCalls() != 2 {
		t.Fatalf("Review(profile revision) = %#v / %v calls=%d, want fresh run", updated, err, provider.structuredCalls())
	}
}

func TestPlan009RelevanceReviewFacadeDegradesWithoutProviderOrBudget(t *testing.T) {
	t.Run("provider unavailable", func(t *testing.T) {
		runtime := openApplicationRuntime(t)
		defer func() { _ = runtime.Close() }()
		runs := intelligencepostgres.NewRepository(runtime)
		profile := applicationRelevanceReviewProfile()
		if err := runs.CreateProfile(context.Background(), &profile); err != nil {
			t.Fatalf("CreateProfile(): %v", err)
		}
		schemas, err := NewSchemaRegistry()
		if err != nil {
			t.Fatalf("NewSchemaRegistry(): %v", err)
		}
		runService, err := NewRunService(RunServiceDependencies{Runs: runs, Providers: NewProviderRegistry(nil), Schemas: schemas, Clock: &applicationClock{value: time.Date(2026, time.July, 17, 14, 0, 0, 0, time.UTC)}})
		if err != nil {
			t.Fatalf("NewRunService(): %v", err)
		}
		service, err := NewRelevanceReviewService(runService)
		if err != nil {
			t.Fatalf("NewRelevanceReviewService(): %v", err)
		}
		result, err := service.Review(context.Background(), applicationRelevanceReviewFacadeInput())
		if err != nil || result.Status != "degraded" || result.ReasonCode != "ai_unavailable" {
			t.Fatalf("Review(without provider) = %#v / %v", result, err)
		}
	})

	t.Run("budget exhausted", func(t *testing.T) {
		runtime := openApplicationRuntime(t)
		defer func() { _ = runtime.Close() }()
		runs := intelligencepostgres.NewRepository(runtime)
		profile := applicationRelevanceReviewProfile()
		daily := "1.0000"
		profile.DailyBudget = &daily
		if err := runs.CreateProfile(context.Background(), &profile); err != nil {
			t.Fatalf("CreateProfile(): %v", err)
		}
		clock := &applicationClock{value: time.Date(2026, time.July, 17, 14, 0, 0, 0, time.UTC)}
		if _, err := runs.Claim(context.Background(), intelligencepostgres.ClaimInput{
			TaskType: domain.TaskTypeRelevanceReview, TargetType: "monitor_match", TargetID: 100, ModelProfileID: profile.ID,
			PromptVersion: relevanceReviewPromptVersion, InputSchemaVersion: relevanceReviewInputSchemaVersion, SchemaVersion: relevanceReviewSchemaVersion,
			ParametersVersion: relevanceReviewParametersVersion, InputHash: strings.Repeat("1", 64), EvidenceSetHash: strings.Repeat("2", 64), Now: clock.Now(),
		}); err != nil {
			t.Fatalf("Claim(reserve budget): %v", err)
		}
		provider := &applicationFakeProvider{}
		runService := newApplicationRunService(t, runs, provider, clock)
		service, err := NewRelevanceReviewService(runService)
		if err != nil {
			t.Fatalf("NewRelevanceReviewService(): %v", err)
		}
		result, err := service.Review(context.Background(), applicationRelevanceReviewFacadeInput())
		if err != nil || result.Status != "degraded" || result.ReasonCode != "ai_unavailable" || provider.structuredCalls() != 0 {
			t.Fatalf("Review(budget exhausted) = %#v / %v calls=%d", result, err, provider.structuredCalls())
		}
	})
}

func TestRunServiceRetriesOnlyTransientProviderFailures(t *testing.T) {
	for _, testCase := range []struct {
		name, hash string
		code       int
	}{
		{name: "rate limited", hash: "c", code: domain.CodeAIProviderRateLimited},
		{name: "transient 5xx", hash: "d", code: domain.CodeAIProviderTransient},
		{name: "deadline", hash: "e", code: domain.CodeAIProviderTimeout},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := openApplicationRuntime(t)
			defer func() { _ = runtime.Close() }()
			runs := intelligencepostgres.NewRepository(runtime)
			profile := applicationTermProfile()
			profile.Name = "retry-profile-" + strings.ReplaceAll(testCase.name, " ", "-")
			profile.MaxAttempts = 2
			if err := runs.CreateProfile(context.Background(), &profile); err != nil {
				t.Fatalf("CreateProfile(): %v", err)
			}
			provider := &applicationFakeProvider{structuredErrors: []error{domain.NewError(testCase.code)}, structured: []domain.StructuredResponse{{
				ModelVersion: profile.ModelVersion, JSON: json.RawMessage(`{"terms":[]}`), Usage: domain.Usage{},
			}}}
			clock := &applicationClock{value: time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)}
			registry, err := NewSchemaRegistry()
			if err != nil {
				t.Fatalf("NewSchemaRegistry(): %v", err)
			}
			service, err := NewRunService(RunServiceDependencies{
				Runs: runs, Providers: NewProviderRegistry(map[domain.ProviderName]domain.Provider{domain.ProviderOpenAI: provider}), Schemas: registry, Clock: clock,
				Sleep: func(_ context.Context, delay time.Duration) error { clock.advance(delay); return nil },
			})
			if err != nil {
				t.Fatalf("NewRunService(): %v", err)
			}
			input := applicationStructuredInput()
			input.InputHash = strings.Repeat(testCase.hash, 64)
			result, err := service.ExecuteStructured(context.Background(), input)
			if err != nil || result.Status != "succeeded" || provider.structuredCalls() != 2 {
				t.Fatalf("ExecuteStructured(retry) = %#v / %v calls=%d", result, err, provider.structuredCalls())
			}
			var attempt int
			if err := runtime.SQL.QueryRow(`SELECT attempt FROM ai_runs WHERE id=$1`, result.Run.ID).Scan(&attempt); err != nil {
				t.Fatalf("read retry attempt: %v", err)
			}
			if attempt != 2 {
				t.Fatalf("attempt = %d, want 2", attempt)
			}
		})
	}
}

func TestRunServiceConcurrentIdenticalRequestKeepsSingleProviderOwner(t *testing.T) {
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	profile := applicationTermProfile()
	profile.Name = "concurrent-profile"
	if err := runs.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile(): %v", err)
	}
	provider := &blockingStructuredProvider{
		started: make(chan struct{}), release: make(chan struct{}),
		response: domain.StructuredResponse{ModelVersion: profile.ModelVersion, JSON: json.RawMessage(`{"terms":[]}`)},
	}
	clock := &applicationClock{value: time.Date(2026, time.July, 17, 12, 30, 0, 0, time.UTC)}
	service := newApplicationRunService(t, runs, provider, clock)
	input := applicationStructuredInput()
	input.InputHash = strings.Repeat("f", 64)
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.ExecuteStructured(context.Background(), input)
		firstDone <- err
	}()
	<-provider.started
	if _, err := service.ExecuteStructured(context.Background(), input); err == nil {
		t.Fatal("concurrent ExecuteStructured() error = nil, want in-progress")
	} else if code, ok := domain.CodeOf(err); !ok || code != domain.CodeAIRunInProgress {
		t.Fatalf("concurrent ExecuteStructured() code = %d/%t, want 70007", code, ok)
	}
	close(provider.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first ExecuteStructured() error = %v", err)
	}
}

func TestRunServicePersistsOneRepairAndAggregatesUsage(t *testing.T) {
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	profile := applicationTermProfile()
	profile.Name = "repair-profile"
	if err := runs.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile(): %v", err)
	}
	provider := &applicationFakeProvider{structured: []domain.StructuredResponse{
		{ModelVersion: profile.ModelVersion, JSON: json.RawMessage(`{"terms":[{"term":1,"language":"en"}]}`), Usage: domain.Usage{InputTokens: 2, OutputTokens: 3}},
		{ModelVersion: profile.ModelVersion, JSON: json.RawMessage(`{"terms":[]}`), Usage: domain.Usage{InputTokens: 4, OutputTokens: 5}},
	}}
	clock := &applicationClock{value: time.Date(2026, time.July, 17, 12, 45, 0, 0, time.UTC)}
	service := newApplicationRunService(t, runs, provider, clock)
	input := applicationStructuredInput()
	input.InputHash = strings.Repeat("1", 64)
	result, err := service.ExecuteStructured(context.Background(), input)
	if err != nil || result.Status != "succeeded" || provider.structuredCalls() != 2 || result.Run.Tokens != 14 {
		t.Fatalf("ExecuteStructured(repair) = %#v / %v calls=%d, want aggregated token success", result, err, provider.structuredCalls())
	}
	var repairAttempted bool
	if err := runtime.SQL.QueryRow(`SELECT repair_attempted FROM ai_runs WHERE id=$1`, result.Run.ID).Scan(&repairAttempted); err != nil {
		t.Fatalf("read repair attempt: %v", err)
	}
	if !repairAttempted {
		t.Fatal("repair_attempted = false, want persisted true")
	}
}

func TestRunServiceFallsBackAfterHigherPriorityBudgetIsUnavailable(t *testing.T) {
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	first := applicationTermProfile()
	first.Name, first.FallbackPriority = "budget-exhausted-first", 10
	daily := "1.0000"
	first.DailyBudget = &daily
	if err := runs.CreateProfile(context.Background(), &first); err != nil {
		t.Fatalf("CreateProfile(first): %v", err)
	}
	second := applicationTermProfile()
	second.Name, second.ModelVersion, second.FallbackPriority = "budget-fallback-second", "term-v2", 20
	if err := runs.CreateProfile(context.Background(), &second); err != nil {
		t.Fatalf("CreateProfile(second): %v", err)
	}
	clock := &applicationClock{value: time.Date(2026, time.July, 17, 13, 0, 0, 0, time.UTC)}
	if _, err := runs.Claim(context.Background(), intelligencepostgres.ClaimInput{
		TaskType: domain.TaskTypeTermExpansion, TargetType: "monitor", TargetID: 1, ModelProfileID: first.ID,
		PromptVersion: "prompt-v1", InputSchemaVersion: "v1", SchemaVersion: "v1", ParametersVersion: "params-v1",
		InputHash: strings.Repeat("2", 64), EvidenceSetHash: strings.Repeat("3", 64), Now: clock.Now(),
	}); err != nil {
		t.Fatalf("reserve higher-priority profile: %v", err)
	}
	provider := &applicationFakeProvider{structured: []domain.StructuredResponse{{ModelVersion: second.ModelVersion, JSON: json.RawMessage(`{"terms":[]}`)}}}
	service := newApplicationRunService(t, runs, provider, clock)
	input := applicationStructuredInput()
	input.InputHash = strings.Repeat("4", 64)
	result, err := service.ExecuteStructured(context.Background(), input)
	if err != nil || result.Status != "succeeded" || result.Run.ModelProfileID != second.ID || provider.structuredCalls() != 1 {
		t.Fatalf("ExecuteStructured(fallback) = %#v / %v calls=%d, want second profile", result, err, provider.structuredCalls())
	}
}

func TestEmbeddingServiceDegradesWithoutProviderAndReusesExactRunVector(t *testing.T) {
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	profile := applicationEmbeddingProfile()
	if err := runs.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile(): %v", err)
	}
	clock := &applicationClock{value: time.Date(2026, time.July, 17, 13, 0, 0, 0, time.UTC)}
	schema, err := NewSchemaRegistry()
	if err != nil {
		t.Fatalf("NewSchemaRegistry(): %v", err)
	}
	withoutProvider, err := NewRunService(RunServiceDependencies{Runs: runs, Providers: NewProviderRegistry(nil), Schemas: schema, Clock: clock})
	if err != nil {
		t.Fatalf("NewRunService(): %v", err)
	}
	degraded, err := NewEmbeddingService(EmbeddingServiceDependencies{Runs: runs, Providers: NewProviderRegistry(nil), RunService: withoutProvider})
	if err != nil {
		t.Fatalf("NewEmbeddingService(): %v", err)
	}
	result, err := degraded.Execute(context.Background(), applicationEmbeddingInput(1))
	if err != nil || result.Status != "degraded" || result.ReasonCode != degradedReasonModelUnavailable {
		t.Fatalf("degraded Execute() = %#v / %v", result, err)
	}

	var monitorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ('application embedding monitor') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("seed monitor: %v", err)
	}
	provider := &applicationFakeProvider{embedding: []domain.EmbeddingResponse{{ModelVersion: profile.ModelVersion, Vectors: [][]float32{applicationVector()}, Usage: domain.Usage{InputTokens: 4}}}}
	runService := newApplicationRunService(t, runs, provider, clock)
	service, err := NewEmbeddingService(EmbeddingServiceDependencies{
		Runs: runs, Providers: NewProviderRegistry(map[domain.ProviderName]domain.Provider{domain.ProviderOpenAI: provider}), RunService: runService,
	})
	if err != nil {
		t.Fatalf("NewEmbeddingService(): %v", err)
	}
	input := applicationEmbeddingInput(monitorID)
	first, err := service.Execute(context.Background(), input)
	if err != nil || first.Status != "succeeded" || first.Reused || len(first.Vector) != domain.EmbeddingDimensions {
		t.Fatalf("Execute(first) = %#v / %v", first, err)
	}
	second, err := service.Execute(context.Background(), input)
	if err != nil || !second.Reused || second.Run.ID != first.Run.ID || provider.embeddingCalls() != 1 {
		t.Fatalf("Execute(reuse) = %#v / %v calls=%d", second, err, provider.embeddingCalls())
	}
	var runID int64
	if err := runtime.SQL.QueryRow(`SELECT ai_run_id FROM monitor_embeddings WHERE monitor_id=$1 AND active`, monitorID).Scan(&runID); err != nil {
		t.Fatalf("read embedding provenance: %v", err)
	}
	if runID != first.Run.ID {
		t.Fatalf("embedding ai_run_id = %d, want %d", runID, first.Run.ID)
	}
}

func openApplicationRuntime(t *testing.T) *database.Runtime {
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

func newApplicationRunService(t *testing.T, runs *intelligencepostgres.Repository, provider domain.Provider, clock *applicationClock) *RunService {
	t.Helper()
	schemas, err := NewSchemaRegistry()
	if err != nil {
		t.Fatalf("NewSchemaRegistry(): %v", err)
	}
	service, err := NewRunService(RunServiceDependencies{
		Runs: runs, Providers: NewProviderRegistry(map[domain.ProviderName]domain.Provider{domain.ProviderOpenAI: provider}), Schemas: schemas, Clock: clock,
		Sleep: func(_ context.Context, delay time.Duration) error { clock.advance(delay); return nil },
	})
	if err != nil {
		t.Fatalf("NewRunService(): %v", err)
	}
	return service
}

func applicationTermProfile() domain.ModelProfile {
	credential := domain.OpenAICredentialReference
	daily := "5.0000"
	return domain.ModelProfile{Name: "application-term-profile", TaskType: domain.TaskTypeTermExpansion, Provider: domain.ProviderOpenAI,
		ModelName: "gpt-test", ModelVersion: "term-v1", CredentialRef: &credential, TimeoutSeconds: 10, MaxAttempts: 2,
		MaxCost: "1.0000", DailyBudget: &daily, FallbackPriority: 10, Enabled: true}
}

func applicationRelevanceReviewProfile() domain.ModelProfile {
	credential := domain.OpenAICredentialReference
	daily := "5.0000"
	return domain.ModelProfile{Name: "application-relevance-review-profile", TaskType: domain.TaskTypeRelevanceReview, Provider: domain.ProviderOpenAI,
		ModelName: "gpt-5.6sol", ModelVersion: "2026-07", CredentialRef: &credential, TimeoutSeconds: 10, MaxAttempts: 2,
		MaxCost: "1.0000", DailyBudget: &daily, FallbackPriority: 10, Enabled: true}
}

func applicationEmbeddingProfile() domain.ModelProfile {
	credential := domain.OpenAICredentialReference
	dimensions := domain.EmbeddingDimensions
	return domain.ModelProfile{Name: "application-embedding-profile", TaskType: domain.TaskTypeEmbedding, Provider: domain.ProviderOpenAI,
		ModelName: "text-embedding-test", ModelVersion: "embedding-v1", CredentialRef: &credential, EmbeddingDimensions: &dimensions,
		TimeoutSeconds: 10, MaxAttempts: 2, MaxCost: "1.0000", FallbackPriority: 10, Enabled: true}
}

func applicationStructuredInput() StructuredExecutionInput {
	return StructuredExecutionInput{TaskType: domain.TaskTypeTermExpansion, TargetType: "monitor", TargetID: 99,
		PromptVersion: "prompt-v1", InputSchemaVersion: "v1", SchemaVersion: "v1", ParametersVersion: "params-v1",
		InputHash: strings.Repeat("a", 64), EvidenceSetHash: strings.Repeat("b", 64), Input: json.RawMessage(`{"intent":"hotkey","terms":["hotkey"],"language":"en"}`)}
}

func applicationRelevanceReviewInput() StructuredExecutionInput {
	return StructuredExecutionInput{TaskType: domain.TaskTypeRelevanceReview, TargetType: "monitor_match", TargetID: 99,
		PromptVersion: "relevance-review-v1", InputSchemaVersion: "v1", SchemaVersion: "v1", ParametersVersion: "relevance-v1",
		InputHash: strings.Repeat("c", 64), EvidenceSetHash: strings.Repeat("d", 64), Input: json.RawMessage(`{"content_excerpt":"A verified OpenAI product announcement.","content_language":"en","monitor_intent":"Track OpenAI product releases.","scoring_version":"relevance-v1","scores":{"semantic":70,"lexical":80,"entity":60,"title":70,"preference":50},"recall_paths":["lexical","vector"],"reason_codes":["lexical_candidate"],"evidence_terms":["OpenAI"]}`)}
}

func applicationRelevanceReviewFacadeInput() RelevanceReviewRequest {
	return RelevanceReviewRequest{TargetID: 99, InputHash: strings.Repeat("c", 64),
		ContentExcerpt: "A verified OpenAI product announcement.", ContentLanguage: "en", MonitorIntent: "Track OpenAI product releases.", ScoringVersion: "relevance-v1",
		Scores: RelevanceReviewScores{Semantic: 70, Lexical: 80, Entity: 60, Title: 70, Preference: 50}, RecallPaths: []string{"lexical", "vector"},
		ReasonCodes: []string{"lexical_candidate", "vector_candidate", "low_confidence"}, EvidenceTerms: []string{"OpenAI"}}
}

func applicationEmbeddingInput(targetID int64) EmbeddingExecutionInput {
	return EmbeddingExecutionInput{Target: intelligencepostgres.EmbeddingTargetMonitor, TargetID: targetID,
		PromptVersion: "prompt-v1", InputSchemaVersion: "v1", SchemaVersion: "v1", ParametersVersion: "params-v1",
		InputHash: strings.Repeat("d", 64), EvidenceSetHash: strings.Repeat("e", 64), Input: "hotkey query", QueryText: "hotkey query"}
}

func applicationVector() []float32 {
	vector := make([]float32, domain.EmbeddingDimensions)
	vector[0] = 1
	return vector
}

type applicationClock struct {
	mu    sync.Mutex
	value time.Time
}

func (clock *applicationClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.value
}

func (clock *applicationClock) advance(duration time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.value = clock.value.Add(duration)
}

type applicationFakeProvider struct {
	mu               sync.Mutex
	structured       []domain.StructuredResponse
	structuredErrors []error
	embedding        []domain.EmbeddingResponse
	structuredCount  int
	embeddingCount   int
}

type blockingStructuredProvider struct {
	started  chan struct{}
	release  chan struct{}
	response domain.StructuredResponse
	once     sync.Once
	mu       sync.Mutex
	calls    int
}

func (provider *blockingStructuredProvider) GenerateStructured(ctx context.Context, _ domain.StructuredRequest) (domain.StructuredResponse, error) {
	provider.mu.Lock()
	provider.calls++
	provider.mu.Unlock()
	provider.once.Do(func() { close(provider.started) })
	select {
	case <-ctx.Done():
		return domain.StructuredResponse{}, domain.NewError(domain.CodeAIProviderTimeout)
	case <-provider.release:
		return provider.response, nil
	}
}

func (provider *blockingStructuredProvider) structuredCalls() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *blockingStructuredProvider) Embed(context.Context, domain.EmbeddingRequest) (domain.EmbeddingResponse, error) {
	return domain.EmbeddingResponse{}, domain.NewError(domain.CodeAIModelUnavailable)
}

func (provider *applicationFakeProvider) GenerateStructured(_ context.Context, _ domain.StructuredRequest) (domain.StructuredResponse, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	index := provider.structuredCount
	provider.structuredCount++
	if index < len(provider.structuredErrors) {
		return domain.StructuredResponse{}, provider.structuredErrors[index]
	}
	responseIndex := index - len(provider.structuredErrors)
	if responseIndex >= len(provider.structured) {
		return domain.StructuredResponse{}, domain.NewError(domain.CodeAIModelUnavailable)
	}
	return provider.structured[responseIndex], nil
}

func (provider *applicationFakeProvider) Embed(_ context.Context, _ domain.EmbeddingRequest) (domain.EmbeddingResponse, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	index := provider.embeddingCount
	provider.embeddingCount++
	if index >= len(provider.embedding) {
		return domain.EmbeddingResponse{}, domain.NewError(domain.CodeAIModelUnavailable)
	}
	return provider.embedding[index], nil
}

func (provider *applicationFakeProvider) structuredCalls() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.structuredCount
}

func (provider *applicationFakeProvider) embeddingCalls() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.embeddingCount
}
