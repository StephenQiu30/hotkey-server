package architecture_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIReplacementInventoryCoversCurrentCallersAndDataDisposition(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")

	callers := []string{
		"backend/internal/modules/intelligence/infrastructure/agent/client.go",
		"backend/internal/modules/intelligence/domain/provider.go",
		"backend/internal/bootstrap/app.go",
		"backend/internal/modules/intelligence/application/run_service.go",
		"backend/internal/modules/intelligence/application/projected_embedding_service.go",
		"backend/internal/bootstrap/monitor_intent_expansion.go",
		"backend/internal/bootstrap/monitor_intent_embedding.go",
		"backend/internal/bootstrap/document_embedding.go",
		"backend/internal/modules/ingestion/application/hybrid_recall.go",
		"backend/internal/modules/ingestion/infrastructure/postgres/hybrid_recall_reader.go",
		"backend/internal/modules/event/infrastructure/postgres/candidate_recall.go",
		"backend/internal/modules/intelligence/transport/http/routes.go",
		"frontend/src/services/hotkey/hotkey-server/ai.ts",
		"backend/internal/modules/report",
		"backend/internal/modules/operations/infrastructure/postgres/runtime_metrics.go",
	}
	for _, caller := range callers {
		if !strings.Contains(plan, "`"+caller+"`") {
			t.Errorf("S05 inventory is missing caller %s", caller)
		}
	}

	dataAssets := []string{
		"ai_model_profiles",
		"ai_runs",
		"ai_run_evidences",
		"ai_budget_ledgers",
		"content_embeddings",
		"monitor_embeddings",
		"event_embeddings",
		"topic_embeddings",
		"monitor_compiled_profiles",
		"monitor_compiled_intent_embeddings",
		"document_version_search_indexes",
		"document_version_embeddings",
	}
	for _, asset := range dataAssets {
		if !strings.Contains(plan, "`"+asset+"`") {
			t.Errorf("S05 inventory is missing data disposition for %s", asset)
		}
	}

	for _, decision := range []string{
		"当前 Provider Registry 只注册 OpenAI、DeepSeek、Ollama 与 ONNX",
		"Go Codex CLI Adapter 已实现但尚未进入 ProviderName、Model Profile Schema 或生产 Registry",
		"Python Agent 仅接入 Go Worker 的可选 Shadow 比较路径",
		"无第一方页面直接调用模型 Profile 管理客户端",
		"报告模块不直接调用 Provider、Embedding、Codex 或 Agent",
		"旧记录继续按原 Model Profile、Run 与向量版本可读",
		"本阶段禁止物理删除",
	} {
		if !strings.Contains(plan, decision) {
			t.Errorf("S05 inventory is missing current fact or disposition %q", decision)
		}
	}
}

func TestAIReplacementInventoryRecordsPythonAgentMigrationBoundary(t *testing.T) {
	root := repositoryRoot(t)
	repository := filepath.Clean(filepath.Join(root, ".."))
	providerDomain := readRepositoryFile(t, root, "internal/modules/intelligence/domain/provider.go")
	bootstrap := readRepositoryFile(t, root, "internal/bootstrap/app.go")
	codexProvider := readRepositoryFile(t, root, "internal/modules/intelligence/infrastructure/provider/codex_cli_provider.go")
	agentClient := readRepositoryFile(t, root, "internal/modules/intelligence/infrastructure/agent/client.go")
	agentMain := readRepositoryFile(t, repository, "agent/src/hotkey_agent/main.py")

	if strings.Contains(providerDomain, "ProviderCodex") {
		t.Fatal("Codex unexpectedly entered the production ProviderName contract before S05 switch tests")
	}
	if strings.Contains(bootstrap, "NewCodexCLIProvider") {
		t.Fatal("Codex unexpectedly entered the production registry before S05 switch tests")
	}
	if !strings.Contains(codexProvider, "type CodexCLIProvider struct") {
		t.Fatal("controlled Codex provider adapter is missing from the replacement inventory")
	}
	if !strings.Contains(agentMain, "FastAPI") || !strings.Contains(agentMain, `"/v1/analyze"`) {
		t.Fatal("approved Python Agent analysis boundary is missing")
	}
	if !strings.Contains(agentClient, "type Client struct") || !strings.Contains(bootstrap, "newAgentShadowRunner") || !strings.Contains(bootstrap, "NewShadowRunner") {
		t.Fatal("Python Agent client must enter only the bounded Shadow bootstrap path before S05 switch tests")
	}
	providerRegistryStart := strings.Index(bootstrap, "func newAIProviderRegistry")
	shadowRunnerStart := strings.Index(bootstrap, "func newAgentShadowRunner")
	if providerRegistryStart < 0 || shadowRunnerStart <= providerRegistryStart || strings.Contains(bootstrap[providerRegistryStart:shadowRunnerStart], "intelligenceagent") {
		t.Fatal("Python Agent unexpectedly entered the production Provider registry before S05 switch tests")
	}
	runServiceStart := strings.Index(bootstrap, "func newAIRunService")
	if runServiceStart <= shadowRunnerStart {
		t.Fatal("Python Agent Shadow runner is not isolated from the primary RunService constructor")
	}
	shadowBootstrap := bootstrap[shadowRunnerStart:runServiceStart]
	observerStart := strings.Index(shadowBootstrap, "Observe:")
	if observerStart < 0 {
		t.Fatal("Agent Shadow bootstrap is missing a sanitized observation sink")
	}
	observerBlock := shadowBootstrap[observerStart:]
	for _, forbidden := range []string{"PrimaryOutputSHA256", "ShadowOutputSHA256", "AuthToken", "Prompt", "StructuredResult"} {
		if strings.Contains(observerBlock, forbidden) {
			t.Fatalf("Agent Shadow bootstrap logs forbidden high-cardinality or secret-bearing field %q", forbidden)
		}
	}
}

