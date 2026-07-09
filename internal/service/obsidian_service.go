package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

// Obsidian sentinel errors.
var (
	ErrMissingVaultRoot  = errors.New("missing obsidian vault root")
	ErrInvalidExportKind = errors.New("invalid obsidian export kind")
)

// MarkdownInput defines the input for rendering an Obsidian markdown file.
type MarkdownInput struct {
	Kind        dto.ExportKind
	Date        time.Time
	ReportID    int64
	MonitorID   int64
	MonitorName string
	Title       string
	Content     string
}

// RenderMarkdown generates Obsidian-style markdown for the given input.
func RenderMarkdown(input MarkdownInput) (string, error) {
	switch input.Kind {
	case dto.ExportDailyDigest:
		return renderDailyDigest(input), nil
	case dto.ExportPublishDraft:
		return renderPublishDraft(input), nil
	default:
		return "", ErrInvalidExportKind
	}
}

func renderDailyDigest(input MarkdownInput) string {
	return frontmatter("hotkey-digest", input, "material", []string{"hotkey", "digest", "daily"}, nil) +
		"\n# " + input.Title + "\n\n" +
		strings.TrimSpace(input.Content) + "\n"
}

func renderPublishDraft(input MarkdownInput) string {
	return frontmatter("hotkey-publish-draft", input, "draft", []string{"hotkey", "publish-draft"}, []string{"wechat", "zhihu", "website"}) +
		"\n# " + input.Title + "\n\n" +
		strings.TrimSpace(input.Content) + "\n"
}

func frontmatter(kind string, input MarkdownInput, publishStatus string, tags []string, targetPlatforms []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("type: %s\n", kind))
	b.WriteString(fmt.Sprintf("date: %s\n", input.Date.Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("report_id: %d\n", input.ReportID))
	b.WriteString("report_type: daily\n")
	b.WriteString(fmt.Sprintf("monitor: %s\n", yamlQuote(input.MonitorName)))
	b.WriteString(fmt.Sprintf("monitor_id: %d\n", input.MonitorID))
	b.WriteString("source: hotkey-server\n")
	b.WriteString(fmt.Sprintf("publish_status: %s\n", publishStatus))
	if len(targetPlatforms) > 0 {
		b.WriteString("target_platforms:\n")
		for _, platform := range targetPlatforms {
			b.WriteString(fmt.Sprintf("  - %s\n", platform))
		}
	}
	b.WriteString("tags:\n")
	for _, tag := range tags {
		b.WriteString(fmt.Sprintf("  - %s\n", tag))
	}
	b.WriteString("---\n")
	return b.String()
}

func yamlQuote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\r", `\r`)
	return `"` + escaped + `"`
}

// WriteAtomicNoOverwrite writes content to path atomically, skipping if the file exists.
func WriteAtomicNoOverwrite(path string, content []byte) (dto.WriteResult, error) {
	if _, err := os.Stat(path); err == nil {
		return dto.WriteResult{Path: path, Status: dto.WriteStatusSkipped, Skipped: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return dto.WriteResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return dto.WriteResult{}, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return dto.WriteResult{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return dto.WriteResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return dto.WriteResult{}, err
	}

	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return dto.WriteResult{Path: path, Status: dto.WriteStatusSkipped, Skipped: true}, nil
		}
		return dto.WriteResult{}, err
	}
	return dto.WriteResult{Path: path, Status: dto.WriteStatusPublished}, nil
}

var nonSlugChar = regexp.MustCompile(`[^a-z0-9]+`)

// BuildPath constructs the Obsidian file path for an export.
func BuildPath(root string, input dto.PathInput) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", ErrMissingVaultRoot
	}
	date := input.Date.Format("2006-01-02")
	slug := Slugify(input.MonitorName)
	switch input.Kind {
	case dto.ExportDailyDigest:
		return filepath.Join(root, "HotKey", "digests", "daily", date, slug+".md"), nil
	case dto.ExportPublishDraft:
		return filepath.Join(root, "HotKey", "publish", "drafts", date, slug+".md"), nil
	default:
		return "", ErrInvalidExportKind
	}
}

// Slugify converts a string to a URL-safe slug.
func Slugify(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = nonSlugChar.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "monitor"
	}
	return normalized
}
