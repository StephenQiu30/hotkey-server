package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/llm"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
	fakedigest "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/digest"
	fakejobs "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/jobs"
)

// vaultWriterAdapter wraps a function into the jobs.VaultWriter interface.
type vaultWriterAdapter struct {
	fn func(path, content string) error
}

func (a *vaultWriterAdapter) WriteAtomic(path, content string) error {
	return a.fn(path, content)
}

// --- helpers ---

func testMonitorConfig() jobs.MonitorConfig {
	return jobs.MonitorConfig{
		ID:   1,
		Name: "AI 监管",
		Slug: "ai-monitor",
	}
}

func testTopics() []digest.TopicEntry {
	return []digest.TopicEntry{
		{ID: 101, Title: "AI 监管政策", Heat: 95.5},
		{ID: 102, Title: "数据隐私合规", Heat: 82.3},
		{ID: 103, Title: "自动驾驶安全", Heat: 71.0},
	}
}

func testPosts(topicID int64) []digest.PostEntry {
	return []digest.PostEntry{
		{PostID: topicID*100 + 1, AuthorName: "张三", ContentExcerpt: "关于AI监管的讨论...", PostURL: "https://example.com/p/1", MembershipScore: 0.95},
		{PostID: topicID*100 + 2, AuthorName: "李四", ContentExcerpt: "最新政策解读...", PostURL: "https://example.com/p/2", MembershipScore: 0.88},
	}
}

func postsMap(topics []digest.TopicEntry) map[int64][]digest.PostEntry {
	m := make(map[int64][]digest.PostEntry)
	for _, t := range topics {
		m[t.ID] = testPosts(t.ID)
	}
	return m
}

// --- Scenario 1: Idempotent ---

func TestPublishDailyTopics_Idempotent(t *testing.T) {
	topics := testTopics()
	filter := &fakedigest.TopicFilter{
		Topics: topics,
		Posts:  postsMap(topics),
	}
	svc := digest.NewService(filter)
	llmClient := &llm.MockClient{Summary: "这是一篇关于AI监管的摘要。"}
	exporter := fakejobs.NewExportRecorder()
	vaultDir := t.TempDir()
	writer := &vaultWriterAdapter{fn: obsidian.WriteAtomic}

	job := jobs.NewPublishDailyTopicsJob(svc, llmClient, exporter, writer, vaultDir, testMonitorConfig())
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)

	// First run: should publish all topics
	results, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}

	publishedCount := 0
	for _, r := range results {
		if r.Status != "published" {
			t.Errorf("first run: topic %d status=%q, want published", r.TopicID, r.Status)
		}
		publishedCount++
	}

	filesAfterFirst := countFiles(t, vaultDir)
	if filesAfterFirst != len(topics) {
		t.Fatalf("first run: expected %d files, got %d", len(topics), filesAfterFirst)
	}

	// Record content of first run files for comparison
	firstContent := readAllMdFiles(t, vaultDir)

	// Second run: same topic+date → file count stable, content updated (S10)
	llmClient.Summary = "更新后的摘要内容。"
	results2, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("second run: unexpected error: %v", err)
	}

	for _, r := range results2 {
		if r.Status != "published" {
			t.Errorf("second run: topic %d status=%q, want published", r.TopicID, r.Status)
		}
	}

	filesAfterSecond := countFiles(t, vaultDir)
	if filesAfterSecond != filesAfterFirst {
		t.Errorf("idempotent: file count changed from %d to %d", filesAfterFirst, filesAfterSecond)
	}

	// Content SHOULD have changed — S10: 已 published 的记录重复执行时 SHALL 覆盖文件内容
	secondContent := readAllMdFiles(t, vaultDir)
	contentChanged := false
	for path, oldContent := range firstContent {
		newContent, ok := secondContent[path]
		if !ok {
			t.Errorf("idempotent: file %s disappeared after second run", path)
			continue
		}
		if newContent != oldContent {
			contentChanged = true
		}
	}
	if !contentChanged {
		t.Error("idempotent: expected content to be overwritten on second run (S10)")
	}
}

