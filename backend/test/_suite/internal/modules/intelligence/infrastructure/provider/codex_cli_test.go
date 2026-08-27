package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodexCLIAdapterUsesFixedArgumentVectorAndPromptStdinWithoutShellEvaluation(t *testing.T) {
	temporaryDirectory := t.TempDir()
	fakeExecutable := filepath.Join(temporaryDirectory, "fake-codex")
	marker := filepath.Join(temporaryDirectory, "prompt-was-executed")
	fixture := `#!/bin/sh
set -eu
fixture_directory=$(dirname "$0")
printf '%s\n' "$@" > "$fixture_directory/argv"
cat > "$fixture_directory/stdin"
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"{}"}}'
`
	if err := os.WriteFile(fakeExecutable, []byte(fixture), 0o700); err != nil {
		t.Fatal(err)
	}
	prompt := []byte("Untrusted source text: $(touch " + marker + "); touch " + marker)
	adapter, err := NewCodexCLIAdapter(fakeExecutable)
	if err != nil {
		t.Fatal(err)
	}

	result, err := adapter.Run(context.Background(), CodexCLIProcessRequest{Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stdout) == 0 {
		t.Fatal("codex stdout was not returned to the validation boundary")
	}
	arguments, err := os.ReadFile(filepath.Join(temporaryDirectory, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	gotArguments := strings.Split(strings.TrimSpace(string(arguments)), "\n")
	wantArguments := []string{"exec", "--json", "--ephemeral", "--sandbox", "read-only", "-"}
	if !reflect.DeepEqual(gotArguments, wantArguments) {
		t.Fatalf("argv = %#v, want %#v", gotArguments, wantArguments)
	}
	standardInput, err := os.ReadFile(filepath.Join(temporaryDirectory, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(standardInput, prompt) {
		t.Fatalf("stdin = %q, want exact untrusted prompt", standardInput)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("untrusted prompt was evaluated by a shell: %v", err)
	}
}

func TestCodexCLIAdapterRejectsInvalidExecutableAndEmptyPromptBeforeStartingProcess(t *testing.T) {
	if _, err := NewCodexCLIAdapter(" "); err == nil {
		t.Fatal("empty executable was accepted")
	}
	adapter, err := NewCodexCLIAdapter(filepath.Join(t.TempDir(), "missing-codex"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Run(context.Background(), CodexCLIProcessRequest{}); err == nil {
		t.Fatal("empty prompt was accepted")
	}
}
