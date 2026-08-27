package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

func TestCodexCLIProviderAcceptsOnlyOneCompletedStructuredAgentResult(t *testing.T) {
	fixtureDirectory := t.TempDir()
	workspaceRoot := filepath.Join(fixtureDirectory, "workspaces")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeExecutable := writeCodexCLIFixture(t, fixtureDirectory, `#!/bin/sh
set -eu
fixture_directory=$(dirname "$0")
printf '%s\n' "$@" > "$fixture_directory/argv"
cat inputs/output.schema.json > "$fixture_directory/output-schema"
cat > "$fixture_directory/stdin"
printf '%s\n' \
  '{"type":"thread.started","thread_id":"thread-1"}' \
  '{"type":"turn.started"}' \
  '{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"{\"terms\":[]}"}}' \
  '{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":3,"cache_write_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":1}}'
`)
	provider := newCodexCLIProviderFixture(t, fakeExecutable, workspaceRoot)

	request := codexCLIStructuredRequest()
	response, err := provider.GenerateStructured(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.ModelVersion != "codex-cli-0.149.1-model-v1" || string(response.JSON) != `{"terms":[]}` ||
		response.Usage.InputTokens != 10 || response.Usage.OutputTokens != 2 {
		t.Fatalf("response = %#v", response)
	}
	arguments, err := os.ReadFile(filepath.Join(fixtureDirectory, "argv"))
	if err != nil || !strings.Contains(string(arguments), "--model\ngpt-test\n--output-schema\ninputs/output.schema.json\n") {
		t.Fatalf("structured argv=%q err=%v", arguments, err)
	}
	outputSchema, err := os.ReadFile(filepath.Join(fixtureDirectory, "output-schema"))
	if err != nil || string(outputSchema) != string(request.Schema) {
		t.Fatalf("output schema=%q err=%v", outputSchema, err)
	}
	standardInput, err := os.ReadFile(filepath.Join(fixtureDirectory, "stdin"))
	if err != nil || !strings.Contains(string(standardInput), `"task_type":"term_expansion"`) ||
		!strings.Contains(string(standardInput), `"schema_name":"term-expansion-output-v1"`) ||
		!strings.Contains(string(standardInput), `"schema_version":"v1"`) {
		t.Fatalf("versioned stdin contract=%q err=%v", standardInput, err)
	}
}

func TestCodexCLIProviderRejectsInvalidOrFailedJSONLWithoutRawErrorLeakage(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		stream     string
		exit       string
		wantedCode int
	}{
		{name: "exit zero without result", stream: `{"type":"thread.started","thread_id":"thread-1"}
{"type":"turn.started"}
{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}
`, wantedCode: intelligencedomain.CodeAIOutputInvalid},
		{name: "broken jsonl", stream: `{"type":"thread.started"}
not-json
`, wantedCode: intelligencedomain.CodeAIOutputInvalid},
		{name: "unknown event", stream: `{"type":"thread.started","thread_id":"thread-1"}
{"type":"turn.started"}
{"type":"future.event","secret":"must-not-leak"}
`, wantedCode: intelligencedomain.CodeAIOutputInvalid},
		{name: "fatal stream error", stream: `{"type":"error","message":"raw upstream authentication detail"}
`, wantedCode: intelligencedomain.CodeAIProviderTransient},
		{name: "turn failed", stream: `{"type":"thread.started","thread_id":"thread-1"}
{"type":"turn.started"}
{"type":"turn.failed","error":{"message":"raw failure detail"}}
`, wantedCode: intelligencedomain.CodeAIProviderTransient},
		{name: "tool execution", stream: `{"type":"thread.started","thread_id":"thread-1"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item-1","type":"command_execution","command":"cat /secret","status":"completed","exit_code":0}}
{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}
`, wantedCode: intelligencedomain.CodeAIOutputInvalid},
		{name: "invalid final json", stream: `{"type":"thread.started","thread_id":"thread-1"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"not-json"}}
{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}
`, wantedCode: intelligencedomain.CodeAIOutputInvalid},
		{name: "nonzero exit", stream: ``, exit: "exit 17", wantedCode: intelligencedomain.CodeAIProviderTransient},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixtureDirectory := t.TempDir()
			workspaceRoot := filepath.Join(fixtureDirectory, "workspaces")
			if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\nset -eu\ncat >/dev/null\ncat <<'HOTKEY_CODEX_STREAM'\n" + testCase.stream + "HOTKEY_CODEX_STREAM\n" + testCase.exit + "\n"
			fakeExecutable := writeCodexCLIFixture(t, fixtureDirectory, script)
			provider := newCodexCLIProviderFixture(t, fakeExecutable, workspaceRoot)

			_, runErr := provider.GenerateStructured(context.Background(), codexCLIStructuredRequest())
			code, known := intelligencedomain.CodeOf(runErr)
			if !known || code != testCase.wantedCode {
				t.Fatalf("error=%v code=%d known=%v", runErr, code, known)
			}
			if runErr != nil && (runErr.Error() == "raw upstream authentication detail" || runErr.Error() == "raw failure detail") {
				t.Fatalf("raw CLI error crossed the adapter: %v", runErr)
			}
		})
	}
}

func newCodexCLIProviderFixture(t *testing.T, executable, workspaceRoot string) *CodexCLIProvider {
	t.Helper()
	adapter, err := NewCodexCLIAdapterWithOptions(CodexCLIAdapterOptions{
		Executable: executable, WorkspaceRoot: workspaceRoot, Timeout: 3 * time.Second,
		MaxOutputBytes: 64 << 10, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewCodexCLIProvider(adapter)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func codexCLIStructuredRequest() intelligencedomain.StructuredRequest {
	return intelligencedomain.StructuredRequest{
		ModelName: "gpt-test", ModelVersion: "codex-cli-0.149.1-model-v1",
		TaskType:   intelligencedomain.TaskTypeTermExpansion,
		SchemaName: "term-expansion-output-v1", SchemaVersion: "v1",
		Instruction: "Return one structured term expansion result and do not execute tools.",
		Schema:      json.RawMessage(`{"type":"object","additionalProperties":false,"required":["terms"],"properties":{"terms":{"type":"array"}}}`),
		Input:       json.RawMessage(`{"objective":"hotkey"}`),
	}
}
