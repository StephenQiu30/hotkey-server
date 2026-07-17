// Package application contains Event use cases and bounded read contracts.
package application

import (
	"context"
	"sort"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

const (
	LexicalLimit     = 16
	TemporalLimit    = 12
	FingerprintLimit = 10
	VectorLimit      = 12
)

type CandidateReader interface {
	Lexical(context.Context, int64, int) ([]domain.Candidate, error)
	Temporal(context.Context, int64, int) ([]domain.Candidate, error)
	Fingerprint(context.Context, int64, int) ([]domain.Candidate, error)
	Vector(context.Context, int64, int) ([]domain.Candidate, error)
}

type RecallInput struct {
	ContentID int64
}

type RecallResult struct {
	Candidates        []domain.Candidate
	VectorUnavailable bool
}

type RecallService struct {
	reader CandidateReader
}

func NewRecallService(reader CandidateReader) *RecallService {
	return &RecallService{reader: reader}
}

// Recall calls each bounded channel independently. A missing vector space is
// a safe degradation, not a reason to discard deterministic candidates.
func (service *RecallService) Recall(ctx context.Context, input RecallInput) (RecallResult, error) {
	if service == nil || service.reader == nil || input.ContentID <= 0 {
		return RecallResult{}, domainError("candidate reader and positive content id are required")
	}
	channels := []struct {
		limit int
		call  func(context.Context, int64, int) ([]domain.Candidate, error)
	}{
		{LexicalLimit, service.reader.Lexical},
		{TemporalLimit, service.reader.Temporal},
		{FingerprintLimit, service.reader.Fingerprint},
	}
	all := make([]domain.Candidate, 0, LexicalLimit+TemporalLimit+FingerprintLimit+VectorLimit)
	for _, channel := range channels {
		candidates, err := channel.call(ctx, input.ContentID, channel.limit)
		if err != nil {
			return RecallResult{}, err
		}
		all = append(all, candidates...)
	}
	vectorUnavailable := false
	vectors, err := service.reader.Vector(ctx, input.ContentID, VectorLimit)
	if err != nil {
		vectorUnavailable = true
	} else {
		all = append(all, vectors...)
	}

	type candidateAggregate struct {
		candidate domain.Candidate
		sources   map[domain.CandidateChannel]float64
		evidence  map[int64]struct{}
	}
	byKey := make(map[string]candidateAggregate, len(all))
	for _, candidate := range all {
		if err := candidate.Validate(); err != nil {
			return RecallResult{}, err
		}
		aggregate, exists := byKey[candidate.EventKey]
		if !exists {
			aggregate = candidateAggregate{candidate: candidate, sources: make(map[domain.CandidateChannel]float64), evidence: make(map[int64]struct{})}
		}
		for _, source := range candidate.Sources() {
			if score, exists := aggregate.sources[source.Channel]; !exists || source.Score > score {
				aggregate.sources[source.Channel] = source.Score
			}
		}
		for _, contentID := range candidate.EvidenceContentIDs {
			aggregate.evidence[contentID] = struct{}{}
		}
		if !exists || candidate.Score > aggregate.candidate.Score || candidate.Score == aggregate.candidate.Score && candidate.Channel < aggregate.candidate.Channel {
			aggregate.candidate = candidate
		}
		byKey[candidate.EventKey] = aggregate
	}
	unique := make([]domain.Candidate, 0, len(byKey))
	for _, aggregate := range byKey {
		sources := make([]domain.CandidateRecall, 0, len(aggregate.sources))
		for channel, score := range aggregate.sources {
			sources = append(sources, domain.CandidateRecall{Channel: channel, Score: score})
		}
		sort.Slice(sources, func(left, right int) bool { return sources[left].Channel < sources[right].Channel })
		evidence := make([]int64, 0, len(aggregate.evidence))
		for contentID := range aggregate.evidence {
			evidence = append(evidence, contentID)
		}
		sort.Slice(evidence, func(left, right int) bool { return evidence[left] < evidence[right] })
		aggregate.candidate.RecallSources = sources
		aggregate.candidate.EvidenceContentIDs = evidence
		unique = append(unique, aggregate.candidate)
	}
	return RecallResult{Candidates: domain.CompareCandidates(unique), VectorUnavailable: vectorUnavailable}, nil
}

type inputError string

func (err inputError) Error() string { return string(err) }

func domainError(message string) error { return inputError(message) }
