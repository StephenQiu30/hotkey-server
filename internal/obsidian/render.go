package obsidian

import (
	"fmt"
	"strings"
	"time"
)

type MarkdownInput struct {
	Kind        ExportKind
	Date        time.Time
	ReportID    int64
	MonitorID   int64
	MonitorName string
	Title       string
	Content     string
}

func RenderMarkdown(input MarkdownInput) (string, error) {
	switch input.Kind {
	case ExportDailyDigest:
		return renderDailyDigest(input), nil
	case ExportPublishDraft:
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
