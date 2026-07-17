// Package application contains Event use cases and bounded read contracts.
package application

import (
	"context"

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

	byKey := make(map[string]domain.Candidate, len(all))
	for _, candidate := range all {
		if err := candidate.Validate(); err != nil {
			return RecallResult{}, err
		}
		if existing, ok := byKey[candidate.EventKey]; !ok || candidate.Score > existing.Score || candidate.Score == existing.Score && candidate.Channel < existing.Channel {
			byKey[candidate.EventKey] = candidate
		}
	}
	unique := make([]domain.Candidate, 0, len(byKey))
	for _, candidate := range byKey {
		unique = append(unique, candidate)
	}
	return RecallResult{Candidates: domain.CompareCandidates(unique), VectorUnavailable: vectorUnavailable}, nil
}

type inputError string

func (err inputError) Error() string { return string(err) }

func domainError(message string) error { return inputError(message) }
