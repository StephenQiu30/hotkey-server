package application

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type recallReaderFake struct {
	vectorErr error
	all       []domain.Candidate
}

func (reader recallReaderFake) Lexical(context.Context, int64, int) ([]domain.Candidate, error) {
	return reader.all, nil
}
func (reader recallReaderFake) Temporal(context.Context, int64, int) ([]domain.Candidate, error) {
	return reader.all, nil
}
func (reader recallReaderFake) Fingerprint(context.Context, int64, int) ([]domain.Candidate, error) {
	return reader.all, nil
}
func (reader recallReaderFake) Vector(context.Context, int64, int) ([]domain.Candidate, error) {
	return nil, reader.vectorErr
}

func TestRecallBoundDeduplicatesAndCapsTheUnion(t *testing.T) {
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
