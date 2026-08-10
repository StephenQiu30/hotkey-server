package domain

import "testing"

func TestDecideDocumentMatchKeepsUncalibratedAndDegradedCandidatesInReview(t *testing.T) {
	uncalibrated := RelevanceDecisionProfile{
		ID: 1, Version: 1, MatchingAlgorithmVersion: "rrf-k60-v1",
		RerankerVersion: "cross-encoder-v1", CalibrationVersion: "uncalibrated-v1",
		Status: RelevanceProfileUncalibrated, RejectThreshold: 0.40, AcceptThreshold: 0.80,
	}
	decision, err := DecideDocumentMatch(uncalibrated, float64Pointer(0.99), false, false)
	if err != nil {
		t.Fatalf("DecideDocumentMatch(uncalibrated): %v", err)
	}
	if decision != MatchDecisionReview {
		t.Fatalf("uncalibrated decision = %q, want review", decision)
	}

	active := uncalibrated
	active.Status = RelevanceProfileActive
	active.EvaluationRunID = 11
	active.CalibrationVersion = "temperature-2026-08"
	active.CalibrationSlope = 1
	decision, err = DecideDocumentMatch(active, float64Pointer(0.99), true, false)
	if err != nil {
		t.Fatalf("DecideDocumentMatch(degraded): %v", err)
	}
	if decision != MatchDecisionReview {
		t.Fatalf("degraded decision = %q, want review", decision)
	}
}

func TestDecideDocumentMatchUsesOnlyActiveCalibratedThresholdsAndHardVeto(t *testing.T) {
	profile := RelevanceDecisionProfile{
		ID: 2, Version: 1, EvaluationRunID: 12, MatchingAlgorithmVersion: "rrf-k60-v1",
		RerankerVersion: "cross-encoder-v1", CalibrationVersion: "isotonic-2026-08",
		Status: RelevanceProfileActive, RejectThreshold: 0.40, AcceptThreshold: 0.80, CalibrationSlope: 1,
	}
	for _, fixture := range []struct {
		name     string
		score    *float64
		hardVeto bool
		want     MatchDecision
	}{
		{name: "accepted", score: float64Pointer(0.80), want: MatchDecisionAccepted},
		{name: "review", score: float64Pointer(0.60), want: MatchDecisionReview},
		{name: "rejected", score: float64Pointer(0.39), want: MatchDecisionRejected},
		{name: "hard veto", score: nil, hardVeto: true, want: MatchDecisionRejected},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			got, err := DecideDocumentMatch(profile, fixture.score, false, fixture.hardVeto)
			if err != nil {
				t.Fatalf("DecideDocumentMatch(): %v", err)
			}
			if got != fixture.want {
				t.Fatalf("decision = %q, want %q", got, fixture.want)
			}
		})
	}
}

func TestDocumentMatchDecisionRejectsProbabilityOrIdentityDrift(t *testing.T) {
	score := 0.81
	decision := DocumentMatchDecision{
		ID: 1, MonitorID: 2, MonitorVersionID: 3, CompiledProfileID: 4,
		DocumentVersionID: 5, RelevanceProfileID: 6,
		MatchingAlgorithmVersion: "rrf-k60-v1", RerankerVersion: "cross-encoder-v1",
		CalibrationVersion: "temperature-2026-08", InputHash: repeatedHex('a'),
		RelevanceProbability: &score, Decision: MatchDecisionAccepted,
		ReasonCodes: []string{"calibrated_accept"},
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("valid decision: %v", err)
	}
	decision.InputHash = "not-a-hash"
	if err := decision.Validate(); err == nil {
		t.Fatal("invalid input hash accepted")
	}
	decision.InputHash = repeatedHex('a')
	invalid := 1.01
	decision.RelevanceProbability = &invalid
	if err := decision.Validate(); err == nil {
		t.Fatal("probability above one accepted")
	}
}

func float64Pointer(value float64) *float64 { return &value }

func repeatedHex(value byte) string {
	result := make([]byte, 64)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
