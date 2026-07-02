package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

// generateKnowledgeSnapshot creates a complete vault directory structure
// under root using the path matrix and renderers.
func generateKnowledgeSnapshot(root string) error {
	// --- Event ---
	eventPath := obsidian.BuildKnowledgePath(root, obsidian.PathInput{
		Kind:        "event",
		MonitorSlug: "ai-regulation",
		Date:        "2026-07-01",
		StableID:    "evt-101",
		TitleSlug:   "ai-guize",
	})
	eventContent := obsidian.RenderEventNote(obsidian.EventNoteInput{
		EventID:   101,
		EventKey:  "evt:ai-regulation:2026-07-01",
		Title:     "AI 监管规则发布",
		Date:      "2026-07-01",
		Summary:   "监管机构发布新规。",
		TopicIDs:  []int64{42},
	})
	if err := obsidian.WriteAtomic(eventPath, eventContent); err != nil {
		return err
	}

	// --- Topic (using existing BuildPath for compatibility) ---
	topicPath := obsidian.BuildPath(root, "ai-regulation", "2026-07-01", "42", "ai-zhengce")
	topicContent := obsidian.RenderTopicNote(obsidian.TopicNoteInput{
		Date:      "2026-07-01",
		Monitor:   "AI监管",
		MonitorID: 1,
		TopicID:   42,
		TopicKey:  "ai:regulation:policy",
		Title:     "AI 监管政策动态",
		Heat:      85.4,
		Trend:     "rising",
		PostCount: 12,
		Summary:   "监管政策动态摘要。",
	})
	if err := obsidian.WriteAtomic(topicPath, topicContent); err != nil {
		return err
	}

	// --- Daily Digest ---
	digestPath := obsidian.BuildKnowledgePath(root, obsidian.PathInput{
		Kind:        "daily-digest",
		MonitorSlug: "ai-regulation",
		Date:        "2026-07-01",
		StableID:    "ddigest-101",
	})
	digestContent := obsidian.RenderDigestNote(obsidian.DigestNoteInput{
		DigestID:   1001,
		Date:       "2026-07-01",
		Monitor:    "AI监管",
		MonitorID:  1,
		TopicCount: 5,
		EventCount: 3,
		Topics: []obsidian.DigestTopicItem{
			{TopicID: 42, Title: "政策动态", Summary: "摘要内容", HeatScore: 85.4},
		},
		Events: []obsidian.DigestEventItem{
			{EventID: 101, Title: "AI 监管规则发布", Summary: "新规发布"},
		},
		Summary: "今日热点汇总",
	})
	if err := obsidian.WriteAtomic(digestPath, digestContent); err != nil {
		return err
	}

	// --- Theme ---
	themePath := obsidian.BuildKnowledgePath(root, obsidian.PathInput{
		Kind:      "theme",
		StableID:  "thm-42",
		TitleSlug: "ai-security",
	})
	themeContent := obsidian.RenderThemeNote(obsidian.ThemeNoteInput{
		ThemeID:       42,
		Title:         "AI安全监管",
		Summary:       "AI安全相关的综合监管专题",
		RelatedTopics: []string{"AI政策", "数据安全"},
		EventCount:    5,
	})
	if err := obsidian.WriteAtomic(themePath, themeContent); err != nil {
		return err
	}

	return nil
}

// assertDirExists checks that dir exists and is a directory.
func assertDirExists(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected directory %s to exist: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
}

// assertDirHasFiles checks that dir contains at least one .md file.
func assertDirHasFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	hasMD := false
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			hasMD = true
			break
		}
	}
	if !hasMD {
		t.Fatalf("directory %s has no .md files", dir)
	}
}

func TestKnowledgeVaultSnapshot(t *testing.T) {
	root := t.TempDir()

	err := generateKnowledgeSnapshot(root)
	if err != nil {
		t.Fatalf("generateKnowledgeSnapshot: %v", err)
	}

	// Verify all required directories exist with content
	assertDirExists(t, filepath.Join(root, "HotKey", "events", "ai-regulation"))
	assertDirHasFiles(t, filepath.Join(root, "HotKey", "events", "ai-regulation"))

	assertDirExists(t, filepath.Join(root, "HotKey", "topics", "ai-regulation"))
	assertDirHasFiles(t, filepath.Join(root, "HotKey", "topics", "ai-regulation"))

	assertDirExists(t, filepath.Join(root, "HotKey", "digests", "daily", "ai-regulation"))
	assertDirHasFiles(t, filepath.Join(root, "HotKey", "digests", "daily", "ai-regulation"))

	assertDirExists(t, filepath.Join(root, "HotKey", "themes"))
	assertDirHasFiles(t, filepath.Join(root, "HotKey", "themes"))
}

func TestKnowledgeVaultSnapshot_FileContent(t *testing.T) {
	root := t.TempDir()

	err := generateKnowledgeSnapshot(root)
	if err != nil {
		t.Fatalf("generateKnowledgeSnapshot: %v", err)
	}

	// Verify that generated .md files contain valid frontmatter
	entries := []string{
		filepath.Join(root, "HotKey", "events", "ai-regulation"),
		filepath.Join(root, "HotKey", "topics", "ai-regulation"),
		filepath.Join(root, "HotKey", "digests", "daily", "ai-regulation"),
		filepath.Join(root, "HotKey", "themes"),
	}

	for _, dir := range entries {
		files, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err != nil {
			t.Fatalf("Glob(%s): %v", dir, err)
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", f, err)
			}
			content := string(data)
			if !strings.HasPrefix(content, "---\n") {
				t.Errorf("file %s does not start with frontmatter delimiter", f)
			}
			if !strings.Contains(content, "\n---\n") {
				t.Errorf("file %s missing closing frontmatter delimiter", f)
			}
		}
	}
}