// --- Scenario 2: Failure Isolation ---

func TestPublishDailyTopics_FailureIsolation(t *testing.T) {
	topics := testTopics()
	filter := &fakedigest.TopicFilter{
		Topics: topics,
		Posts:  postsMap(topics),
	}
	svc := digest.NewService(filter)

	// LLM fails for topic 102 only
	failingClient := &failingLLMClient{
		failForTopic: "数据隐私合规",
		normalSummary: "正常摘要。",
	}

	exporter := fakejobs.NewExportRecorder()
	vaultDir := t.TempDir()
	writer := &vaultWriterAdapter{fn: obsidian.WriteAtomic}

	job := jobs.NewPublishDailyTopicsJob(svc, failingClient, exporter, writer, vaultDir, testMonitorConfig())
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)

	results, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the failing topic has status "failed"
	var failedTopic *jobs.ExportResult
	var publishedCount int
	for i, r := range results {
		switch r.Status {
		case "published":
			publishedCount++
		case "failed":
			failedTopic = &results[i]
		}
	}

	if failedTopic == nil {
		t.Fatal("failure isolation: expected at least one topic to fail, none did")
	}
	if failedTopic.TopicID != 102 {
		t.Errorf("failure isolation: expected topic 102 to fail, got topic %d", failedTopic.TopicID)
	}

	// Other topics should be published
	if publishedCount != 2 {
		t.Errorf("failure isolation: expected 2 published, got %d", publishedCount)
	}

	// Files should exist for published topics only
	fileCount := countFiles(t, vaultDir)
	if fileCount != 2 {
		t.Errorf("failure isolation: expected 2 files, got %d", fileCount)
	}

	// Exporter should have marked topic 102 as failed
	if _, hasFailed := exporter.Failed["102:2026-06-13"]; !hasFailed {
		t.Error("failure isolation: topic 102 not marked as failed in exporter")
	}
}

// --- Scenario 3: Vault Permission Failure ---

func TestPublishDailyTopics_VaultPermissionFailed(t *testing.T) {
	topics := []digest.TopicEntry{
		{ID: 201, Title: "量子计算突破", Heat: 88.0},
	}
	filter := &fakedigest.TopicFilter{
		Topics: topics,
		Posts:  postsMap(topics),
	}
	svc := digest.NewService(filter)
	llmClient := &llm.MockClient{Summary: "量子计算摘要。"}
	exporter := fakejobs.NewExportRecorder()

	// Vault writer that always fails (simulating permission denied)
	permErr := errors.New("permission denied: cannot write to vault")
	writer := &vaultWriterAdapter{fn: func(_, _ string) error { return permErr }}
	vaultDir := t.TempDir()

	job := jobs.NewPublishDailyTopicsJob(svc, llmClient, exporter, writer, vaultDir, testMonitorConfig())
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)

	results, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Status != "failed" {
		t.Errorf("vault permission: expected status=failed, got %q", r.Status)
	}
	if r.Error == nil {
		t.Error("vault permission: expected error to be set")
	}

	// No files should exist
	fileCount := countFiles(t, vaultDir)
	if fileCount != 0 {
		t.Errorf("vault permission: expected 0 files, got %d", fileCount)
	}

	// Exporter should have marked as failed
	if _, hasFailed := exporter.Failed["201:2026-06-13"]; !hasFailed {
		t.Error("vault permission: topic 201 not marked as failed in exporter")
	}
}

// --- helpers ---

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			count++
		}
		return nil
	})
	return count
}

// readAllMdFiles reads all .md files under dir and returns a map of path → content.
func readAllMdFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[path] = string(data)
		return nil
	})
	return result
}

// failingLLMClient is an llm.Client that fails for a specific topic title.
type failingLLMClient struct {
	failForTopic  string
	normalSummary string
}

func (c *failingLLMClient) SummarizeTopic(_ context.Context, in llm.TopicSummaryInput) (string, error) {
	if in.TopicTitle == c.failForTopic {
		return "", errors.New("LLM timeout: topic too complex")
	}
	return c.normalSummary, nil
}
