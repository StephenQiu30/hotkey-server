package domain

import (
	"math"
	"testing"
)

func TestReciprocalRankFusionUsesOnlyRanksAndPreservesSignals(t *testing.T) {
	signals := []RecallSignal{
		mustRecallSignal(t, 11, RecallChannelLexical, 1, 0.01, "lexical-v1"),
		mustRecallSignal(t, 12, RecallChannelLexical, 2, 999, "lexical-v1"),
		mustRecallSignal(t, 12, RecallChannelSemantic, 1, 0.02, "semantic-v1"),
	}

	result, err := FuseRecallSignals(signals, 60, 200)
	if err != nil {
		t.Fatalf("FuseRecallSignals() error = %v", err)
	}
	if len(result) != 2 || result[0].DocumentVersionID() != 12 || result[1].DocumentVersionID() != 11 {
		t.Fatalf("fused order = %#v, want document 12 then 11", result)
	}
	want := 1.0/62.0 + 1.0/61.0
	if math.Abs(result[0].RRFScore()-want) > 1e-12 {
		t.Fatalf("RRF score = %.15f, want %.15f", result[0].RRFScore(), want)
	}
	preserved := result[0].Signals()
	if len(preserved) != 2 || preserved[0].RawScore() != 999 || preserved[1].RawScore() != 0.02 {
		t.Fatalf("signals did not preserve channel raw scores: %#v", preserved)
	}
}

func TestReciprocalRankFusionRejectsDuplicateChannelFacts(t *testing.T) {
	_, err := FuseRecallSignals([]RecallSignal{
		mustRecallSignal(t, 7, RecallChannelLexical, 1, 0.9, "lexical-v1"),
		mustRecallSignal(t, 7, RecallChannelLexical, 2, 0.8, "lexical-v1"),
	}, 60, 200)
	if err == nil {
		t.Fatal("FuseRecallSignals() error = nil, want duplicate-channel rejection")
	}
}

func mustRecallSignal(t *testing.T, documentVersionID int64, channel RecallChannel, rank int, rawScore float64, algorithmVersion string) RecallSignal {
	t.Helper()
	signal, err := NewRecallSignal(documentVersionID, channel, rank, rawScore, algorithmVersion)
	if err != nil {
		t.Fatalf("NewRecallSignal() error = %v", err)
	}
	return signal
}
