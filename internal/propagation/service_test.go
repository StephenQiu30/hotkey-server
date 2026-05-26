package propagation

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPropagationPathOrdersFactAndSignalSteps(t *testing.T) {
	service := NewService()
	later := time.Date(2026, 5, 26, 10, 40, 0, 0, time.UTC)
	earlier := later.Add(-5 * time.Minute)

	if err := service.AddStep(StepInput{EventID: "event_1", SourceID: "x-signal", Layer: LayerSignal, URL: "https://x.example/post", ObservedAt: later, Note: "developer signal"}); err != nil {
		t.Fatalf("add signal step: %v", err)
	}
	if err := service.AddStep(StepInput{EventID: "event_1", SourceID: "arxiv", Layer: LayerFact, URL: "https://arxiv.org/abs/1", ObservedAt: earlier, Note: "paper source"}); err != nil {
		t.Fatalf("add fact step: %v", err)
	}

	path := service.GetPath("event_1")
	if len(path.Steps) != 2 {
		t.Fatalf("steps = %#v, want 2", path.Steps)
	}
	if path.Steps[0].SourceID != "arxiv" || path.Steps[1].SourceID != "x-signal" {
		t.Fatalf("steps not ordered by observed time: %#v", path.Steps)
	}
	if path.Steps[0].Layer != LayerFact || path.Steps[1].Layer != LayerSignal {
		t.Fatalf("layers = %#v", path.Steps)
	}
}

func TestFactSourceConflictGetsArbitrationStatusAndExplanation(t *testing.T) {
	service := NewService()
	for _, claim := range []ClaimInput{
		{EventID: "event_1", ClaimKey: "benchmark_score", Value: "92.1", SourceID: "lab-a", Layer: LayerFact, TrustScore: 92},
		{EventID: "event_1", ClaimKey: "benchmark_score", Value: "84.7", SourceID: "lab-b", Layer: LayerFact, TrustScore: 88},
		{EventID: "event_1", ClaimKey: "benchmark_score", Value: "92.1", SourceID: "social-signal", Layer: LayerSignal, TrustScore: 40},
	} {
		if err := service.AddClaim(claim); err != nil {
			t.Fatalf("add claim %#v: %v", claim, err)
		}
	}

	result := service.Arbitrate("event_1")
	if result.Status != StatusConflict {
		t.Fatalf("status = %s, want %s", result.Status, StatusConflict)
	}
	if result.WinningValue != "92.1" {
		t.Fatalf("winning value = %s, want 92.1", result.WinningValue)
	}
	if !strings.Contains(result.Explanation, "conflicting fact sources") {
		t.Fatalf("explanation = %q", result.Explanation)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("conflicts = %#v, want one disputed claim key", result.Conflicts)
	}
}

func TestPropagationRejectsInvalidClaim(t *testing.T) {
	service := NewService()
	err := service.AddClaim(ClaimInput{EventID: "event_1", ClaimKey: " ", Value: "value", SourceID: "source", Layer: LayerFact, TrustScore: 80})
	if !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidClaim)
	}
}
