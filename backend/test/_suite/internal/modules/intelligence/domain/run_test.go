package domain

import (
	"strings"
	"testing"
)

func TestReuseKeyIsStableAndInvalidatedByEverySemanticVersion(t *testing.T) {
	input := ReuseKeyInput{
		TaskType:            TaskTypeEmbedding,
		TargetType:          "content",
		TargetID:            1,
		ModelProfileID:      7,
		ModelProfileVersion: 3,
		ModelVersion:        "2026-07",
		PromptVersion:       "prompt-v1",
		InputSchemaVersion:  "v1",
		SchemaVersion:       "v1",
		ParametersVersion:   "parameters-v1",
		InputHash:           strings.Repeat("a", 64),
		EvidenceSetHash:     strings.Repeat("b", 64),
	}
	first, err := NewReuseKey(input)
	if err != nil {
		t.Fatalf("NewReuseKey() error = %v", err)
	}
	second, err := NewReuseKey(input)
	if err != nil {
		t.Fatalf("NewReuseKey() second error = %v", err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("reuse key = %q/%q, want stable SHA-256", first, second)
	}

	for _, mutate := range []func(*ReuseKeyInput){
		func(input *ReuseKeyInput) { input.ModelVersion = "2026-08" },
		func(input *ReuseKeyInput) { input.PromptVersion = "prompt-v2" },
		func(input *ReuseKeyInput) { input.InputSchemaVersion = "v2" },
		func(input *ReuseKeyInput) { input.SchemaVersion = "v2" },
		func(input *ReuseKeyInput) { input.ParametersVersion = "parameters-v2" },
		func(input *ReuseKeyInput) { input.InputHash = strings.Repeat("c", 64) },
		func(input *ReuseKeyInput) { input.EvidenceSetHash = strings.Repeat("d", 64) },
	} {
		candidate := input
		mutate(&candidate)
		key, err := NewReuseKey(candidate)
		if err != nil {
			t.Fatalf("NewReuseKey() changed input error = %v", err)
		}
		if key == first {
			t.Fatal("NewReuseKey() failed to invalidate changed semantic input")
		}
	}
}

func TestRunStatusAllowsOnlyDocumentedTransitions(t *testing.T) {
	if !CanTransition(RunStatusQueued, RunStatusRunning) || !CanTransition(RunStatusRunning, RunStatusRetryWait) || !CanTransition(RunStatusRetryWait, RunStatusRunning) || !CanTransition(RunStatusValidating, RunStatusSucceeded) {
		t.Fatal("CanTransition() rejected a documented run state transition")
	}
	if CanTransition(RunStatusQueued, RunStatusSucceeded) || CanTransition(RunStatusSucceeded, RunStatusRunning) {
		t.Fatal("CanTransition() accepted an undocumented run state transition")
	}
}
