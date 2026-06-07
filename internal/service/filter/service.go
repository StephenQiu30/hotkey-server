package filter

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
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
	Keywords        []string
	ExcludeWords    []string
	MinTitleRunes   int
	MinSnippetRunes int
}

type Service struct {
	cfg Config
}

func NewService(cfg Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Filter(_ context.Context, item content.SourceItem) (Result, error) {
	// Check minimum length first
	if utf8.RuneCountInString(item.Title) < s.cfg.MinTitleRunes ||
		utf8.RuneCountInString(item.Snippet) < s.cfg.MinSnippetRunes {
		return Result{Accepted: false, Reason: ReasonShortContent}, nil
	}

	// Check exclusion words (takes precedence)
	combined := item.Title + " " + item.Snippet
	if len(s.cfg.ExcludeWords) > 0 && containsAny(combined, s.cfg.ExcludeWords) {
		return Result{Accepted: false, Reason: ReasonExcluded}, nil
	}

	// Check keywords (if configured)
	if len(s.cfg.Keywords) > 0 && !containsAny(combined, s.cfg.Keywords) {
		return Result{Accepted: false, Reason: ReasonNoKeywords}, nil
	}

	return Result{Accepted: true, Reason: ReasonPassed}, nil
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
