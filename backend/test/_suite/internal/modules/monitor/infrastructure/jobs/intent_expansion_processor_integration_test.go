package jobs_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/postgres"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/jobs"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestPostgresIntentExpansionHandlerPersistsRealAIRunAuditAndActualModelProvenance(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	defer func() { _ = runtime.Close() }()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}

	monitorID, draftID := insertIntentExpansionRevisionFixture(t, runtime)
	intentRepository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	intents, err := monitorapplication.NewIntentService(monitorapplication.IntentServiceDependencies{
		Drafts: intentRepository, Runs: intentRepository, Clock: sharedclock.System{},
	})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := intents.SubmitExpansionRun(ctx, monitorapplication.SubmitExpansionRunCommand{
		MonitorID: monitorID, DraftID: draftID, ExpectedResourceVersion: 1,
		IdempotencyKey: "real-ai-expansion", ExpansionProfile: monitorapplication.IntentExpansionProfile,
	})
	if err != nil {
		t.Fatalf("SubmitExpansionRun(): %v", err)
	}

	intelligenceRepository := intelligencepostgres.NewRepository(runtime)
	credential := intelligencedomain.OpenAICredentialReference
	dailyBudget := "5.0000"
	profile := intelligencedomain.ModelProfile{
		Name: "monitor-intent-expansion-integration", TaskType: intelligencedomain.TaskTypeTermExpansion,
		Provider: intelligencedomain.ProviderOpenAI, ModelName: "gpt-integration", ModelVersion: "actual-term-model-v7",
		CredentialRef: &credential, TimeoutSeconds: 10, MaxAttempts: 1, MaxCost: "1.0000",
		DailyBudget: &dailyBudget, FallbackPriority: 1, Enabled: true,
	}
	if err := intelligenceRepository.CreateProfile(ctx, &profile); err != nil {
		t.Fatalf("CreateProfile(): %v", err)
	}
	provider := &intentExpansionIntegrationProvider{response: intelligencedomain.StructuredResponse{
		ModelVersion: profile.ModelVersion,
		JSON:         json.RawMessage(`{"terms":[{"term":"service interruption","language":"en","reason":"semantic wording related to the monitoring objective","similarity":0.83,"risk":"medium"}]}`),
		Usage:        intelligencedomain.Usage{InputTokens: 9, OutputTokens: 7},
	}}
	schemas, err := intelligenceapplication.NewSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	aiRuns, err := intelligenceapplication.NewRunService(intelligenceapplication.RunServiceDependencies{
		Runs: intelligenceRepository,
		Providers: intelligenceapplication.NewProviderRegistry(map[intelligencedomain.ProviderName]intelligencedomain.Provider{
			intelligencedomain.ProviderOpenAI: provider,
		}),
		Schemas: schemas, Clock: sharedclock.System{},
	})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := monitorjobs.NewIntentAnalysisCompositeProcessor(intents, aiRuns)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := monitorjobs.NewIntentAnalysisHandler(intents, processor)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := monitorjobs.EncodeIntentAnalysisJobArgs(monitorjobs.IntentAnalysisJobArgs{
		RunID: submitted.Run.ID, DraftID: draftID, DraftResourceVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(ctx, queue.Job{
		Kind: queue.KindAnalyzeMonitorIntent, UniqueKey: "real-ai-expansion", DurableArgs: encoded,
		ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 3,
	}); err != nil {
		t.Fatalf("Handle(): %v", err)
	}

	completed, err := intents.ReadExpansionRun(ctx, monitorapplication.ReadExpansionRunQuery{
		MonitorID: monitorID, DraftID: draftID, DraftResourceVersion: 1, RunID: submitted.Run.ID,
	})
	if err != nil {
		t.Fatalf("ReadExpansionRun(): %v", err)
	}
	if completed.Expansion.Run.Status != "invalidated" || len(completed.Expansion.Candidates) != 1 {
		t.Fatalf("completed expansion = %#v", completed.Expansion)
	}
	candidate := completed.Expansion.Candidates[0]
	if candidate.ModelVersion != profile.ModelVersion || candidate.PromptVersion != monitorjobs.IntentExpansionPromptVersion || candidate.InputHash != submitted.Run.InputHash || candidate.Value != "service interruption" {
		t.Fatalf("candidate actual provenance = %#v", candidate)
	}
	current, err := intentRepository.Find(ctx, monitorapplication.ReadIntentDraftQuery{MonitorID: monitorID, DraftID: draftID})
	if err != nil || current.ResourceVersion != 2 || len(current.Candidates) != 1 || current.Candidates[0].ApprovalStatus != "pending" {
		t.Fatalf("advanced immutable draft = %#v / %v", current, err)
	}

	var modelVersion, promptVersion, parametersVersion, inputHash, targetType, status string
	var targetID, tokens int64
	if err := runtime.SQL.QueryRow(`
SELECT model_version,prompt_version,parameters_version,input_hash,target_type,target_id,status,tokens
FROM ai_runs WHERE task_type='term_expansion' AND target_type=$1 AND target_id=$2`, monitorjobs.IntentExpansionTargetType, submitted.Run.ID).Scan(
		&modelVersion, &promptVersion, &parametersVersion, &inputHash, &targetType, &targetID, &status, &tokens,
	); err != nil {
		t.Fatalf("read AI run audit: %v", err)
	}
	if modelVersion != profile.ModelVersion || promptVersion != monitorjobs.IntentExpansionPromptVersion || parametersVersion != monitorapplication.IntentExpansionProfile ||
		inputHash != submitted.Run.InputHash || targetType != monitorjobs.IntentExpansionTargetType || targetID != submitted.Run.ID || status != "succeeded" || tokens != 16 {
		t.Fatalf("AI run audit = model:%q prompt:%q parameters:%q input:%q target:%q/%d status:%q tokens:%d", modelVersion, promptVersion, parametersVersion, inputHash, targetType, targetID, status, tokens)
	}
	if provider.request.TaskType != intelligencedomain.TaskTypeTermExpansion || provider.request.ModelVersion != profile.ModelVersion ||
		!strings.Contains(string(provider.request.Input), `"objective":"Track launch disruption"`) || strings.Contains(string(provider.request.Input), "raw_body") {
		t.Fatalf("provider request = %#v", provider.request)
	}
}

func insertIntentExpansionRevisionFixture(t *testing.T, runtime *database.Runtime) (int64, int64) {
	t.Helper()
	var monitorID, configID, draftID, revisionID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name,status) VALUES ('intent expansion integration','draft') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id,revision,state) VALUES ($1,1,'draft') RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("insert config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=$2 WHERE id=$1`, monitorID, configID); err != nil {
		t.Fatalf("bind config: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_intent_drafts (monitor_id,config_version_id) VALUES ($1,$2) RETURNING id`, monitorID, configID).Scan(&draftID); err != nil {
		t.Fatalf("insert draft: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_intent_draft_revisions (draft_id,monitor_id,config_version_id,resource_version,objective)
VALUES ($1,$2,$3,1,'Track launch disruption') RETURNING id`, draftID, monitorID, configID).Scan(&revisionID); err != nil {
		t.Fatalf("insert revision: %v", err)
	}
	for ordinal, clause := range []monitorapplication.IntentClauseDTO{
		{Operator: "must", Field: "action", Value: "launch"},
		{Operator: "must_not", Field: "location", Value: "test environment"},
	} {
		if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_intent_clauses (revision_id,draft_id,resource_version,ordinal,operator,field,value)
VALUES ($1,$2,1,$3,$4,$5,$6)`, revisionID, draftID, ordinal, clause.Operator, clause.Field, clause.Value); err != nil {
			t.Fatalf("insert clause: %v", err)
		}
	}
	return monitorID, draftID
}

type intentExpansionIntegrationProvider struct {
	request  intelligencedomain.StructuredRequest
	response intelligencedomain.StructuredResponse
}

func (provider *intentExpansionIntegrationProvider) GenerateStructured(_ context.Context, request intelligencedomain.StructuredRequest) (intelligencedomain.StructuredResponse, error) {
	provider.request = request
	return provider.response, nil
}

func (*intentExpansionIntegrationProvider) Embed(context.Context, intelligencedomain.EmbeddingRequest) (intelligencedomain.EmbeddingResponse, error) {
	return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
}
