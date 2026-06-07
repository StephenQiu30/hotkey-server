package obsidian

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	domain "github.com/StephenQiu30/hotkey-server/internal/domain/obsidian"
)

// Config holds service-level configuration.
type Config struct {
	Now func() time.Time
}

// Service implements the Obsidian Git sync business logic.
type Service struct {
	repo domain.Repository
	git  GitProvider
	now  func() time.Time
}

func NewService(repo domain.Repository, git GitProvider, cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, git: git, now: now}
}

// --- Connect / Disconnect ---

type ConnectInput struct {
	UserID             string
	RepoURL            string
	Branch             string
	BaseDir            string
	AccessToken        string
	EventNoteTemplate  string
	DailyReportIndex   string
	WeeklyReportIndex  string
	ConflictResolution domain.ConflictResolution
}

type ConnectResult struct {
	ConfigID      string
	Status        domain.SyncStatus
	DefaultBranch string
	Branches      []string
}

func (s *Service) Connect(ctx context.Context, input ConnectInput) (ConnectResult, error) {
	input.RepoURL = strings.TrimSpace(input.RepoURL)
	input.Branch = strings.TrimSpace(input.Branch)
	input.BaseDir = strings.TrimSpace(input.BaseDir)
	input.AccessToken = strings.TrimSpace(input.AccessToken)

	if input.RepoURL == "" || input.AccessToken == "" {
		return ConnectResult{}, ErrInvalidInput
	}

	// Validate repo
	validate, err := s.git.ValidateRepo(ctx, input.RepoURL, input.Branch, input.AccessToken)
	if err != nil {
		return ConnectResult{}, err
	}

	// Check branch exists
	branch := input.Branch
	if branch == "" {
		branch = validate.DefaultBranch
	}
	if !branchExists(validate.Branches, branch) {
		return ConnectResult{}, ErrBranchNotFound
	}

	// Check base dir exists if specified
	if input.BaseDir != "" {
		exists, err := s.git.CheckDirExists(ctx, input.RepoURL, branch, input.BaseDir, input.AccessToken)
		if err != nil {
			return ConnectResult{}, err
		}
		if !exists {
			return ConnectResult{}, ErrDirNotFound
		}
	}

	// Set defaults
	if input.EventNoteTemplate == "" {
		input.EventNoteTemplate = defaultEventNoteTemplate
	}
	if input.ConflictResolution == "" {
		input.ConflictResolution = domain.ConflictResolutionSkip
	}

	now := s.now().UTC()
	cfg := domain.SyncConfig{
		UserID:             input.UserID,
		RepoURL:            input.RepoURL,
		Branch:             branch,
		BaseDir:            input.BaseDir,
		AccessToken:        input.AccessToken,
		EventNoteTemplate:  input.EventNoteTemplate,
		DailyReportIndex:   input.DailyReportIndex,
		WeeklyReportIndex:  input.WeeklyReportIndex,
		ConflictResolution: input.ConflictResolution,
		Status:             domain.SyncStatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	cfg, err = s.repo.SaveConfig(ctx, cfg)
	if err != nil {
		return ConnectResult{}, err
	}

	return ConnectResult{
		ConfigID:      cfg.ID,
		Status:        cfg.Status,
		DefaultBranch: validate.DefaultBranch,
		Branches:      validate.Branches,
	}, nil
}

func (s *Service) Disconnect(ctx context.Context, userID string) error {
	cfg, err := s.repo.FindConfigByUser(ctx, userID)
	if err != nil {
		return ErrNotConnected
	}
	return s.repo.DeleteConfig(ctx, cfg.ID)
}

// --- GetStatus ---

type StatusResult struct {
	ConfigID  string
	RepoURL   string
	Branch    string
	BaseDir   string
	Status    domain.SyncStatus
	LastError string
	LastSync  *time.Time
}

func (s *Service) GetStatus(ctx context.Context, userID string) (StatusResult, error) {
	cfg, err := s.repo.FindConfigByUser(ctx, userID)
	if err != nil {
		return StatusResult{}, ErrNotConnected
	}
	return StatusResult{
		ConfigID:  cfg.ID,
		RepoURL:   cfg.RepoURL,
		Branch:    cfg.Branch,
		BaseDir:   cfg.BaseDir,
		Status:    cfg.Status,
		LastError: cfg.LastError,
		LastSync:  cfg.LastSyncAt,
	}, nil
}

// --- Markdown rendering ---

type EventNoteInput struct {
	Title   string
	Body    string
	Tags    []string
	Date    string
	Sources []SourceRef
}

type SourceRef struct {
	Title string
	URL   string
}

type ReportIndexInput struct {
	Date    string
	Reports []ReportIndexEntry
}

type ReportIndexEntry struct {
	Title    string
	FilePath string
}

func (s *Service) RenderEventNote(input EventNoteInput) string {
	var b strings.Builder

	// Frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: %q\n", input.Title))
	b.WriteString(fmt.Sprintf("date: %s\n", input.Date))
	if len(input.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, t := range input.Tags {
			b.WriteString(fmt.Sprintf("  - %s\n", t))
		}
	}
	b.WriteString("source: hotkey\n")
	b.WriteString("---\n\n")

	// Title
	b.WriteString("# " + input.Title + "\n\n")

	// Body
	b.WriteString(input.Body + "\n")

	// Sources
	if len(input.Sources) > 0 {
		b.WriteString("\n## 来源\n\n")
		for _, s := range input.Sources {
			b.WriteString(fmt.Sprintf("- [%s](%s)\n", s.Title, s.URL))
		}
	}

	return b.String()
}

func (s *Service) RenderDailyReportIndex(input ReportIndexInput) string {
	var b strings.Builder
	b.WriteString("# " + input.Date + " 日报索引\n\n")
	for _, r := range input.Reports {
		b.WriteString(fmt.Sprintf("- [[%s|%s]]\n", r.FilePath, r.Title))
	}
	return b.String()
}

// --- File path generation ---

type PathInput struct {
	ContentType string
	Title       string
	Date        string
	BaseDir     string
	Sequence    int
}

var illegalChars = regexp.MustCompile(`[:*?"<>|\\]`)
var multiDash = regexp.MustCompile(`-{2,}`)

func (s *Service) GenerateFilePath(input PathInput) string {
	slug := illegalChars.ReplaceAllString(input.Title, "-")
	slug = multiDash.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "untitled"
	}

	dir := strings.TrimRight(input.BaseDir, "/") + "/"
	prefix := input.Date + "-" + slug
	if input.Sequence > 1 {
		prefix += fmt.Sprintf("-%d", input.Sequence)
	}
	return dir + prefix + ".md"
}

