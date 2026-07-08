package obsidian

import (
	"path/filepath"
	"regexp"
	"strings"
)

var nonSlugChar = regexp.MustCompile(`[^a-z0-9]+`)

func BuildPath(root string, input PathInput) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", ErrMissingVaultRoot
	}
	date := input.Date.Format("2006-01-02")
	slug := Slugify(input.MonitorName)
	switch input.Kind {
	case ExportDailyDigest:
		return filepath.Join(root, "HotKey", "digests", "daily", date, slug+".md"), nil
	case ExportPublishDraft:
		return filepath.Join(root, "HotKey", "publish", "drafts", date, slug+".md"), nil
	default:
		return "", ErrInvalidExportKind
	}
}

func Slugify(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = nonSlugChar.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "monitor"
	}
	return normalized
}
