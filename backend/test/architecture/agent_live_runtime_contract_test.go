package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonAgentTrustedModelRuntimeRemainsShadowOnly(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	runtime := readRepositoryFile(t, repository, "agent/src/hotkey_agent/model_runtime.py")
	for _, fragment := range []string{
		"class CodexAppServerAnalyzer",
		"class CodexAppServerClient",
		`"thread/start"`,
		`"turn/start"`,
		`"outputSchema"`,
		`"networkAccess": False`,
		"_DISABLED_FEATURES",
		"ModelOutputInvalidError",
		"input_tokens",
		"output_tokens",
	} {
		if !strings.Contains(runtime, fragment) {
			t.Errorf("trusted Python model runtime lost %q", fragment)
		}
	}

	manifest := readRepositoryFile(t, repository, "agent/pyproject.toml")
	if !strings.Contains(manifest, `"websockets==17.1"`) {
		t.Error("Python Agent runtime must lock its Codex App Server transport dependency")
	}
	qualityCommand := readRepositoryFile(t, repository, "backend/internal/bootstrap/agent_quality_command.go")
	for _, fragment := range []string{
		"AgentInputUSDPerMillionTokens",
		"AgentOutputUSDPerMillionTokens",
		`"agent-input-usd-per-million"`,
		`"agent-output-usd-per-million"`,
		"UsageAvailable: options.AgentModelVersion != intelligenceagent.DeterministicRuntimeVersion",
	} {
		if !strings.Contains(qualityCommand, fragment) {
			t.Errorf("trusted Agent quality cost plumbing lost %q", fragment)
		}
	}
	for _, composePath := range []string{"docker-compose.yml", "docker-compose-prod.yml"} {
		compose := readRepositoryFile(t, repository, composePath)
		block := dockerComposeServiceBlock(t, compose, "hotkey-agent")
		for _, fragment := range []string{`HOTKEY_AGENT_CODEX_APP_SERVER_URL: "${HOTKEY_AGENT_CODEX_APP_SERVER_URL:-ws://host.docker.internal:4500}"`} {
			if !strings.Contains(block, fragment) {
				t.Errorf("%s trusted model runtime configuration lost %q", composePath, fragment)
			}
		}
		for _, forbidden := range []string{"HOTKEY_DATABASE_URL", "HOTKEY_REDIS_URL", "HOTKEY_MINIO", "HOTKEY_VAULT", "HOTKEY_AGENT_MODEL_", "HOTKEY_AGENT_RUNTIME"} {
			if strings.Contains(block, forbidden) {
				t.Errorf("%s gives the Python Agent forbidden business credential %s", composePath, forbidden)
			}
		}
	}

	bootstrap := readRepositoryFile(t, repository, "backend/internal/bootstrap/app.go")
	providerDomain := readRepositoryFile(t, repository, "backend/internal/modules/intelligence/domain/provider.go")
	if strings.Contains(providerDomain, "ProviderAgent") || strings.Contains(providerDomain, "ProviderCodexAppServer") {
		t.Fatal("Python Agent model runtime entered the production ProviderName before G5 approval")
	}
	providerRegistryStart := strings.Index(bootstrap, "func newAIProviderRegistry")
	shadowRunnerStart := strings.Index(bootstrap, "func newAgentShadowRunner")
	if providerRegistryStart < 0 || shadowRunnerStart <= providerRegistryStart ||
		strings.Contains(bootstrap[providerRegistryStart:shadowRunnerStart], "intelligenceagent") {
		t.Fatal("Python Agent model runtime escaped the Shadow-only bootstrap boundary")
	}

	design := readRepositoryFile(t, repository, "docs/design/001-HotKey产品需求分析与总体架构设计.md")
	for _, fragment := range []string{
		"Python Agent 的唯一真实模型运行时是本机 `codex app-server`",
		"`deterministic.v1` 仅保留为无模型降级与契约测试实现",
		"不调用 Ollama 或 OpenAI-compatible 模型服务",
		"尚未进入 Live 决策或业务事实写入",
	} {
		if !strings.Contains(design, fragment) {
			t.Errorf("Design 001 does not preserve the trusted model boundary: missing %q", fragment)
		}
	}
	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")
	if row := markdownChecklistRow(t, plan, "CHK-003-G5-001"); !strings.HasPrefix(row, "- [ ]") {
		t.Errorf("trusted model runtime plumbing must not auto-approve G5: %s", row)
	}
}
