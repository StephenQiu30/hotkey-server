package export

import (
	"fmt"
	"strings"
)

// PeriodicReportInput holds data for rendering a periodic report (daily/weekly/monthly).
type PeriodicReportInput struct {
	Title       string
	PeriodLabel string
	TopicCount  int
	EventCount  int
	Topics      []ReportTopicItem
	Summary     string
}

// ReportTopicItem represents a topic entry in a periodic report.
type ReportTopicItem struct {
	Rank      int
	Title     string
	HeatScore float64
	Trend     string
}

// ThematicReportInput holds data for rendering a thematic report.
type ThematicReportInput struct {
	Title   string
	Summary string
	Topics  []string
	Events  []string
}

// MaterialListInput holds data for rendering a material list.
type MaterialListInput struct {
	ThemeTitle string
	Items      []MaterialItem
}

// MaterialItem represents a single item in a material list.
type MaterialItem struct {
	Fact      string
	SourceURL string
	Author    string
	Date      string
}

// frontmatterStart writes the YAML frontmatter opening delimiter.
func frontmatterStart(b *strings.Builder) {
	b.WriteString("---\n")
}

// frontmatterEnd writes the YAML frontmatter closing delimiter and a blank line.
func frontmatterEnd(b *strings.Builder) {
	b.WriteString("---\n\n")
}

// writeExportFrontmatter writes the common export frontmatter fields.
func writeExportFrontmatter(b *strings.Builder, kind ExportKind, title string) {
	fmt.Fprintf(b, "type: hotkey-export\n")
	fmt.Fprintf(b, "export_kind: %s\n", kind)
	if title != "" {
		fmt.Fprintf(b, "title: %s\n", title)
	}
	b.WriteString("tags:\n")
	b.WriteString("  - hotkey\n")
	b.WriteString("  - export\n")
}

// RenderPeriodicReport renders a periodic report (daily, weekly, monthly)
// as an Obsidian-compatible Markdown note.
func RenderPeriodicReport(bundle ExportBundle, input PeriodicReportInput) string {
	var b strings.Builder

	frontmatterStart(&b)
	writeExportFrontmatter(&b, bundle.Kind, input.Title)
	fmt.Fprintf(&b, "period: %s\n", input.PeriodLabel)
	if bundle.DateRange.Start != "" {
		fmt.Fprintf(&b, "date_start: %s\n", bundle.DateRange.Start)
	}
	if bundle.DateRange.End != "" {
		fmt.Fprintf(&b, "date_end: %s\n", bundle.DateRange.End)
	}
	fmt.Fprintf(&b, "topic_count: %d\n", input.TopicCount)
	fmt.Fprintf(&b, "event_count: %d\n", input.EventCount)
	frontmatterEnd(&b)

	// Title heading
	fmt.Fprintf(&b, "# %s\n\n", input.Title)

	// Overview section
	b.WriteString("## 本周概览\n\n")
	fmt.Fprintf(&b, "**周期**: %s\n\n", input.PeriodLabel)
	fmt.Fprintf(&b, "- 热点主题数: %d\n", input.TopicCount)
	fmt.Fprintf(&b, "- 重要事件数: %d\n", input.EventCount)

	if input.Summary != "" {
		fmt.Fprintf(&b, "\n%s\n\n", input.Summary)
	}

	// Topic ranking
	if len(input.Topics) > 0 {
		b.WriteString("## 热点主题排行\n\n")
		for _, t := range input.Topics {
			fmt.Fprintf(&b, "%d. **%s** — 热度: %.1f | 趋势: %s\n",
				t.Rank, t.Title, t.HeatScore, t.Trend)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// RenderThematicReport renders a thematic report as an Obsidian-compatible Markdown note.
func RenderThematicReport(bundle ExportBundle, input ThematicReportInput) string {
	var b strings.Builder

	frontmatterStart(&b)
	writeExportFrontmatter(&b, bundle.Kind, input.Title)
	frontmatterEnd(&b)

	fmt.Fprintf(&b, "# %s\n\n", input.Title)

	if input.Summary != "" {
		b.WriteString(input.Summary)
		b.WriteString("\n\n")
	}

	if len(input.Topics) > 0 {
		b.WriteString("## 涉及主题\n\n")
		for _, t := range input.Topics {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}

	if len(input.Events) > 0 {
		b.WriteString("## 相关事件\n\n")
		for _, e := range input.Events {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// RenderMaterialList renders a material list as an Obsidian-compatible Markdown note.
func RenderMaterialList(input MaterialListInput) string {
	var b strings.Builder

	frontmatterStart(&b)
	writeExportFrontmatter(&b, ExportMaterial, "")
	fmt.Fprintf(&b, "theme_title: %s\n", input.ThemeTitle)
	b.WriteString("source_type: material\n")
	frontmatterEnd(&b)

	fmt.Fprintf(&b, "# 素材清单: %s\n\n", input.ThemeTitle)

	if len(input.Items) > 0 {
		for i, item := range input.Items {
			fmt.Fprintf(&b, "%d. **%s**\n", i+1, item.Fact)
			if item.SourceURL != "" {
				fmt.Fprintf(&b, "   SourceURL: %s\n", item.SourceURL)
			}
			if item.Author != "" {
				fmt.Fprintf(&b, "   Source: %s", item.Author)
				if item.Date != "" {
					fmt.Fprintf(&b, " (%s)", item.Date)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}
