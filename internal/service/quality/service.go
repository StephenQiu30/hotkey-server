package quality

import (
	"context"
	"math"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

type Result struct {
	Score        float64
	Summarizable bool
}

type Config struct {
	MinTitleRunesForSummary   int
	MinSnippetRunesForSummary int
	MaxScore                  float64
}

func DefaultConfig() Config {
	return Config{
		MinTitleRunesForSummary:   5,
		MinSnippetRunesForSummary: 20,
		MaxScore:                  1.0,
	}
}

type Service struct {
	cfg Config
}

func NewService(cfg Config) *Service {
	if cfg.MaxScore <= 0 {
		cfg.MaxScore = 1.0
	}
	if cfg.MinTitleRunesForSummary <= 0 {
		cfg.MinTitleRunesForSummary = 5
	}
	if cfg.MinSnippetRunesForSummary <= 0 {
		cfg.MinSnippetRunesForSummary = 20
	}
	return &Service{cfg: cfg}
}

func (s *Service) Score(_ context.Context, item content.SourceItem) (Result, error) {
	score := 0.0

	// Title quality (0.0 - 0.2)
	titleLen := runeLen(item.Title)
	if titleLen > 0 {
		score += 0.2 * math.Min(float64(titleLen)/15.0, 1.0)
	}

	// Snippet quality (0.0 - 0.35)
	snippetLen := runeLen(item.Snippet)
	if snippetLen > 0 {
		score += 0.35 * math.Min(float64(snippetLen)/50.0, 1.0)
	}

	// URL quality (0.0 - 0.15)
	if item.CanonicalURL != "" {
		score += 0.15
	}

	// Language quality (0.0 - 0.15)
	if item.Language != "" && item.Language != "unknown" {
		score += 0.15
	}

	// PublishedAt quality (0.0 - 0.15)
	if item.PublishedAt != nil {
		score += 0.15
	}

	// Summarizable: needs enough title + snippet content
	summarizable := titleLen >= s.cfg.MinTitleRunesForSummary &&
		snippetLen >= s.cfg.MinSnippetRunesForSummary

	return Result{
		Score:        math.Min(score, s.cfg.MaxScore),
		Summarizable: summarizable,
	}, nil
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}