// --- Sync ---

func (s *Service) Sync(ctx context.Context, input SyncInput) (SyncOutput, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.ContentID = strings.TrimSpace(input.ContentID)
	input.Title = strings.TrimSpace(input.Title)

	if input.UserID == "" || input.ContentID == "" || input.ContentType == "" {
		return SyncOutput{}, ErrInvalidInput
	}

	cfg, err := s.repo.FindConfigByUser(ctx, input.UserID)
	if err != nil {
		return SyncOutput{}, ErrNotConnected
	}
	if cfg.Status != domain.SyncStatusActive {
		return SyncOutput{}, ErrNotConnected
	}

	// Build idempotency key from config + content type + content ID
	idempotencyKey := fmt.Sprintf("%s:%s:%s", cfg.ID, input.ContentType, input.ContentID)

	// Check idempotency
	existing, err := s.repo.FindRecordByIdempotencyKey(ctx, cfg.ID, idempotencyKey)
	if err == nil && existing.State == domain.SyncRecordStateSynced {
		return SyncOutput{
			RecordID:      existing.ID,
			FilePath:      existing.FilePath,
			CommitSHA:     existing.CommitSHA,
			State:         existing.State,
			WasIdempotent: true,
		}, nil
	}

	// Render markdown
	var md string
	switch input.ContentType {
	case "event_note":
		md = s.RenderEventNote(EventNoteInput{
			Title:   input.Title,
			Body:    input.Body,
			Tags:    input.Tags,
			Date:    s.now().UTC().Format("2006-01-02"),
			Sources: toServiceSourceRefs(input.Frontmatter),
		})
	default:
		md = input.Body
	}

	// Generate file path
	filePath := s.GenerateFilePath(PathInput{
		ContentType: input.ContentType,
		Title:       input.Title,
		Date:        s.now().UTC().Format("2006-01-02"),
		BaseDir:     cfg.BaseDir,
	})

	// Check for conflict
	conflict, _ := s.git.PullAndCheckConflict(ctx, cfg.RepoURL, cfg.Branch, filePath, cfg.AccessToken)
	if conflict {
		switch cfg.ConflictResolution {
		case domain.ConflictResolutionSkip:
			record := s.saveRecord(ctx, cfg.ID, input, filePath, idempotencyKey, domain.SyncRecordStateSkipped, "", "conflict skipped")
			return SyncOutput{
				RecordID: record.ID,
				FilePath: filePath,
				State:    domain.SyncRecordStateSkipped,
			}, nil
		case domain.ConflictResolutionBranch:
			// Generate a branch name with timestamp
			filePath = s.GenerateFilePath(PathInput{
				ContentType: input.ContentType,
				Title:       input.Title,
				Date:        s.now().UTC().Format("2006-01-02"),
				BaseDir:     cfg.BaseDir,
				Sequence:    int(s.now().UTC().UnixNano()%1000) + 2,
			})
		}
	}

	// Commit and push
	commitMsg := fmt.Sprintf("hotkey: sync %s - %s", input.ContentType, input.Title)
	result, err := s.git.CommitAndPush(ctx, CommitInput{
		RepoURL:        cfg.RepoURL,
		Branch:         cfg.Branch,
		Dir:            cfg.BaseDir,
		FilePath:       filePath,
		Content:        []byte(md),
		CommitMsg:      commitMsg,
		AccessToken:    cfg.AccessToken,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		// Update config status on auth failure
		if IsAuthError(err) {
			cfg.Status = domain.SyncStatusError
			cfg.LastError = err.Error()
			cfg.UpdatedAt = s.now().UTC()
			s.repo.SaveConfig(ctx, cfg)
		}
		record := s.saveRecord(ctx, cfg.ID, input, filePath, idempotencyKey, domain.SyncRecordStateFailed, "", err.Error())
		return SyncOutput{
			RecordID: record.ID,
			FilePath: filePath,
			State:    domain.SyncRecordStateFailed,
		}, nil
	}

	// Success
	record := s.saveRecord(ctx, cfg.ID, input, filePath, idempotencyKey, domain.SyncRecordStateSynced, result.CommitSHA, "")
	cfg.LastSyncAt = &record.UpdatedAt
	cfg.Status = domain.SyncStatusActive
	cfg.LastError = ""
	cfg.UpdatedAt = s.now().UTC()
	s.repo.SaveConfig(ctx, cfg)

	return SyncOutput{
		RecordID:  record.ID,
		FilePath:  filePath,
		CommitSHA: result.CommitSHA,
		CommitURL: result.CommitURL,
		State:     domain.SyncRecordStateSynced,
	}, nil
}

func (s *Service) saveRecord(ctx context.Context, configID string, input SyncInput, filePath, idempotencyKey string, state domain.SyncRecordState, commitSHA, errMsg string) domain.SyncRecord {
	rec, _ := s.repo.SaveRecord(ctx, domain.SyncRecord{
		ConfigID:       configID,
		ContentType:    input.ContentType,
		ContentID:      input.ContentID,
		FilePath:       filePath,
		IdempotencyKey: idempotencyKey,
		CommitSHA:      commitSHA,
		State:          state,
		Error:          errMsg,
	})
	return rec
}

// --- helpers ---

func branchExists(branches []string, target string) bool {
	for _, b := range branches {
		if b == target {
			return true
		}
	}
	return false
}

func IsAuthError(err error) bool {
	return err != nil && (err.Error() == ErrAuthFailed.Error() || strings.Contains(strings.ToLower(err.Error()), "auth") || strings.Contains(strings.ToLower(err.Error()), "403") || strings.Contains(strings.ToLower(err.Error()), "401"))
}

func toServiceSourceRefs(fm map[string]string) []SourceRef {
	if fm == nil {
		return nil
	}
	var refs []SourceRef
	if url, ok := fm["source_url"]; ok {
		title := fm["source_title"]
		if title == "" {
			title = "来源"
		}
		refs = append(refs, SourceRef{Title: title, URL: url})
	}
	return refs
}

const defaultEventNoteTemplate = `---
title: "{{.Title}}"
date: {{.Date}}
tags:
{{range .Tags}}  - {{.}}
{{end}}source: hotkey
---

# {{.Title}}

{{.Body}}

## 来源

{{range .Sources}}- [{{.Title}}]({{.URL}})
{{end}}`
