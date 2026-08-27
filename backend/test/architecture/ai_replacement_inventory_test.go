package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAIReplacementInventoryCoversCurrentCallersAndDataDisposition(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")

	callers := []string{
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
		"Codex CLI Adapter 已实现但尚未进入 ProviderName、Model Profile Schema 或生产 Registry",
		"无第一方页面直接调用模型 Profile 管理客户端",
		"报告模块不直接调用 Provider、Embedding 或 Codex",
		"旧记录继续按原 Model Profile、Run 与向量版本可读",
		"本阶段禁止物理删除",
	} {
		if !strings.Contains(plan, decision) {
			t.Errorf("S05 inventory is missing current fact or disposition %q", decision)
		}
	}
}

func TestAIReplacementInventoryRecordsCodexProductionWiringGap(t *testing.T) {
	root := repositoryRoot(t)
	providerDomain := readRepositoryFile(t, root, "internal/modules/intelligence/domain/provider.go")
	bootstrap := readRepositoryFile(t, root, "internal/bootstrap/app.go")
	codexProvider := readRepositoryFile(t, root, "internal/modules/intelligence/infrastructure/provider/codex_cli_provider.go")

	if strings.Contains(providerDomain, "ProviderCodex") {
		t.Fatal("Codex unexpectedly entered the production ProviderName contract before S05 switch tests")
	}
	if strings.Contains(bootstrap, "NewCodexCLIProvider") {
		t.Fatal("Codex unexpectedly entered the production registry before S05 switch tests")
	}
	if !strings.Contains(codexProvider, "type CodexCLIProvider struct") {
		t.Fatal("controlled Codex provider adapter is missing from the replacement inventory")
	}
}
