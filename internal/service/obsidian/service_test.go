package obsidian_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/StephenQiu30/hotkey-server/internal/domain/obsidian"
	svc "github.com/StephenQiu30/hotkey-server/internal/service/obsidian"
)

// --- test doubles ---

type fakeGitProvider struct {
	validateResult svc.ValidateRepoResult
	validateErr    error
	dirExists      bool
	dirErr         error
	commitResult   svc.CommitResult
	commitErr      error
	conflictFound  bool
	conflictErr    error
}

func (f *fakeGitProvider) ValidateRepo(_ context.Context, _, _, _ string) (svc.ValidateRepoResult, error) {
	return f.validateResult, f.validateErr
}
func (f *fakeGitProvider) CheckDirExists(_ context.Context, _, _, _, _ string) (bool, error) {
	return f.dirExists, f.dirErr
}
func (f *fakeGitProvider) CommitAndPush(_ context.Context, _ svc.CommitInput) (svc.CommitResult, error) {
	return f.commitResult, f.commitErr
}
func (f *fakeGitProvider) PullAndCheckConflict(_ context.Context, _, _, _, _ string) (bool, error) {
	return f.conflictFound, f.conflictErr
}

func newTestService(git svc.GitProvider) (*svc.Service, *domain.MemoryRepository) {
	repo := domain.NewMemoryRepository()
	s := svc.NewService(repo, git, svc.Config{
		Now: func() time.Time { return time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC) },
	})
	return s, repo
}

