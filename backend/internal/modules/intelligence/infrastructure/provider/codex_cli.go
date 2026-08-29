package provider

import (
	"bytes"
	"context"
	"encoding/json"
	stdErrors "errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

var codexCLIArgumentVector = []string{
	"exec",
	"--json",
	"--ephemeral",
	"--ignore-user-config",
	"--ignore-rules",
	"--skip-git-repo-check",
	"--strict-config",
	"--disable", "shell_tool",
	"--disable", "browser_use",
	"--disable", "browser_use_external",
	"--disable", "browser_use_full_cdp_access",
	"--disable", "in_app_browser",
	"--disable", "computer_use",
	"--disable", "apps",
	"--disable", "image_generation",
	"--disable", "multi_agent",
	"--disable", "multi_agent_v2",
	"--disable", "tool_suggest",
	"--color", "never",
	"--sandbox", "read-only",
}

const (
	codexCLISafePath              = "/usr/local/bin:/usr/bin:/bin"
	codexCLIOutputSchemaName      = "output.schema.json"
	defaultCodexCLITimeout        = 30 * time.Second
	defaultCodexCLIOutputBytes    = 1 << 20
	defaultCodexCLIConcurrentRuns = 1
	maxCodexCLIInputFiles         = 32
	maxCodexCLIInputBytes         = 16 << 20
	maxCodexCLIAuthBytes          = 64 << 10
	maxCodexCLIOutputBytes        = 64 << 20
	maxCodexCLIConcurrentRuns     = 64
	maxCodexCLITimeout            = time.Hour
)

type CodexCLIInput struct {
	Name    string
	Content []byte
}

// CodexCLIProcessRequest is the untrusted prompt presented to the isolated
// process boundary. Business persistence and structured-output validation are
// intentionally owned by later Application and Domain stages.
type CodexCLIProcessRequest struct {
	Prompt       []byte
	Model        string
	OutputSchema json.RawMessage
	Inputs       []CodexCLIInput
}

// CodexCLIProcessResult contains only stdout for the downstream JSONL
// validation boundary. Raw stderr never crosses the infrastructure adapter.
type CodexCLIProcessResult struct {
	Stdout               []byte
	DurationMicros       int64
	ProcessCPUTimeMicros int64
	PeakRSSBytes         int64
}

// CodexCLIAdapter starts the Codex executable directly with a fixed argument
// vector. It never invokes a command shell and passes untrusted text on stdin.
type CodexCLIAdapter struct {
	executable     string
	workspaceRoot  string
	authFile       string
	timeout        time.Duration
	maxOutputBytes int64
	concurrency    chan struct{}
}

type CodexCLIAdapterOptions struct {
	Executable     string
	WorkspaceRoot  string
	AuthFile       string
	Timeout        time.Duration
	MaxOutputBytes int64
	MaxConcurrent  int
}

func NewCodexCLIAdapter(executable string) (*CodexCLIAdapter, error) {
	return NewCodexCLIAdapterWithOptions(CodexCLIAdapterOptions{
		Executable:     executable,
		WorkspaceRoot:  os.TempDir(),
		Timeout:        defaultCodexCLITimeout,
		MaxOutputBytes: defaultCodexCLIOutputBytes,
		MaxConcurrent:  defaultCodexCLIConcurrentRuns,
	})
}

func NewCodexCLIAdapterWithOptions(options CodexCLIAdapterOptions) (*CodexCLIAdapter, error) {
	executable := strings.TrimSpace(options.Executable)
	workspaceRoot := strings.TrimSpace(options.WorkspaceRoot)
	authFile := strings.TrimSpace(options.AuthFile)
	if executable == "" || !filepath.IsAbs(executable) || workspaceRoot == "" || !filepath.IsAbs(workspaceRoot) ||
		(authFile != "" && !filepath.IsAbs(authFile)) ||
		options.Timeout <= 0 || options.Timeout > maxCodexCLITimeout ||
		options.MaxOutputBytes <= 0 || options.MaxOutputBytes > maxCodexCLIOutputBytes ||
		options.MaxConcurrent <= 0 || options.MaxConcurrent > maxCodexCLIConcurrentRuns {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	workspaceInfo, err := os.Stat(workspaceRoot)
	if err != nil || !workspaceInfo.IsDir() {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	return &CodexCLIAdapter{
		executable:     executable,
		workspaceRoot:  workspaceRoot,
		authFile:       authFile,
		timeout:        options.Timeout,
		maxOutputBytes: options.MaxOutputBytes,
		concurrency:    make(chan struct{}, options.MaxConcurrent),
	}, nil
}

func (adapter *CodexCLIAdapter) Run(ctx context.Context, request CodexCLIProcessRequest) (result CodexCLIProcessResult, runErr error) {
	if adapter == nil || strings.TrimSpace(adapter.executable) == "" || ctx == nil || len(bytes.TrimSpace(request.Prompt)) == 0 {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if err := validateCodexCLIInputs(request.Inputs); err != nil {
		return CodexCLIProcessResult{}, err
	}
	if (strings.TrimSpace(request.Model) == "") != (len(request.OutputSchema) == 0) ||
		(len(request.OutputSchema) > 0 && !json.Valid(request.OutputSchema)) {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	runContext, cancel := context.WithTimeout(ctx, adapter.timeout)
	defer cancel()
	select {
	case adapter.concurrency <- struct{}{}:
		defer func() { <-adapter.concurrency }()
	case <-runContext.Done():
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
	}

	workspace, err := os.MkdirTemp(adapter.workspaceRoot, "hotkey-codex-task-")
	if err != nil {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	defer func() {
		_ = os.Chmod(filepath.Join(workspace, "inputs"), 0o700)
		_ = os.Chmod(workspace, 0o700)
		if cleanupErr := os.RemoveAll(workspace); cleanupErr != nil && runErr == nil {
			result = CodexCLIProcessResult{}
			runErr = intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
		}
	}()
	if err := os.Chmod(workspace, 0o700); err != nil {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	if err := materializeCodexCLIRuntimeDirectories(workspace); err != nil {
		return CodexCLIProcessResult{}, err
	}
	if err := materializeCodexCLIAuth(workspace, adapter.authFile); err != nil {
		return CodexCLIProcessResult{}, err
	}
	if err := materializeCodexCLIInputs(workspace, request.Inputs, request.OutputSchema); err != nil {
		return CodexCLIProcessResult{}, err
	}

	command := exec.Command(adapter.executable, codexCLIArguments(request)...)
	command.Dir = workspace
	command.Env = codexCLIEnvironment(workspace)
	command.Stdin = bytes.NewReader(request.Prompt)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout := newCodexCLILimitedBuffer(adapter.maxOutputBytes)
	command.Stdout = stdout
	startedAt := time.Now()
	if err := command.Start(); err != nil {
		var executableError *exec.Error
		if stdErrors.As(err, &executableError) || stdErrors.Is(err, os.ErrNotExist) {
			return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
		}
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	select {
	case err = <-waited:
	case <-runContext.Done():
		killCodexCLIProcessGroup(command)
		<-waited
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
	case <-stdout.overflow:
		killCodexCLIProcessGroup(command)
		<-waited
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	if stdout.exceeded() {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	if err != nil {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	return CodexCLIProcessResult{
		Stdout: stdout.bytes(), DurationMicros: time.Since(startedAt).Microseconds(),
		ProcessCPUTimeMicros: command.ProcessState.UserTime().Microseconds() + command.ProcessState.SystemTime().Microseconds(),
		PeakRSSBytes:         codexCLIProcessPeakRSSBytes(command.ProcessState),
	}, nil
}

func codexCLIProcessPeakRSSBytes(state *os.ProcessState) int64 {
	if state == nil {
		return 0
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || usage.Maxrss <= 0 {
		return 0
	}
	peak := int64(usage.Maxrss)
	if runtime.GOOS != "darwin" {
		peak *= 1024
	}
	return peak
}

func codexCLIArguments(request CodexCLIProcessRequest) []string {
	arguments := append([]string(nil), codexCLIArgumentVector...)
	if len(request.OutputSchema) > 0 {
		arguments = append(arguments,
			"--model", strings.TrimSpace(request.Model),
			"--output-schema", filepath.Join("inputs", codexCLIOutputSchemaName),
		)
	}
	return append(arguments, "-")
}

func materializeCodexCLIRuntimeDirectories(workspace string) error {
	for _, name := range []string{"tmp", "codex-home"} {
		if err := os.Mkdir(filepath.Join(workspace, name), 0o700); err != nil {
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
		}
	}
	return nil
}

// materializeCodexCLIAuth copies one explicitly approved private auth file into
// the per-task CODEX_HOME. It is read for every task so an atomic file switch
// becomes visible after a process rollout without inheriting the worker's own
// CODEX_HOME or provider environment.
func materializeCodexCLIAuth(workspace, authFile string) error {
	authFile = strings.TrimSpace(authFile)
	if authFile == "" {
		return nil
	}
	linkInfo, err := os.Lstat(authFile)
	if err != nil || !linkInfo.Mode().IsRegular() || linkInfo.Mode().Perm()&0o077 != 0 || linkInfo.Size() <= 0 || linkInfo.Size() > maxCodexCLIAuthBytes {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	source, err := os.Open(authFile)
	if err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(linkInfo, openedInfo) {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	contents, err := io.ReadAll(io.LimitReader(source, maxCodexCLIAuthBytes+1))
	if err != nil || len(contents) == 0 || len(contents) > maxCodexCLIAuthBytes {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(contents, &object); err != nil || len(object) == 0 {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	destinationPath := filepath.Join(workspace, "codex-home", "auth.json")
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	removeDestination := true
	defer func() {
		_ = destination.Close()
		if removeDestination {
			_ = os.Remove(destinationPath)
		}
	}()
	if _, err := destination.Write(contents); err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	if err := destination.Close(); err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	removeDestination = false
	return nil
}

func codexCLIEnvironment(workspace string) []string {
	return []string{
		"HOME=" + workspace,
		"TMPDIR=" + filepath.Join(workspace, "tmp"),
		"CODEX_HOME=" + filepath.Join(workspace, "codex-home"),
		"PATH=" + codexCLISafePath,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"NO_COLOR=1",
	}
}

func validateCodexCLIInputs(inputs []CodexCLIInput) error {
	if len(inputs) > maxCodexCLIInputFiles {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	seen := make(map[string]struct{}, len(inputs))
	totalBytes := 0
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" || name != input.Name || filepath.Base(name) != name || name == "." || strings.ContainsAny(name, `/\\`) {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if _, exists := seen[name]; exists {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if name == codexCLIOutputSchemaName {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		seen[name] = struct{}{}
		if len(input.Content) > maxCodexCLIInputBytes-totalBytes {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		totalBytes += len(input.Content)
	}
	return nil
}

func materializeCodexCLIInputs(workspace string, inputs []CodexCLIInput, outputSchema json.RawMessage) error {
	if len(inputs) == 0 && len(outputSchema) == 0 {
		return nil
	}
	inputDirectory := filepath.Join(workspace, "inputs")
	if err := os.Mkdir(inputDirectory, 0o700); err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	for _, input := range inputs {
		if err := os.WriteFile(filepath.Join(inputDirectory, input.Name), input.Content, 0o400); err != nil {
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
		}
	}
	if len(outputSchema) > 0 {
		if err := os.WriteFile(filepath.Join(inputDirectory, codexCLIOutputSchemaName), outputSchema, 0o400); err != nil {
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
		}
	}
	if err := os.Chmod(inputDirectory, 0o500); err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	return nil
}

func killCodexCLIProcessGroup(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	_ = command.Process.Kill()
}

type codexCLILimitedBuffer struct {
	mutex    sync.Mutex
	buffer   bytes.Buffer
	limit    int64
	overflow chan struct{}
	once     sync.Once
	over     bool
}

func newCodexCLILimitedBuffer(limit int64) *codexCLILimitedBuffer {
	return &codexCLILimitedBuffer{limit: limit, overflow: make(chan struct{})}
}

func (buffer *codexCLILimitedBuffer) Write(payload []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	remaining := buffer.limit - int64(buffer.buffer.Len())
	if int64(len(payload)) > remaining {
		if remaining > 0 {
			_, _ = buffer.buffer.Write(payload[:remaining])
		}
		buffer.over = true
		buffer.once.Do(func() { close(buffer.overflow) })
		return len(payload), nil
	}
	return buffer.buffer.Write(payload)
}

func (buffer *codexCLILimitedBuffer) exceeded() bool {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.over
}

func (buffer *codexCLILimitedBuffer) bytes() []byte {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return append([]byte(nil), buffer.buffer.Bytes()...)
}
