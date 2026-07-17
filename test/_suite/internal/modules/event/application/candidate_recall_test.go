package application

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type recallReaderFake struct {
	vectorErr                     error
	all, lexical, temporal        []domain.Candidate
	fingerprint, vectorCandidates []domain.Candidate
}

func (reader recallReaderFake) Lexical(context.Context, int64, int) ([]domain.Candidate, error) {
	if reader.lexical != nil {
		return reader.lexical, nil
	}
	return reader.all, nil
}
func (reader recallReaderFake) Temporal(context.Context, int64, int) ([]domain.Candidate, error) {
	if reader.temporal != nil {
		return reader.temporal, nil
	}
	return reader.all, nil
}
func (reader recallReaderFake) Fingerprint(context.Context, int64, int) ([]domain.Candidate, error) {
	if reader.fingerprint != nil {
		return reader.fingerprint, nil
	}
	return reader.all, nil
}
func (reader recallReaderFake) Vector(context.Context, int64, int) ([]domain.Candidate, error) {
	if reader.vectorErr != nil {
		return nil, reader.vectorErr
	}
	return reader.vectorCandidates, nil
}

func TestRecallCandidateLimitDeduplicatesAndCapsTheUnion(t *testing.T) {
	candidates := make([]domain.Candidate, 0, domain.MaxCandidates+4)
	for i := 0; i < domain.MaxCandidates+4; i++ {
		candidates = append(candidates, domain.Candidate{EventID: int64(i + 1), EventKey: "evt_" + string(rune('a'+i)), Channel: domain.ChannelLexical, Score: float64(i)})
	}
	result, err := NewRecallService(recallReaderFake{all: candidates, vectorErr: errors.New("no vector")}).Recall(context.Background(), RecallInput{ContentID: 1})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !result.VectorUnavailable || len(result.Candidates) != domain.MaxCandidates {
		t.Fatalf("Recall() = %d candidates, vectorUnavailable=%t; want 50,true", len(result.Candidates), result.VectorUnavailable)
	}
	if result.Candidates[0].Score <= result.Candidates[len(result.Candidates)-1].Score {
		t.Fatalf("Recall() is not score ordered: %#v", result.Candidates)
	}
}

func TestRecallRejectsInvalidCandidates(t *testing.T) {
	_, err := NewRecallService(recallReaderFake{all: []domain.Candidate{{EventID: 0, EventKey: "bad", Channel: domain.ChannelLexical}}}).Recall(context.Background(), RecallInput{ContentID: 1})
	if err == nil {
		t.Fatal("Recall() accepted invalid candidate")
	}
}

func TestRecallPreservesAllPathsAndCandidateEvidence(t *testing.T) {
	base := func(channel domain.CandidateChannel, score float64, evidence int64) domain.Candidate {
		return domain.Candidate{EventID: 7, EventKey: "evt_7", Channel: channel, Score: score, EvidenceContentIDs: []int64{evidence}}
	}
	result, err := NewRecallService(recallReaderFake{
		lexical:          []domain.Candidate{base(domain.ChannelLexical, 80, 41)},
		temporal:         []domain.Candidate{base(domain.ChannelTemporal, 85, 42)},
		fingerprint:      []domain.Candidate{base(domain.ChannelFingerprint, 90, 43)},
		vectorCandidates: []domain.Candidate{base(domain.ChannelVector, 95, 44)},
	}).Recall(context.Background(), RecallInput{ContentID: 1})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("Recall() candidates = %#v", result.Candidates)
	}
	candidate := result.Candidates[0]
	if candidate.Channel != domain.ChannelVector || candidate.Score != 95 {
		t.Fatalf("primary candidate = %#v", candidate)
	}
	if len(candidate.Sources()) != 4 || len(candidate.EvidenceContentIDs) != 4 {
		t.Fatalf("candidate provenance = %#v", candidate)
	}
	for index, contentID := range candidate.EvidenceContentIDs {
		if contentID != int64(41+index) {
			t.Fatalf("candidate evidence = %#v", candidate.EvidenceContentIDs)
		}
	}
}