func seedConfig(repo *domain.MemoryRepository, userID string) domain.SyncConfig {
	cfg := domain.SyncConfig{
		ID:                 "cfg-test-1",
		UserID:             userID,
		RepoURL:            "https://github.com/user/vault.git",
		Branch:             "main",
		BaseDir:            "hotkey/",
		AccessToken:        "ghp_test",
		EventNoteTemplate:  "# {{title}}\n\n{{body}}",
		DailyReportIndex:   "daily/",
		WeeklyReportIndex:  "weekly/",
		ConflictResolution: domain.ConflictResolutionSkip,
		Status:             domain.SyncStatusActive,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	repo.SaveConfig(context.Background(), cfg)
	return cfg
}

// --- 1. Connect / Disconnect tests ---

func TestConnect_Success(t *testing.T) {
	git := &fakeGitProvider{
		validateResult: svc.ValidateRepoResult{DefaultBranch: "main", Branches: []string{"main", "develop"}},
		dirExists:      true,
	}
	s, _ := newTestService(git)

	result, err := s.Connect(context.Background(), svc.ConnectInput{
		UserID:      "user-1",
		RepoURL:     "https://github.com/user/vault.git",
		Branch:      "main",
		BaseDir:     "hotkey/",
		AccessToken: "ghp_test",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != domain.SyncStatusActive {
		t.Fatalf("expected status active, got %s", result.Status)
	}
	if result.DefaultBranch != "main" {
		t.Fatalf("expected default branch main, got %s", result.DefaultBranch)
	}
}

func TestConnect_InvalidInput(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	_, err := s.Connect(context.Background(), svc.ConnectInput{UserID: "user-1"})
	if !errors.Is(err, svc.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestConnect_AuthFailed(t *testing.T) {
	git := &fakeGitProvider{validateErr: svc.ErrAuthFailed}
	s, _ := newTestService(git)

	_, err := s.Connect(context.Background(), svc.ConnectInput{
		UserID:      "user-1",
		RepoURL:     "https://github.com/user/vault.git",
		Branch:      "main",
		AccessToken: "bad-token",
	})
	if !errors.Is(err, svc.ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestConnect_BranchNotFound(t *testing.T) {
	git := &fakeGitProvider{
		validateResult: svc.ValidateRepoResult{DefaultBranch: "main", Branches: []string{"main"}},
	}
	s, _ := newTestService(git)

	_, err := s.Connect(context.Background(), svc.ConnectInput{
		UserID:      "user-1",
		RepoURL:     "https://github.com/user/vault.git",
		Branch:      "nonexistent",
		AccessToken: "ghp_test",
	})
	if !errors.Is(err, svc.ErrBranchNotFound) {
		t.Fatalf("expected ErrBranchNotFound, got %v", err)
	}
}

func TestConnect_DirNotFound(t *testing.T) {
	git := &fakeGitProvider{
		validateResult: svc.ValidateRepoResult{DefaultBranch: "main", Branches: []string{"main"}},
		dirExists:      false,
	}
	s, _ := newTestService(git)

	_, err := s.Connect(context.Background(), svc.ConnectInput{
		UserID:      "user-1",
		RepoURL:     "https://github.com/user/vault.git",
		Branch:      "main",
		BaseDir:     "nonexistent/",
		AccessToken: "ghp_test",
	})
	if !errors.Is(err, svc.ErrDirNotFound) {
		t.Fatalf("expected ErrDirNotFound, got %v", err)
	}
}

func TestDisconnect_Success(t *testing.T) {
	git := &fakeGitProvider{}
	s, repo := newTestService(git)
	seedConfig(repo, "user-1")

	err := s.Disconnect(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err = repo.FindConfigByUser(context.Background(), "user-1")
	if !errors.Is(err, domain.ErrConfigNotFound) {
		t.Fatalf("expected config deleted, got %v", err)
	}
}

func TestDisconnect_NotConnected(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	err := s.Disconnect(context.Background(), "user-unknown")
	if !errors.Is(err, svc.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

// --- 2. Markdown rendering snapshot tests ---

func TestRenderEventNote_ContainsFrontmatter(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	md := s.RenderEventNote(svc.EventNoteInput{
		Title:   "AI 热点事件：GPT-5 发布",
		Body:    "OpenAI 发布了 GPT-5，性能提升显著。",
		Tags:    []string{"AI", "GPT"},
		Date:    "2026-06-07",
		Sources: []svc.SourceRef{{Title: "TechCrunch", URL: "https://tc.com/1"}},
	})

	// Must contain YAML frontmatter delimiters
	if !strings.HasPrefix(md, "---\n") {
		t.Fatalf("expected frontmatter start, got first 10 chars: %q", md[:10])
	}
	if !strings.Contains(md, "---\n\n# ") {
		t.Fatalf("expected frontmatter end before title")
	}
	// Must contain title as H1
	if !strings.Contains(md, "# AI 热点事件：GPT-5 发布") {
		t.Fatalf("expected title as H1")
	}
	// Must contain tags
	if !strings.Contains(md, "tags:") {
		t.Fatalf("expected tags in frontmatter")
	}
	// Must contain body
	if !strings.Contains(md, "OpenAI 发布了 GPT-5") {
		t.Fatalf("expected body content")
	}
	// Must contain sources section
	if !strings.Contains(md, "## 来源") {
		t.Fatalf("expected sources section")
	}
}

func TestRenderDailyReportIndex_ContainsLinks(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	md := s.RenderDailyReportIndex(svc.ReportIndexInput{
		Date:    "2026-06-07",
		Reports: []svc.ReportIndexEntry{{Title: "06-07 日报", FilePath: "daily/2026-06-07.md"}},
	})

	if !strings.Contains(md, "# 2026-06-07 日报索引") {
		t.Fatalf("expected index title")
	}
	if !strings.Contains(md, "[[daily/2026-06-07.md|06-07 日报]]") {
		t.Fatalf("expected Obsidian wikilink")
	}
}

// --- 3. File path generation tests ---

func TestGenerateFilePath_EventNote(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	path := s.GenerateFilePath(svc.PathInput{
		ContentType: "event_note",
		Title:       "AI 热点事件：GPT-5 发布",
		Date:        "2026-06-07",
		BaseDir:     "hotkey/events/",
	})

	if !strings.HasPrefix(path, "hotkey/events/2026-06-07-") {
		t.Fatalf("expected date-prefixed path, got %s", path)
	}
	if !strings.HasSuffix(path, ".md") {
		t.Fatalf("expected .md suffix, got %s", path)
	}
	// No illegal characters
	illegal := []string{":", "*", "?", "\"", "<", ">", "|", "\\", "//"}
	for _, ch := range illegal {
		if strings.Contains(path, ch) {
			t.Fatalf("path contains illegal char %q: %s", ch, path)
		}
	}
}

func TestGenerateFilePath_SanitizesIllegalChars(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	path := s.GenerateFilePath(svc.PathInput{
		ContentType: "event_note",
		Title:       "What's <new> in 2026? | Everything*",
		Date:        "2026-06-07",
		BaseDir:     "events/",
	})

	illegal := []string{":", "*", "?", "\"", "<", ">", "|", "\\", "//"}
	for _, ch := range illegal {
		if strings.Contains(path, ch) {
			t.Fatalf("path contains illegal char %q: %s", ch, path)
		}
	}
}

func TestGenerateFilePath_DeduplicatesTitle(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	path1 := s.GenerateFilePath(svc.PathInput{
		ContentType: "event_note",
		Title:       "Same Title",
		Date:        "2026-06-07",
		BaseDir:     "events/",
		Sequence:    1,
	})
	path2 := s.GenerateFilePath(svc.PathInput{
		ContentType: "event_note",
		Title:       "Same Title",
		Date:        "2026-06-07",
		BaseDir:     "events/",
		Sequence:    2,
	})

	if path1 == path2 {
		t.Fatalf("expected different paths for duplicate titles, got same: %s", path1)
	}
}

// --- 4. Sync idempotency tests ---

func TestSync_Idempotent_NoDuplicateCommit(t *testing.T) {
	git := &fakeGitProvider{
		commitResult: svc.CommitResult{CommitSHA: "abc1234", CommitURL: "https://github.com/user/vault/commit/abc1234"},
	}
	s, repo := newTestService(git)
	seedConfig(repo, "user-1")

	input := svc.SyncInput{
		UserID:      "user-1",
		ContentType: "event_note",
		ContentID:   "event-1",
		Title:       "Test Event",
		Body:        "Body content",
	}

	// First sync
	result1, err := s.Sync(context.Background(), input)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if result1.State != domain.SyncRecordStateSynced {
		t.Fatalf("expected synced, got %s", result1.State)
	}

	// Second sync with same input — should be idempotent
	result2, err := s.Sync(context.Background(), input)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !result2.WasIdempotent {
		t.Fatalf("expected idempotent on second sync")
	}
	if result2.CommitSHA != result1.CommitSHA {
		t.Fatalf("expected same commit SHA, got %s vs %s", result1.CommitSHA, result2.CommitSHA)
	}
}

func TestSync_NotConnected(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	_, err := s.Sync(context.Background(), svc.SyncInput{
		UserID:      "user-unknown",
		ContentType: "event_note",
		ContentID:   "event-1",
		Title:       "Test",
	})
	if !errors.Is(err, svc.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestSync_DisconnectedConfig(t *testing.T) {
	git := &fakeGitProvider{}
	s, repo := newTestService(git)
	cfg := seedConfig(repo, "user-1")
	cfg.Status = domain.SyncStatusDisconnected
	repo.SaveConfig(context.Background(), cfg)

	_, err := s.Sync(context.Background(), svc.SyncInput{
		UserID:      "user-1",
		ContentType: "event_note",
		ContentID:   "event-1",
	})
	if !errors.Is(err, svc.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected for disconnected config, got %v", err)
	}
}

// --- 5. Conflict detection tests ---

func TestSync_ConflictSkip(t *testing.T) {
	git := &fakeGitProvider{
		conflictFound: true,
		commitResult:  svc.CommitResult{CommitSHA: "new1234", CommitURL: "https://github.com/user/vault/commit/new1234"},
	}
	s, repo := newTestService(git)
	cfg := seedConfig(repo, "user-1")
	cfg.ConflictResolution = domain.ConflictResolutionSkip
	repo.SaveConfig(context.Background(), cfg)

	result, err := s.Sync(context.Background(), svc.SyncInput{
		UserID:      "user-1",
		ContentType: "event_note",
		ContentID:   "event-1",
		Title:       "Test Event",
		Body:        "Body",
	})
	if err != nil {
		t.Fatalf("expected no error with skip resolution, got %v", err)
	}
	if result.State != domain.SyncRecordStateSkipped {
		t.Fatalf("expected skipped, got %s", result.State)
	}
}

func TestSync_ConflictBranch(t *testing.T) {
	git := &fakeGitProvider{
		conflictFound: true,
		commitResult:  svc.CommitResult{CommitSHA: "br12345", CommitURL: "https://github.com/user/vault/commit/br12345"},
	}
	s, repo := newTestService(git)
	cfg := seedConfig(repo, "user-1")
	cfg.ConflictResolution = domain.ConflictResolutionBranch
	repo.SaveConfig(context.Background(), cfg)

	result, err := s.Sync(context.Background(), svc.SyncInput{
		UserID:      "user-1",
		ContentType: "event_note",
		ContentID:   "event-1",
		Title:       "Test Event",
		Body:        "Body",
	})
	if err != nil {
		t.Fatalf("expected no error with branch resolution, got %v", err)
	}
	if result.State != domain.SyncRecordStateSynced {
		t.Fatalf("expected synced (on new branch), got %s", result.State)
	}
}

// --- 6. Auth failure tests ---

func TestSync_CommitAuthFailure(t *testing.T) {
	git := &fakeGitProvider{
		commitErr: svc.ErrAuthFailed,
	}
	s, repo := newTestService(git)
	seedConfig(repo, "user-1")

	result, err := s.Sync(context.Background(), svc.SyncInput{
		UserID:      "user-1",
		ContentType: "event_note",
		ContentID:   "event-1",
		Title:       "Test Event",
		Body:        "Body",
	})
	if err != nil {
		t.Fatalf("expected no error (recorded in result), got %v", err)
	}
	if result.State != domain.SyncRecordStateFailed {
		t.Fatalf("expected failed, got %s", result.State)
	}

	// Config should be in error state
	cfg, _ := repo.FindConfigByUser(context.Background(), "user-1")
	if cfg.Status != domain.SyncStatusError {
		t.Fatalf("expected config status error, got %s", cfg.Status)
	}
	if !strings.Contains(cfg.LastError, "auth") {
		t.Fatalf("expected auth error in LastError, got %s", cfg.LastError)
	}
}

// --- 7. GetStatus tests ---

func TestGetStatus_Connected(t *testing.T) {
	git := &fakeGitProvider{}
	s, repo := newTestService(git)
	seedConfig(repo, "user-1")

	status, err := s.GetStatus(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if status.Status != domain.SyncStatusActive {
		t.Fatalf("expected active, got %s", status.Status)
	}
	if status.RepoURL != "https://github.com/user/vault.git" {
		t.Fatalf("unexpected repo URL: %s", status.RepoURL)
	}
}

func TestGetStatus_NotConnected(t *testing.T) {
	git := &fakeGitProvider{}
	s, _ := newTestService(git)

	_, err := s.GetStatus(context.Background(), "user-unknown")
	if !errors.Is(err, svc.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}
