package filter

import (
	"context"
	"errors"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

var (
	ErrInvalidInput = errors.New("invalid input")
)

type FilterReason string

const (
	ReasonPassed       FilterReason = "passed"
	ReasonNoKeywords   FilterReason = "no_keyword_match"
	ReasonExcluded     FilterReason = "exclusion_word_match"
	ReasonShortContent FilterReason = "content_too_short"
)

type Result struct {
	Accepted bool
	Reason   FilterReason
}

type Config struct {
	Keywords      []string
	ExcludeWords  []string
	MinTitleRunes int
	MinSnippetRunes int
}

type Service struct {
	cfg Config
}

func NewService(cfg Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Filter(_ context.Context, item content.SourceItem) (Result, error) {
	panic("not implemented")
}

func containsAny(text string, words []string) bool {
	lower := strings.ToLower(text)
	for _, w := range words {
		if strings.Contains(lower, strings.ToLower(w)) {
			return true
		}
	}
	return false
}
