package quality

import (
	"context"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

type Result struct {
	Score        float64
	Summarizable bool
}

type Config struct {
	MinTitleRunesForSummary  int
	MinSnippetRunesForSummary int
	MaxScore                 float64
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
	panic("not implemented")
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}
