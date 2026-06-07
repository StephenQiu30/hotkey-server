package normalize

import (
	"context"
	"errors"
	"html"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/strutil"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrEmptyContent = errors.New("empty content after normalization")
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
	MaxTitleRunes   int
	MaxSnippetRunes int
	MaxContentRunes int
	DefaultLanguage string
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
	sourceID := strings.TrimSpace(input.SourceID)
	if sourceID == "" {
		return Result{}, ErrInvalidInput
	}

	canonicalURL, err := content.CanonicalURL(input.URL)
	if err != nil {
		return Result{}, ErrInvalidInput
	}

	title := cleanText(input.Title)
	snippet := cleanText(input.Snippet)
	rawContent := cleanText(input.RawContent)

	title = strutil.TrimRunes(title, s.cfg.MaxTitleRunes)
	snippet = strutil.TrimRunes(snippet, s.cfg.MaxSnippetRunes)
	rawContent = strutil.TrimRunes(rawContent, s.cfg.MaxContentRunes)

	if title == "" && snippet == "" {
		return Result{}, ErrEmptyContent
	}

	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = detectLanguage(title + " " + snippet)
	}
	if language == "" {
		language = s.cfg.DefaultLanguage
	}

	now := s.now().UTC()
	return Result{
		Item: content.SourceItem{
			ID:           content.NewID(),
			SourceID:     sourceID,
			Title:        title,
			Snippet:      snippet,
			RawURL:       strings.TrimSpace(input.URL),
			CanonicalURL: canonicalURL,
			PublishedAt:  strutil.CloneTime(input.PublishedAt),
			ContentHash: content.ContentHash(content.HashInput{
				Title:        title,
				Snippet:      snippet,
				CanonicalURL: canonicalURL,
			}),
			Language:  language,
			Status:    content.ItemStatusPrimary,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}, nil
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)
var multiSpaceRe = regexp.MustCompile(`\s+`)
var scriptRe = regexp.MustCompile(`(?is)<script[\s>].*?</script>`)
var styleRe = regexp.MustCompile(`(?is)<style[\s>].*?</style>`)

func cleanText(value string) string {
	value = scriptRe.ReplaceAllString(value, "")
	value = styleRe.ReplaceAllString(value, "")
	value = htmlTagRe.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	value = multiSpaceRe.ReplaceAllString(strings.TrimSpace(value), " ")
	return value
}

func detectLanguage(text string) string {
	var chinese, latin int
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			chinese++
		} else if unicode.Is(unicode.Latin, r) {
			latin++
		}
	}
	total := chinese + latin
	if total == 0 {
		return "unknown"
	}
	threshold := 0.3
	if float64(chinese)/float64(total) >= threshold {
		return "zh"
	}
	if float64(latin)/float64(total) >= threshold {
		return "en"
	}
	return "unknown"
}
