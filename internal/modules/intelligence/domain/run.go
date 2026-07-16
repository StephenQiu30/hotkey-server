package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type RunStatus string

const (
	RunStatusQueued     RunStatus = "queued"
	RunStatusRunning    RunStatus = "running"
	RunStatusValidating RunStatus = "validating"
	RunStatusRetryWait  RunStatus = "retry_wait"
	RunStatusSucceeded  RunStatus = "succeeded"
	RunStatusFailed     RunStatus = "failed"
	RunStatusCancelled  RunStatus = "cancelled"
)

func (status RunStatus) Valid() bool {
	switch status {
	case RunStatusQueued, RunStatusRunning, RunStatusValidating, RunStatusRetryWait, RunStatusSucceeded, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

func CanTransition(from, to RunStatus) bool {
	switch from {
	case RunStatusQueued:
		return to == RunStatusRunning || to == RunStatusFailed || to == RunStatusCancelled
	case RunStatusRunning:
		return to == RunStatusValidating || to == RunStatusRetryWait || to == RunStatusFailed || to == RunStatusCancelled
	case RunStatusValidating:
		return to == RunStatusSucceeded || to == RunStatusRetryWait || to == RunStatusFailed || to == RunStatusCancelled
	case RunStatusRetryWait:
		return to == RunStatusRunning || to == RunStatusFailed || to == RunStatusCancelled
	default:
		return false
	}
}

type ReuseKeyInput struct {
	TaskType                                                       TaskType
	TargetType                                                     string
	TargetID, ModelProfileID, ModelProfileVersion                  int64
	ModelVersion, PromptVersion, InputSchemaVersion, SchemaVersion string
	ParametersVersion, InputHash, EvidenceSetHash                  string
}

func NewReuseKey(input ReuseKeyInput) (string, error) {
	if !input.TaskType.Valid() || strings.TrimSpace(input.TargetType) == "" || input.TargetID <= 0 || input.ModelProfileID <= 0 || input.ModelProfileVersion <= 0 ||
		strings.TrimSpace(input.ModelVersion) == "" || strings.TrimSpace(input.PromptVersion) == "" || strings.TrimSpace(input.InputSchemaVersion) == "" ||
		strings.TrimSpace(input.SchemaVersion) == "" || strings.TrimSpace(input.ParametersVersion) == "" || !validSHA256(input.InputHash) || !validSHA256(input.EvidenceSetHash) {
		return "", NewError(CodeAIModelProfileInvalid)
	}
	// A fixed ordered array (not a map or delimiter-joined text) makes
	// serialization deterministic and prevents ambiguous field boundaries.
	payload := []any{
		string(input.TaskType), input.TargetType, input.TargetID, input.ModelProfileID, input.ModelProfileVersion,
		input.ModelVersion, input.PromptVersion, input.InputSchemaVersion, input.SchemaVersion, input.ParametersVersion,
		input.InputHash, input.EvidenceSetHash,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode reuse key: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}