func TestAgentQualityFrameworkCannotMasqueradeAsLiveApproval(t *testing.T) {
	root := repositoryRoot(t)
	repository := filepath.Clean(filepath.Join(root, ".."))
	bootstrap := readRepositoryFile(t, root, "internal/bootstrap/app.go")
	command := readRepositoryFile(t, root, "internal/bootstrap/agent_quality_command.go")
	evaluator := readRepositoryFile(t, root, "internal/modules/intelligence/application/shadow_quality.go")
	fixture := readRepositoryFile(t, root, "test/fixtures/agent-shadow/v1/golden-dataset.json")
	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")

	if !strings.Contains(bootstrap, `args[0] == "agent-quality"`) || !strings.Contains(bootstrap, "runAgentQualityCommand") {
		t.Fatal("trusted Agent quality command is not registered")
	}
	for _, required := range []string{"ShadowQualityStatusCandidate", "ApprovalReady: false", "quality_thresholds_not_approved", "human_review_incomplete"} {
		if !strings.Contains(evaluator, required) {
			t.Fatalf("Agent quality evaluator is missing non-approval guard %q", required)
		}
	}
	if strings.Contains(command, `"activate"`) || strings.Contains(command, `"live"`) {
		t.Fatal("candidate quality command unexpectedly exposes activation or Live switching")
	}
	for _, required := range []string{
		"backend/test/fixtures/agent-shadow/v1/golden-dataset.json",
		"backend/internal/modules/intelligence/application/shadow_quality.go",
		"candidate",
		"approval_ready=false",
		"不能勾选 `CHK-003-G5-001`",
	} {
		if !strings.Contains(plan, required) {
			t.Fatalf("S05 T02 evidence is missing %q", required)
		}
	}

	var dataset struct {
		AnnotatorCount int `json:"annotator_count"`
		Samples        []struct {
			TaskType      string `json:"task_type"`
			SchemaVersion string `json:"schema_version"`
		} `json:"samples"`
	}
	if err := json.Unmarshal([]byte(fixture), &dataset); err != nil {
		t.Fatalf("decode Agent Golden fixture: %v", err)
	}
	if dataset.AnnotatorCount != 0 || len(dataset.Samples) != 5 {
		t.Fatalf("unreviewed fixed fixture = %#v", dataset)
	}
	wanted := map[string]bool{
		"term_expansion/v1": false, "relevance_review/v1": false, "event_cluster/v1": false,
		"event_summary/v1": false, "entity_claim_extraction/v2": false,
	}
	for _, sample := range dataset.Samples {
		key := sample.TaskType + "/" + sample.SchemaVersion
		if _, ok := wanted[key]; !ok {
			t.Fatalf("unapproved Agent Golden contract %q", key)
		}
		wanted[key] = true
	}
	for contract, covered := range wanted {
		if !covered {
			t.Fatalf("Agent Golden fixture is missing %s", contract)
		}
	}
}
