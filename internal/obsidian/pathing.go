package obsidian

import (
	"fmt"
	"path/filepath"
)

// PathInput holds all data needed to generate a knowledge object file path.
type PathInput struct {
	Kind        string // 知识类型: event / topic / daily-digest / theme / *-export
	MonitorSlug string // 监控 slug（可选，部分类型使用）
	Date        string // 日期或周期标识（可选，部分类型使用）
	StableID    string // 稳定 ID
	TitleSlug   string // 标题 slug（可选，部分类型使用）
}

// BuildKnowledgePath constructs the full file path for a knowledge object
// inside the Obsidian vault. Uses an explicit switch-case matrix so each
// kind's path format is independently auditable.
func BuildKnowledgePath(root string, in PathInput) string {
	base := filepath.Join(root, "HotKey")

	switch in.Kind {
	case "event":
		return filepath.Join(base, "events", in.MonitorSlug,
			fmt.Sprintf("%s-%s-%s.md", in.Date, in.StableID, in.TitleSlug))
	case "topic":
		return filepath.Join(base, "topics", in.MonitorSlug,
			fmt.Sprintf("%s-topic-%s-%s.md", in.Date, in.StableID, in.TitleSlug))
	case "daily-digest":
		return filepath.Join(base, "digests", "daily", in.MonitorSlug,
			fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
	case "theme":
		return filepath.Join(base, "themes",
			fmt.Sprintf("%s-%s.md", in.StableID, in.TitleSlug))
	case "daily-export":
		return filepath.Join(base, "exports", "daily", in.MonitorSlug,
			fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
	case "weekly-export":
		return filepath.Join(base, "exports", "weekly", in.MonitorSlug,
			fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
	case "monthly-export":
		return filepath.Join(base, "exports", "monthly", in.MonitorSlug,
			fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
	case "thematic-export":
		return filepath.Join(base, "exports", "thematic",
			fmt.Sprintf("%s-%s-%s.md", in.Date, in.StableID, in.TitleSlug))
	case "material-export":
		return filepath.Join(base, "exports", "material",
			fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
	default:
		if in.StableID == "" && in.TitleSlug == "" {
			return filepath.Join(base, "misc", "unfiled.md")
		}
		return filepath.Join(base, "misc",
			fmt.Sprintf("%s-%s.md", in.Date, in.StableID))
	}
}
