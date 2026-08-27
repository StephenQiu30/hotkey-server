package provider

import (
	"bytes"
	"context"
	stdErrors "errors"
	"os/exec"
	"strings"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

var codexCLIArgumentVector = []string{
	"exec",
	"--json",
	"--ephemeral",
	"--sandbox", "read-only",
	"-",
}

// CodexCLIProcessRequest is the untrusted prompt presented to the isolated
// process boundary. Business persistence and structured-output validation are
// intentionally owned by later Application and Domain stages.
type CodexCLIProcessRequest struct {
	Prompt []byte
}

// CodexCLIProcessResult contains only stdout for the downstream JSONL
// validation boundary. Raw stderr never crosses the infrastructure adapter.
type CodexCLIProcessResult struct {
	Stdout []byte
}

// CodexCLIAdapter starts the Codex executable directly with a fixed argument
// vector. It never invokes a command shell and passes untrusted text on stdin.
type CodexCLIAdapter struct {
	executable string
}

func NewCodexCLIAdapter(executable string) (*CodexCLIAdapter, error) {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	return &CodexCLIAdapter{executable: executable}, nil
}

func (adapter *CodexCLIAdapter) Run(ctx context.Context, request CodexCLIProcessRequest) (CodexCLIProcessResult, error) {
	if adapter == nil || strings.TrimSpace(adapter.executable) == "" || ctx == nil || len(bytes.TrimSpace(request.Prompt)) == 0 {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	command := exec.CommandContext(ctx, adapter.executable, codexCLIArgumentVector...)
	command.Stdin = bytes.NewReader(request.Prompt)
	stdout, err := command.Output()
	if err == nil {
		return CodexCLIProcessResult{Stdout: append([]byte(nil), stdout...)}, nil
	}
	if ctx.Err() != nil {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
	}
	var executableError *exec.Error
	if stdErrors.As(err, &executableError) {
		return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	return CodexCLIProcessResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
}
