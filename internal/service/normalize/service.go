package normalize

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrEmptyContent  = errors.New("empty content after normalization")
)

type Input struct {
	SourceID    string
	Title       string
	Snippet     string
	RawContent  string
	URL         string
	Platform    string
	Language    string
	PublishedAt *time.Time
}

type Result struct {
	Item content.SourceItem
}

type Config struct {
	MaxTitleRunes    int
	MaxSnippetRunes  int
	MaxContentRunes  int
	DefaultLanguage  string
}

func DefaultConfig() Config {
	return Config{
		MaxTitleRunes:   256,
		MaxSnippetRunes: 1024,
		MaxContentRunes: 8192,
		DefaultLanguage: "unknown",
	}
}

type Service struct {
	cfg Config
	now func() time.Time
}

func NewService(cfg Config) *Service {
	if cfg.MaxTitleRunes <= 0 {
		cfg.MaxTitleRunes = 256
	}
	if cfg.MaxSnippetRunes <= 0 {
		cfg.MaxSnippetRunes = 1024
	}
	if cfg.MaxContentRunes <= 0 {
		cfg.MaxContentRunes = 8192
	}
	if cfg.DefaultLanguage == "" {
		cfg.DefaultLanguage = "unknown"
	}
	return &Service{cfg: cfg, now: time.Now}
}

func (s *Service) Normalize(_ context.Context, input Input) (Result, error) {
	panic("not implemented")
}

func cleanText(value string) string {
	panic("not implemented")
}

func detectLanguage(text string) string {
	panic("not implemented")
}

func trimRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
