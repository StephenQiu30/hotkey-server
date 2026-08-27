package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
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
	wantArguments := []string{
		"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check",
		"--color", "never", "--sandbox", "read-only", "-",
	}
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
	if _, err := NewCodexCLIAdapter("codex"); err == nil {
		t.Fatal("PATH-resolved executable was accepted")
	}
	adapter, err := NewCodexCLIAdapter(filepath.Join(t.TempDir(), "missing-codex"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Run(context.Background(), CodexCLIProcessRequest{}); err == nil {
		t.Fatal("empty prompt was accepted")
	}
}

func TestCodexCLIAdapterMapsMissingExecutableToModelUnavailable(t *testing.T) {
	adapter, err := NewCodexCLIAdapter(filepath.Join(t.TempDir(), "missing-codex"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Run(context.Background(), CodexCLIProcessRequest{Prompt: []byte("bounded prompt")})
	if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIModelUnavailable {
		t.Fatalf("missing executable error=%v code=%d known=%v", err, code, ok)
	}
}

func TestCodexCLIAdapterCreatesReadOnlyTaskInputsCapsOutputAndCleansWorkspace(t *testing.T) {
	fixtureDirectory := t.TempDir()
	workspaceRoot := filepath.Join(fixtureDirectory, "workspaces")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeExecutable := writeCodexCLIFixture(t, fixtureDirectory, `#!/bin/sh
set -eu
fixture_directory=$(dirname "$0")
mode=$(cat)
pwd > "$fixture_directory/cwd"
if [ -w inputs/context.json ]; then printf 'writable' > "$fixture_directory/input-mode"; else printf 'readonly' > "$fixture_directory/input-mode"; fi
if [ "$mode" = "oversize" ]; then
  printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
else
  printf '{"ok":true}'
fi
`)
	adapter, err := NewCodexCLIAdapterWithOptions(CodexCLIAdapterOptions{
		Executable:     fakeExecutable,
		WorkspaceRoot:  workspaceRoot,
		Timeout:        10 * time.Second,
		MaxOutputBytes: 64,
		MaxConcurrent:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Run(context.Background(), CodexCLIProcessRequest{
		Prompt: []byte("normal"),
		Inputs: []CodexCLIInput{{Name: "context.json", Content: []byte(`{"evidence_ids":["ev-1"]}`)}},
	})
	if err != nil || string(result.Stdout) != `{"ok":true}` || result.DurationMicros <= 0 || result.PeakRSSBytes <= 0 || result.ProcessCPUTimeMicros < 0 {
		t.Fatalf("result=%q err=%v", result.Stdout, err)
	}
	workingDirectory, err := os.ReadFile(filepath.Join(fixtureDirectory, "cwd"))
	if err != nil {
		t.Fatal(err)
	}
	resolvedWorkspaceRoot, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(workingDirectory)), resolvedWorkspaceRoot+string(os.PathSeparator)) {
		t.Fatalf("working directory %q is outside isolated root", workingDirectory)
	}
	mode, err := os.ReadFile(filepath.Join(fixtureDirectory, "input-mode"))
	if err != nil || string(mode) != "readonly" {
		t.Fatalf("input mode=%q err=%v", mode, err)
	}
	assertDirectoryEmpty(t, workspaceRoot)

	_, err = adapter.Run(context.Background(), CodexCLIProcessRequest{
		Prompt: []byte("oversize"),
		Inputs: []CodexCLIInput{{Name: "context.json", Content: []byte(`{}`)}},
	})
	if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIOutputInvalid {
		t.Fatalf("oversize error=%v code=%d known=%v", err, code, ok)
	}
	assertDirectoryEmpty(t, workspaceRoot)
}

func TestCodexCLIAdapterTimeoutKillsProcessGroupAndCleansWorkspace(t *testing.T) {
	fixtureDirectory := t.TempDir()
	workspaceRoot := filepath.Join(fixtureDirectory, "workspaces")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeExecutable := writeCodexCLIFixture(t, fixtureDirectory, `#!/bin/sh
set -eu
fixture_directory=$(dirname "$0")
cat >/dev/null
sleep 60 &
printf '%s' "$!" > "$fixture_directory/child-pid"
wait
`)
	adapter, err := NewCodexCLIAdapterWithOptions(CodexCLIAdapterOptions{
		Executable:     fakeExecutable,
		WorkspaceRoot:  workspaceRoot,
		Timeout:        3 * time.Second,
		MaxOutputBytes: 1024,
		MaxConcurrent:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Run(context.Background(), CodexCLIProcessRequest{Prompt: []byte("timeout")})
	if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIProviderTimeout {
		t.Fatalf("timeout error=%v code=%d known=%v", err, code, ok)
	}
	childPIDBytes, readErr := os.ReadFile(filepath.Join(fixtureDirectory, "child-pid"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	childPID, parseErr := strconv.Atoi(string(childPIDBytes))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	deadline := time.Now().Add(2 * time.Second)
	for processExists(childPID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processExists(childPID) {
		t.Fatalf("child process %d survived process-group termination", childPID)
	}
	assertDirectoryEmpty(t, workspaceRoot)
}

func TestCodexCLIAdapterCancellationKillsProcessGroupAndCleansWorkspace(t *testing.T) {
	fixtureDirectory := t.TempDir()
	workspaceRoot := filepath.Join(fixtureDirectory, "workspaces")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeExecutable := writeCodexCLIFixture(t, fixtureDirectory, `#!/bin/sh
set -eu
fixture_directory=$(dirname "$0")
cat >/dev/null
sleep 60 &
printf '%s' "$!" > "$fixture_directory/child-pid"
wait
`)
	adapter, err := NewCodexCLIAdapterWithOptions(CodexCLIAdapterOptions{
		Executable:     fakeExecutable,
		WorkspaceRoot:  workspaceRoot,
		Timeout:        10 * time.Second,
		MaxOutputBytes: 1024,
		MaxConcurrent:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, runErr := adapter.Run(ctx, CodexCLIProcessRequest{Prompt: []byte("cancel")})
		result <- runErr
	}()
	waitForFile(t, filepath.Join(fixtureDirectory, "child-pid"), 5*time.Second)
	cancel()
	err = <-result
	if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIProviderTimeout {
		t.Fatalf("cancellation error=%v code=%d known=%v", err, code, ok)
	}
	childPIDBytes, readErr := os.ReadFile(filepath.Join(fixtureDirectory, "child-pid"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	childPID, parseErr := strconv.Atoi(string(childPIDBytes))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	deadline := time.Now().Add(2 * time.Second)
	for processExists(childPID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processExists(childPID) {
		t.Fatalf("child process %d survived cancellation", childPID)
	}
	assertDirectoryEmpty(t, workspaceRoot)
}

func TestCodexCLIAdapterEnforcesConfiguredConcurrencyBeforeStartingNextTask(t *testing.T) {
	fixtureDirectory := t.TempDir()
	workspaceRoot := filepath.Join(fixtureDirectory, "workspaces")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeExecutable := writeCodexCLIFixture(t, fixtureDirectory, `#!/bin/sh
set -eu
fixture_directory=$(dirname "$0")
mode=$(cat)
printf 'started' > "$fixture_directory/started-$mode"
if [ "$mode" = "first" ]; then
  while [ ! -f "$fixture_directory/release" ]; do sleep 0.01; done
fi
printf '{"ok":true}'
`)
	adapter, err := NewCodexCLIAdapterWithOptions(CodexCLIAdapterOptions{
		Executable:     fakeExecutable,
		WorkspaceRoot:  workspaceRoot,
		Timeout:        10 * time.Second,
		MaxOutputBytes: 1024,
		MaxConcurrent:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	errorsChannel := make(chan error, 2)
	go func() {
		_, runErr := adapter.Run(context.Background(), CodexCLIProcessRequest{Prompt: []byte("first")})
		errorsChannel <- runErr
	}()
	waitForFile(t, filepath.Join(fixtureDirectory, "started-first"), 5*time.Second)
	go func() {
		_, runErr := adapter.Run(context.Background(), CodexCLIProcessRequest{Prompt: []byte("second")})
		errorsChannel <- runErr
	}()
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(fixtureDirectory, "started-second")); !os.IsNotExist(err) {
		t.Fatalf("second task started before the configured slot was released: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDirectory, "release"), []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if runErr := <-errorsChannel; runErr != nil {
			t.Fatal(runErr)
		}
	}
	waitForFile(t, filepath.Join(fixtureDirectory, "started-second"), 5*time.Second)
	assertDirectoryEmpty(t, workspaceRoot)
}

func TestCodexCLIAdapterBuildsExplicitEnvironmentWithoutWorkerSecrets(t *testing.T) {
	fixtureDirectory := t.TempDir()
	workspaceRoot := filepath.Join(fixtureDirectory, "workspaces")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	secrets := map[string]string{
		"HOTKEY_DATABASE_URL":            "postgres://secret-database",
		"HOTKEY_TEST_DSN":                "postgres://secret-test-database",
		"X_API_TOKEN":                    "secret-source-token",
		"HOTKEY_JWT_SECRET":              "secret-jwt-signing-value",
		"MINIO_ROOT_PASSWORD":            "secret-object-store-value",
		"HOTKEY_REQUEST_COOKIE":          "secret-user-cookie",
		"OPENAI_API_KEY":                 "secret-provider-key",
		"DEEPSEEK_API_KEY":               "secret-provider-key-two",
		"AWS_ACCESS_KEY_ID":              "secret-cloud-access-key",
		"AWS_SECRET_ACCESS_KEY":          "secret-cloud-access-value",
		"GOOGLE_APPLICATION_CREDENTIALS": "/secret/cloud/credentials.json",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}
	fakeExecutable := writeCodexCLIFixture(t, fixtureDirectory, `#!/bin/sh
set -eu
fixture_directory=$(dirname "$0")
cat >/dev/null
env | sort > "$fixture_directory/environment"
printf '%s\n%s\n%s\n%s\n' "$HOME" "$TMPDIR" "$CODEX_HOME" "$PATH" > "$fixture_directory/safe-environment"
printf '{"ok":true}'
`)
	adapter, err := NewCodexCLIAdapterWithOptions(CodexCLIAdapterOptions{
		Executable:     fakeExecutable,
		WorkspaceRoot:  workspaceRoot,
		Timeout:        10 * time.Second,
		MaxOutputBytes: 1024,
		MaxConcurrent:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Run(context.Background(), CodexCLIProcessRequest{Prompt: []byte("environment")}); err != nil {
		t.Fatal(err)
	}
	environment, err := os.ReadFile(filepath.Join(fixtureDirectory, "environment"))
	if err != nil {
		t.Fatal(err)
	}
	for name, secret := range secrets {
		if strings.Contains(string(environment), name+"=") || strings.Contains(string(environment), secret) {
			t.Fatalf("child environment leaked %s", name)
		}
	}
	allowedNames := map[string]bool{
		"HOME": true, "TMPDIR": true, "CODEX_HOME": true, "PATH": true,
		"LANG": true, "LC_ALL": true, "NO_COLOR": true,
		"PWD": true, "OLDPWD": true, "SHLVL": true, "_": true,
	}
	for _, entry := range strings.Split(strings.TrimSpace(string(environment)), "\n") {
		name, _, found := strings.Cut(entry, "=")
		if !found || !allowedNames[name] {
			t.Fatalf("child environment contains non-whitelisted entry %q", entry)
		}
	}
	safeEnvironment, err := os.ReadFile(filepath.Join(fixtureDirectory, "safe-environment"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(safeEnvironment)), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], workspaceRoot+string(os.PathSeparator)) ||
		!strings.HasPrefix(lines[1], lines[0]+string(os.PathSeparator)) ||
		!strings.HasPrefix(lines[2], lines[0]+string(os.PathSeparator)) || lines[3] != codexCLISafePath {
		t.Fatalf("safe environment = %#v", lines)
	}
	assertDirectoryEmpty(t, workspaceRoot)
}

func writeCodexCLIFixture(t *testing.T, directory, contents string) string {
	t.Helper()
	path := filepath.Join(directory, "fake-codex")
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertDirectoryEmpty(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("workspace root retained %d task directories", len(entries))
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filepath.Base(path))
}

func processExists(processID int) bool {
	err := syscall.Kill(processID, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
