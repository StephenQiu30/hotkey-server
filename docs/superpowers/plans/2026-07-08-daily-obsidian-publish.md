# Daily Obsidian Publish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate daily HotKey content on a backend schedule and persist it as two no-overwrite Markdown files in a configured Obsidian vault: a factual digest and a publish-ready draft.

**Architecture:** `internal/report` remains the source of daily report content. `internal/obsidian` owns local Markdown pathing, frontmatter rendering, and atomic no-overwrite writes. `internal/worker` owns scheduled execution, idempotency through `knowledge_runs`, and per-file export status through `report_exports`.

**Tech Stack:** Go, Gin, GORM, Fx, PostgreSQL, local filesystem, Markdown with YAML frontmatter.

## Global Constraints

- Backend-only: no hotkey-web or hotkey-miniapp changes.
- Obsidian is a local Markdown editing and draft-management workspace, not a bidirectional sync source.
- Do not sync Obsidian edits back into HotKey.
- Do not build an Obsidian plugin.
- Do not publish directly to external platforms in this phase.
- Do not overwrite existing local Markdown files.
- Use global `OBSIDIAN_VAULT_PATH` in this phase; leave room for user-level paths later.
- Generate two files per active monitor per daily run: `daily-digest` and `publish-draft`.
- Use `knowledge_runs` for daily run idempotency with run key `daily-obsidian-publish:{target-date}`.
- Store each file export in `report_exports` with `pending`, `published`, `skipped`, or `failed`.
- Use TDD: write the failing test, run it red, implement, run it green.
- Commit after each task with only that task's files.

## Precondition

This plan assumes the current backend report generation work exists in the worktree:

- `internal/report/model.go`
- `internal/report/repository.go`
- `internal/report/service.go`
- `internal/repository/gormimpl/report.go`
- `internal/platform/http/report.go`
- `reports` table in `db/schema.sql` and `db/migrations/000001_create_all_tables.up.sql`

Before executing Task 1, run:

```bash
go test ./tests/unit/report ./tests/unit/platform/http -run 'TestServiceCreateWeeklyReport|TestReportRoutesCreateReadHTMLAndSend' -count=1
```

Expected: PASS. If this fails because `internal/report` or `reports` does not exist, finish the report-service work before this plan.

---

## File Structure

Create:

- `internal/obsidian/model.go` — shared export kinds, inputs, write status constants.
- `internal/obsidian/pathing.go` — monitor slugging and stable Vault path generation.
- `internal/obsidian/render.go` — YAML frontmatter and Markdown renderers for `daily-digest` and `publish-draft`.
- `internal/obsidian/writer.go` — atomic no-overwrite file writer.
- `internal/report/export_repository.go` — report export persistence interface and domain model.
- `internal/repository/gormimpl/report_export.go` — GORM implementation for `report_exports`.
- `internal/worker/daily_obsidian_publish.go` — scheduled daily export job.
- `internal/worker/daily_scheduler.go` — target date and due-run calculation.
- `tests/unit/obsidian/pathing_test.go`
- `tests/unit/obsidian/render_test.go`
- `tests/unit/obsidian/writer_test.go`
- `tests/unit/worker/daily_scheduler_test.go`
- `tests/unit/worker/daily_obsidian_publish_test.go`

Modify:

- `internal/config/config.go` — add Obsidian and daily digest configuration defaults.
- `internal/config/config_test.go` and `tests/unit/config/config_test.go` — assert defaults.
- `db/schema.sql` — add `report_exports`.
- `db/migrations/000001_create_all_tables.up.sql` — add `report_exports`.
- `db/migrations/000001_create_all_tables.down.sql` — drop `report_exports` before `reports`.
- `internal/repository/gormimpl/model.go` — add `ReportExport` GORM model.
- `internal/fxapp/app.go` — wire export repo, Obsidian exporter, and worker.
- `tests/testutil/db.go` — clean `report_exports`.

---

### Task 1: Configuration and `report_exports` Persistence

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `tests/unit/config/config_test.go`
- Modify: `db/schema.sql`
- Modify: `db/migrations/000001_create_all_tables.up.sql`
- Modify: `db/migrations/000001_create_all_tables.down.sql`
- Modify: `internal/repository/gormimpl/model.go`
- Create: `internal/report/export_repository.go`
- Create: `internal/repository/gormimpl/report_export.go`
- Test: `tests/unit/config/config_test.go`
- Test: `tests/unit/database/architecture_boundary_test.go`

**Interfaces:**
- Consumes: `reports.id` from existing report generation.
- Produces:
  - `config.Config.ObsidianVaultPath string`
  - `config.Config.DailyDigestTime string`
  - `config.Config.DailyDigestTimezone string`
  - `config.Config.DailyDigestTarget string`
  - `config.Config.DailyDigestTopN int`
  - `report.ExportRepository`
  - `report.ReportExport`
  - `gormimpl.NewReportExportRepo(db *gorm.DB) *ReportExportRepo`

- [ ] **Step 1: Write failing config default tests**

Add or extend this test in `tests/unit/config/config_test.go`:

```go
func TestLoadDailyObsidianDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hotkey_test?sslmode=disable")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("OBSIDIAN_VAULT_PATH", "")
	t.Setenv("DAILY_DIGEST_TIME", "")
	t.Setenv("DAILY_DIGEST_TIMEZONE", "")
	t.Setenv("DAILY_DIGEST_TARGET", "")
	t.Setenv("DAILY_DIGEST_TOP_N", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ObsidianVaultPath != "" {
		t.Fatalf("ObsidianVaultPath = %q, want empty default", cfg.ObsidianVaultPath)
	}
	if cfg.DailyDigestTime != "08:00" {
		t.Fatalf("DailyDigestTime = %q, want 08:00", cfg.DailyDigestTime)
	}
	if cfg.DailyDigestTimezone != "Asia/Shanghai" {
		t.Fatalf("DailyDigestTimezone = %q, want Asia/Shanghai", cfg.DailyDigestTimezone)
	}
	if cfg.DailyDigestTarget != "yesterday" {
		t.Fatalf("DailyDigestTarget = %q, want yesterday", cfg.DailyDigestTarget)
	}
	if cfg.DailyDigestTopN != 20 {
		t.Fatalf("DailyDigestTopN = %d, want 20", cfg.DailyDigestTopN)
	}
}
```

- [ ] **Step 2: Run config test red**

Run:

```bash
go test ./tests/unit/config -run TestLoadDailyObsidianDefaults -count=1 -v
```

Expected: FAIL with missing fields such as `cfg.ObsidianVaultPath undefined`.

- [ ] **Step 3: Add config fields and defaults**

Modify `internal/config/config.go`:

```go
type Config struct {
	// existing fields...
	ObsidianVaultPath  string `mapstructure:"OBSIDIAN_VAULT_PATH"`
	DailyDigestTime    string `mapstructure:"DAILY_DIGEST_TIME"`
	DailyDigestTimezone string `mapstructure:"DAILY_DIGEST_TIMEZONE"`
	DailyDigestTarget  string `mapstructure:"DAILY_DIGEST_TARGET"`
	DailyDigestTopN    int    `mapstructure:"DAILY_DIGEST_TOP_N"`
}
```

In `Load()`, after environment binding and unmarshalling:

```go
if cfg.DailyDigestTime == "" {
	cfg.DailyDigestTime = "08:00"
}
if cfg.DailyDigestTimezone == "" {
	cfg.DailyDigestTimezone = "Asia/Shanghai"
}
if cfg.DailyDigestTarget == "" {
	cfg.DailyDigestTarget = "yesterday"
}
if cfg.DailyDigestTopN == 0 {
	cfg.DailyDigestTopN = 20
}
```

- [ ] **Step 4: Run config test green**

Run:

```bash
go test ./tests/unit/config -run TestLoadDailyObsidianDefaults -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Add `report_exports` schema**

Add to `db/schema.sql` and `db/migrations/000001_create_all_tables.up.sql` after the `reports` table:

```sql
create table report_exports (
  id bigserial primary key,
  report_id bigint not null references reports(id),
  export_kind text not null check (export_kind in ('daily-digest', 'publish-draft')),
  target_path text not null,
  status text not null default 'pending' check (status in ('pending', 'published', 'skipped', 'failed')),
  error_message text not null default '',
  published_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (report_id, export_kind)
);

create index idx_report_exports_report_id on report_exports(report_id);
create index idx_report_exports_status on report_exports(status);
```

Add near the top of `db/migrations/000001_create_all_tables.down.sql`, before dropping `reports`:

```sql
DROP TABLE IF EXISTS report_exports;
```

Add `"report_exports"` before `"reports"` in `tests/testutil/db.go`.

- [ ] **Step 6: Add domain model and repository interface**

Create `internal/report/export_repository.go`:

```go
package report

import (
	"context"
	"time"
)

const (
	ExportKindDailyDigest  = "daily-digest"
	ExportKindPublishDraft = "publish-draft"

	ExportStatusPending   = "pending"
	ExportStatusPublished = "published"
	ExportStatusSkipped   = "skipped"
	ExportStatusFailed    = "failed"
)

type ReportExport struct {
	ID           int64      `json:"id"`
	ReportID     int64      `json:"report_id"`
	ExportKind   string     `json:"export_kind"`
	TargetPath   string     `json:"target_path"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message"`
	PublishedAt  *time.Time `json:"published_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type CreateReportExportInput struct {
	ReportID   int64
	ExportKind string
	TargetPath string
}

type ExportRepository interface {
	CreatePending(ctx context.Context, input CreateReportExportInput) (ReportExport, error)
	MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (ReportExport, error)
	MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (ReportExport, error)
	MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (ReportExport, error)
	ListByReport(ctx context.Context, reportID int64) ([]ReportExport, error)
}
```

- [ ] **Step 7: Add GORM model**

Add to `internal/repository/gormimpl/model.go`:

```go
type ReportExport struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	ReportID     int64      `gorm:"column:report_id"`
	ExportKind   string     `gorm:"column:export_kind"`
	TargetPath   string     `gorm:"column:target_path"`
	Status       string     `gorm:"column:status"`
	ErrorMessage string     `gorm:"column:error_message"`
	PublishedAt  *time.Time `gorm:"column:published_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (ReportExport) TableName() string { return "report_exports" }
```

- [ ] **Step 8: Add GORM repository**

Create `internal/repository/gormimpl/report_export.go`:

```go
package gormimpl

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/report"
	"gorm.io/gorm"
)

type ReportExportRepo struct {
	db *gorm.DB
}

func NewReportExportRepo(db *gorm.DB) *ReportExportRepo {
	return &ReportExportRepo{db: db}
}

func (r *ReportExportRepo) CreatePending(ctx context.Context, input report.CreateReportExportInput) (report.ReportExport, error) {
	model := ReportExport{
		ReportID:   input.ReportID,
		ExportKind: input.ExportKind,
		TargetPath: input.TargetPath,
		Status:     report.ExportStatusPending,
	}
	err := r.db.WithContext(ctx).Where(ReportExport{
		ReportID:   input.ReportID,
		ExportKind: input.ExportKind,
	}).Attrs(model).FirstOrCreate(&model).Error
	if err != nil {
		return report.ReportExport{}, err
	}
	return toReportExport(model), nil
}

func (r *ReportExportRepo) MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (report.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, report.ExportStatusPublished, "", &publishedAt, publishedAt)
}

func (r *ReportExportRepo) MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (report.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, report.ExportStatusSkipped, "", &skippedAt, skippedAt)
}

func (r *ReportExportRepo) MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (report.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, report.ExportStatusFailed, message, nil, failedAt)
}

func (r *ReportExportRepo) ListByReport(ctx context.Context, reportID int64) ([]report.ReportExport, error) {
	var models []ReportExport
	if err := r.db.WithContext(ctx).Where("report_id = ?", reportID).Order("export_kind ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]report.ReportExport, len(models))
	for i, model := range models {
		out[i] = toReportExport(model)
	}
	return out, nil
}

func (r *ReportExportRepo) updateStatus(ctx context.Context, reportID int64, exportKind string, path string, status string, message string, publishedAt *time.Time, updatedAt time.Time) (report.ReportExport, error) {
	model := ReportExport{}
	err := r.db.WithContext(ctx).Where(ReportExport{
		ReportID:   reportID,
		ExportKind: exportKind,
	}).Attrs(ReportExport{
		TargetPath: path,
	}).FirstOrCreate(&model).Error
	if err != nil {
		return report.ReportExport{}, err
	}

	updates := map[string]any{
		"target_path":    path,
		"status":         status,
		"error_message":  message,
		"published_at":   publishedAt,
		"updated_at":     updatedAt,
	}
	if err := r.db.WithContext(ctx).Model(&model).Updates(updates).Error; err != nil {
		return report.ReportExport{}, err
	}
	return r.ListOne(ctx, reportID, exportKind)
}

func (r *ReportExportRepo) ListOne(ctx context.Context, reportID int64, exportKind string) (report.ReportExport, error) {
	var model ReportExport
	if err := r.db.WithContext(ctx).Where("report_id = ? AND export_kind = ?", reportID, exportKind).First(&model).Error; err != nil {
		return report.ReportExport{}, err
	}
	return toReportExport(model), nil
}

func toReportExport(model ReportExport) report.ReportExport {
	return report.ReportExport{
		ID:           model.ID,
		ReportID:     model.ReportID,
		ExportKind:   model.ExportKind,
		TargetPath:   model.TargetPath,
		Status:       model.Status,
		ErrorMessage: model.ErrorMessage,
		PublishedAt:  model.PublishedAt,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

var _ report.ExportRepository = (*ReportExportRepo)(nil)
```

- [ ] **Step 9: Validate schema and architecture**

Run:

```bash
go test ./tests/unit/config -run TestLoadDailyObsidianDefaults -count=1 -v
bash scripts/validate-repository.sh
```

Expected: PASS. Schema validation should report one additional table compared to the previous baseline.

- [ ] **Step 10: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go tests/unit/config/config_test.go db/schema.sql db/migrations/000001_create_all_tables.up.sql db/migrations/000001_create_all_tables.down.sql tests/testutil/db.go internal/report/export_repository.go internal/repository/gormimpl/model.go internal/repository/gormimpl/report_export.go
git commit -m "feat: add report export persistence"
```

---

### Task 2: Obsidian Pathing and No-Overwrite Writer

**Files:**
- Create: `internal/obsidian/model.go`
- Create: `internal/obsidian/pathing.go`
- Create: `internal/obsidian/writer.go`
- Test: `tests/unit/obsidian/pathing_test.go`
- Test: `tests/unit/obsidian/writer_test.go`

**Interfaces:**
- Produces:
  - `obsidian.ExportKind`
  - `obsidian.ExportDailyDigest`
  - `obsidian.ExportPublishDraft`
  - `obsidian.PathInput`
  - `obsidian.BuildPath(root string, input PathInput) (string, error)`
  - `obsidian.WriteAtomicNoOverwrite(path string, content []byte) (WriteResult, error)`

- [ ] **Step 1: Write failing path tests**

Create `tests/unit/obsidian/pathing_test.go`:

```go
package obsidian_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

func TestBuildPathDailyDigest(t *testing.T) {
	got, err := obsidian.BuildPath("/vault", obsidian.PathInput{
		Kind:        obsidian.ExportDailyDigest,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		MonitorName: "AI Regulation",
	})
	if err != nil {
		t.Fatalf("BuildPath returned error: %v", err)
	}
	want := filepath.Join("/vault", "HotKey", "digests", "daily", "2026-07-08", "ai-regulation.md")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestBuildPathPublishDraft(t *testing.T) {
	got, err := obsidian.BuildPath("/vault", obsidian.PathInput{
		Kind:        obsidian.ExportPublishDraft,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		MonitorName: "AI Regulation",
	})
	if err != nil {
		t.Fatalf("BuildPath returned error: %v", err)
	}
	want := filepath.Join("/vault", "HotKey", "publish", "drafts", "2026-07-08", "ai-regulation.md")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestBuildPathRejectsMissingVault(t *testing.T) {
	_, err := obsidian.BuildPath("", obsidian.PathInput{
		Kind:        obsidian.ExportDailyDigest,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		MonitorName: "AI Regulation",
	})
	if err != obsidian.ErrMissingVaultRoot {
		t.Fatalf("error = %v, want %v", err, obsidian.ErrMissingVaultRoot)
	}
}
```

- [ ] **Step 2: Run path tests red**

Run:

```bash
go test ./tests/unit/obsidian -run TestBuildPath -count=1 -v
```

Expected: FAIL because `internal/obsidian` does not exist.

- [ ] **Step 3: Implement pathing**

Create `internal/obsidian/model.go`:

```go
package obsidian

import (
	"errors"
	"time"
)

type ExportKind string

const (
	ExportDailyDigest  ExportKind = "daily-digest"
	ExportPublishDraft ExportKind = "publish-draft"
)

const (
	WriteStatusPublished = "published"
	WriteStatusSkipped   = "skipped"
)

var (
	ErrMissingVaultRoot = errors.New("missing obsidian vault root")
	ErrInvalidExportKind = errors.New("invalid obsidian export kind")
)

type PathInput struct {
	Kind        ExportKind
	Date        time.Time
	MonitorName string
}

type WriteResult struct {
	Path    string
	Status  string
	Skipped bool
}
```

Create `internal/obsidian/pathing.go`:

```go
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
```

- [ ] **Step 4: Run path tests green**

Run:

```bash
go test ./tests/unit/obsidian -run TestBuildPath -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Write failing writer tests**

Create `tests/unit/obsidian/writer_test.go`:

```go
package obsidian_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

func TestWriteAtomicNoOverwriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "HotKey", "digests", "daily", "2026-07-08", "ai.md")

	result, err := obsidian.WriteAtomicNoOverwrite(path, []byte("# Daily"))
	if err != nil {
		t.Fatalf("WriteAtomicNoOverwrite returned error: %v", err)
	}
	if result.Status != obsidian.WriteStatusPublished || result.Skipped {
		t.Fatalf("result = %+v, want published not skipped", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != "# Daily" {
		t.Fatalf("file content = %q", string(got))
	}
}

func TestWriteAtomicNoOverwriteSkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("edited in obsidian"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := obsidian.WriteAtomicNoOverwrite(path, []byte("new generated content"))
	if err != nil {
		t.Fatalf("WriteAtomicNoOverwrite returned error: %v", err)
	}
	if result.Status != obsidian.WriteStatusSkipped || !result.Skipped {
		t.Fatalf("result = %+v, want skipped", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != "edited in obsidian" {
		t.Fatalf("existing file was overwritten: %q", string(got))
	}
}
```

- [ ] **Step 6: Run writer tests red**

Run:

```bash
go test ./tests/unit/obsidian -run TestWriteAtomicNoOverwrite -count=1 -v
```

Expected: FAIL with `undefined: obsidian.WriteAtomicNoOverwrite`.

- [ ] **Step 7: Implement writer**

Create `internal/obsidian/writer.go`:

```go
package obsidian

import (
	"errors"
	"os"
	"path/filepath"
)

func WriteAtomicNoOverwrite(path string, content []byte) (WriteResult, error) {
	if _, err := os.Stat(path); err == nil {
		return WriteResult{Path: path, Status: WriteStatusSkipped, Skipped: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return WriteResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WriteResult{}, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return WriteResult{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return WriteResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return WriteResult{}, err
	}

	if _, err := os.Stat(path); err == nil {
		return WriteResult{Path: path, Status: WriteStatusSkipped, Skipped: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return WriteResult{}, err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Path: path, Status: WriteStatusPublished}, nil
}
```

- [ ] **Step 8: Run obsidian path and writer tests green**

Run:

```bash
go test ./tests/unit/obsidian -count=1 -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/obsidian/model.go internal/obsidian/pathing.go internal/obsidian/writer.go tests/unit/obsidian/pathing_test.go tests/unit/obsidian/writer_test.go
git commit -m "feat: add obsidian pathing and no-overwrite writer"
```

---

### Task 3: Obsidian Markdown Rendering

**Files:**
- Create: `internal/obsidian/render.go`
- Test: `tests/unit/obsidian/render_test.go`

**Interfaces:**
- Consumes:
  - `obsidian.ExportKind`
- Produces:
  - `obsidian.MarkdownInput`
  - `obsidian.RenderMarkdown(input MarkdownInput) (string, error)`

- [ ] **Step 1: Write failing render tests**

Create `tests/unit/obsidian/render_test.go`:

```go
package obsidian_test

import (
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

func TestRenderMarkdownDailyDigest(t *testing.T) {
	got, err := obsidian.RenderMarkdown(obsidian.MarkdownInput{
		Kind:        obsidian.ExportDailyDigest,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		ReportID:    123,
		MonitorID:   10,
		MonitorName: "AI Regulation",
		Title:       "AI Regulation 日报 2026-07-08",
		Content:     "## 今日概览\n\n今日热点。",
	})
	if err != nil {
		t.Fatalf("RenderMarkdown returned error: %v", err)
	}
	for _, want := range []string{
		"type: hotkey-digest",
		"report_id: 123",
		"monitor_id: 10",
		"- daily",
		"# AI Regulation 日报 2026-07-08",
		"## 今日概览",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown missing %q:\n%s", want, got)
		}
	}
}

func TestRenderMarkdownPublishDraft(t *testing.T) {
	got, err := obsidian.RenderMarkdown(obsidian.MarkdownInput{
		Kind:        obsidian.ExportPublishDraft,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		ReportID:    123,
		MonitorID:   10,
		MonitorName: "AI Regulation",
		Title:       "今日 AI 行业热点",
		Content:     "## 导语\n\n这是一篇草稿。",
	})
	if err != nil {
		t.Fatalf("RenderMarkdown returned error: %v", err)
	}
	for _, want := range []string{
		"type: hotkey-publish-draft",
		"publish_status: draft",
		"- wechat",
		"- zhihu",
		"- website",
		"# 今日 AI 行业热点",
		"## 导语",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown missing %q:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 2: Run render tests red**

Run:

```bash
go test ./tests/unit/obsidian -run TestRenderMarkdown -count=1 -v
```

Expected: FAIL with `undefined: obsidian.RenderMarkdown`.

- [ ] **Step 3: Implement renderer**

Create `internal/obsidian/render.go`:

```go
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
	b.WriteString(fmt.Sprintf("monitor: %s\n", input.MonitorName))
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
```

- [ ] **Step 4: Run render tests green**

Run:

```bash
go test ./tests/unit/obsidian -run TestRenderMarkdown -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/obsidian/render.go tests/unit/obsidian/render_test.go
git commit -m "feat: render obsidian daily markdown exports"
```

---

### Task 4: Daily Scheduler and Target-Date Logic

**Files:**
- Create: `internal/worker/daily_scheduler.go`
- Test: `tests/unit/worker/daily_scheduler_test.go`

**Interfaces:**
- Produces:
  - `worker.DailyScheduleConfig`
  - `worker.ResolveTargetDate(now time.Time, cfg DailyScheduleConfig) (time.Time, error)`
  - `worker.RunKeyForDate(date time.Time) string`
  - `worker.ShouldRun(now time.Time, lastRunDate *time.Time, cfg DailyScheduleConfig) (bool, time.Time, error)`

- [ ] **Step 1: Write failing scheduler tests**

Create `tests/unit/worker/daily_scheduler_test.go`:

```go
package worker_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

func TestResolveTargetDateYesterday(t *testing.T) {
	now := time.Date(2026, 7, 8, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	got, err := worker.ResolveTargetDate(now, worker.DailyScheduleConfig{
		Timezone: "Asia/Shanghai",
		Target:   "yesterday",
	})
	if err != nil {
		t.Fatalf("ResolveTargetDate returned error: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-07" {
		t.Fatalf("target date = %s, want 2026-07-07", got.Format("2006-01-02"))
	}
}

func TestRunKeyForDate(t *testing.T) {
	date := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	got := worker.RunKeyForDate(date)
	want := "daily-obsidian-publish:2026-07-07"
	if got != want {
		t.Fatalf("run key = %q, want %q", got, want)
	}
}

func TestShouldRunAfterConfiguredTime(t *testing.T) {
	now := time.Date(2026, 7, 8, 8, 1, 0, 0, time.FixedZone("CST", 8*60*60))
	should, target, err := worker.ShouldRun(now, nil, worker.DailyScheduleConfig{
		Time:     "08:00",
		Timezone: "Asia/Shanghai",
		Target:   "yesterday",
	})
	if err != nil {
		t.Fatalf("ShouldRun returned error: %v", err)
	}
	if !should {
		t.Fatal("ShouldRun = false, want true")
	}
	if target.Format("2006-01-02") != "2026-07-07" {
		t.Fatalf("target = %s, want 2026-07-07", target.Format("2006-01-02"))
	}
}

func TestShouldRunBeforeConfiguredTime(t *testing.T) {
	now := time.Date(2026, 7, 8, 7, 59, 0, 0, time.FixedZone("CST", 8*60*60))
	should, _, err := worker.ShouldRun(now, nil, worker.DailyScheduleConfig{
		Time:     "08:00",
		Timezone: "Asia/Shanghai",
		Target:   "yesterday",
	})
	if err != nil {
		t.Fatalf("ShouldRun returned error: %v", err)
	}
	if should {
		t.Fatal("ShouldRun = true before configured time")
	}
}
```

- [ ] **Step 2: Run scheduler tests red**

Run:

```bash
go test ./tests/unit/worker -run 'TestResolveTargetDate|TestRunKey|TestShouldRun' -count=1 -v
```

Expected: FAIL because `internal/worker` does not exist.

- [ ] **Step 3: Implement scheduler logic**

Create `internal/worker/daily_scheduler.go`:

```go
package worker

import (
	"fmt"
	"time"
)

type DailyScheduleConfig struct {
	Time     string
	Timezone string
	Target   string
}

func ResolveTargetDate(now time.Time, cfg DailyScheduleConfig) (time.Time, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(loc)
	switch cfg.Target {
	case "", "yesterday":
		local = local.AddDate(0, 0, -1)
	case "today":
	default:
		return time.Time{}, fmt.Errorf("invalid daily digest target: %s", cfg.Target)
	}
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc), nil
}

func RunKeyForDate(date time.Time) string {
	return "daily-obsidian-publish:" + date.Format("2006-01-02")
}

func ShouldRun(now time.Time, lastRunDate *time.Time, cfg DailyScheduleConfig) (bool, time.Time, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return false, time.Time{}, err
	}
	hour, minute, err := parseClock(cfg.Time)
	if err != nil {
		return false, time.Time{}, err
	}
	local := now.In(loc)
	due := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	target, err := ResolveTargetDate(now, cfg)
	if err != nil {
		return false, time.Time{}, err
	}
	if local.Before(due) {
		return false, target, nil
	}
	if lastRunDate != nil && lastRunDate.Format("2006-01-02") == target.Format("2006-01-02") {
		return false, target, nil
	}
	return true, target, nil
}

func parseClock(value string) (int, int, error) {
	if value == "" {
		value = "08:00"
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, err
	}
	return parsed.Hour(), parsed.Minute(), nil
}
```

- [ ] **Step 4: Run scheduler tests green**

Run:

```bash
go test ./tests/unit/worker -run 'TestResolveTargetDate|TestRunKey|TestShouldRun' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/daily_scheduler.go tests/unit/worker/daily_scheduler_test.go
git commit -m "feat: add daily obsidian scheduler"
```

---

### Task 5: Daily Obsidian Publish Worker

**Files:**
- Create: `internal/worker/daily_obsidian_publish.go`
- Test: `tests/unit/worker/daily_obsidian_publish_test.go`

**Interfaces:**
- Consumes:
  - `report.Service.Create(ctx, userID, report.CreateInput)`
  - `report.ExportRepository`
  - `obsidian.BuildPath`
  - `obsidian.RenderMarkdown`
  - `obsidian.WriteAtomicNoOverwrite`
- Produces:
  - `worker.DailyObsidianPublishJob`
  - `worker.NewDailyObsidianPublishJob(...) *DailyObsidianPublishJob`
  - `(*DailyObsidianPublishJob).RunOnce(ctx context.Context, targetDate time.Time) error`

- [ ] **Step 1: Write failing worker test for two exports**

Create `tests/unit/worker/daily_obsidian_publish_test.go`:

```go
package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

type fakeMonitorLister struct {
	monitors []monitor.Monitor
}

func (f *fakeMonitorLister) ListActive(ctx context.Context) ([]monitor.Monitor, error) {
	return f.monitors, nil
}

type fakeDailyReportService struct {
	reports []report.Report
}

func (f *fakeDailyReportService) Create(ctx context.Context, userID int64, input report.CreateInput) (report.Report, error) {
	item := report.Report{
		ID:          int64(len(f.reports) + 1),
		UserID:      userID,
		ReportType:  report.TypeDaily,
		PeriodStart: *input.PeriodStart,
		PeriodEnd:   *input.PeriodEnd,
		Subject:     "AI Regulation 日报 2026-07-07",
		Summary:     "今日热点",
		Content:     "## 今日概览\n\n今日热点。\n\n## 热点主题\n\n- AI Regulation",
		Status:      report.StatusDraft,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	f.reports = append(f.reports, item)
	return item, nil
}

type fakeExportRepo struct {
	exports []report.ReportExport
}

func (f *fakeExportRepo) CreatePending(ctx context.Context, input report.CreateReportExportInput) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: input.ReportID, ExportKind: input.ExportKind, TargetPath: input.TargetPath, Status: report.ExportStatusPending}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: report.ExportStatusPublished, PublishedAt: &publishedAt}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: report.ExportStatusSkipped, PublishedAt: &skippedAt}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: report.ExportStatusFailed, ErrorMessage: message}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) ListByReport(ctx context.Context, reportID int64) ([]report.ReportExport, error) {
	return f.exports, nil
}

func TestDailyObsidianPublishJobWritesDigestAndDraft(t *testing.T) {
	vault := t.TempDir()
	exports := &fakeExportRepo{}
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: vault,
		Monitors: &fakeMonitorLister{monitors: []monitor.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
		Reports:  &fakeDailyReportService{},
		Exports:  exports,
		Now:      func() time.Time { return time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC) },
	})

	targetDate := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	digestPath := filepath.Join(vault, "HotKey", "digests", "daily", "2026-07-07", "ai-regulation.md")
	draftPath := filepath.Join(vault, "HotKey", "publish", "drafts", "2026-07-07", "ai-regulation.md")
	for _, path := range []string{digestPath, draftPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %s: %v", path, err)
		}
	}
	if len(exports.exports) < 4 {
		t.Fatalf("expected pending and final export records, got %d", len(exports.exports))
	}
}
```

- [ ] **Step 2: Run worker test red**

Run:

```bash
go test ./tests/unit/worker -run TestDailyObsidianPublishJobWritesDigestAndDraft -count=1 -v
```

Expected: FAIL with `undefined: worker.NewDailyObsidianPublishJob`.

- [ ] **Step 3: Implement worker job**

Create `internal/worker/daily_obsidian_publish.go`:

```go
package worker

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
	"github.com/StephenQiu30/hotkey-server/internal/report"
)

type MonitorLister interface {
	ListActive(ctx context.Context) ([]monitor.Monitor, error)
}

type DailyReportService interface {
	Create(ctx context.Context, userID int64, input report.CreateInput) (report.Report, error)
}

type DailyObsidianPublishDeps struct {
	VaultRoot string
	Monitors  MonitorLister
	Reports   DailyReportService
	Exports   report.ExportRepository
	Now       func() time.Time
}

type DailyObsidianPublishJob struct {
	deps DailyObsidianPublishDeps
}

func NewDailyObsidianPublishJob(deps DailyObsidianPublishDeps) *DailyObsidianPublishJob {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &DailyObsidianPublishJob{deps: deps}
}

func (j *DailyObsidianPublishJob) RunOnce(ctx context.Context, targetDate time.Time) error {
	if j.deps.VaultRoot == "" {
		return obsidian.ErrMissingVaultRoot
	}
	monitors, err := j.deps.Monitors.ListActive(ctx)
	if err != nil {
		return err
	}
	var runErr error
	for _, m := range monitors {
		periodStart := targetDate
		periodEnd := targetDate
		item, err := j.deps.Reports.Create(ctx, m.UserID, report.CreateInput{
			ReportType:  report.TypeDaily,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
		})
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		runErr = errors.Join(runErr, j.exportOne(ctx, item, m, obsidian.ExportDailyDigest, targetDate))
		runErr = errors.Join(runErr, j.exportOne(ctx, item, m, obsidian.ExportPublishDraft, targetDate))
	}
	return runErr
}

func (j *DailyObsidianPublishJob) exportOne(ctx context.Context, item report.Report, m monitor.Monitor, kind obsidian.ExportKind, targetDate time.Time) error {
	path, err := obsidian.BuildPath(j.deps.VaultRoot, obsidian.PathInput{
		Kind:        kind,
		Date:        targetDate,
		MonitorName: m.Name,
	})
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), "", err.Error(), j.deps.Now())
		return err
	}
	_, _ = j.deps.Exports.CreatePending(ctx, report.CreateReportExportInput{
		ReportID:   item.ID,
		ExportKind: string(kind),
		TargetPath: path,
	})
	markdown, err := obsidian.RenderMarkdown(obsidian.MarkdownInput{
		Kind:        kind,
		Date:        targetDate,
		ReportID:    item.ID,
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Title:       item.Subject,
		Content:     item.Content,
	})
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), path, err.Error(), j.deps.Now())
		return err
	}
	result, err := obsidian.WriteAtomicNoOverwrite(path, []byte(markdown))
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), path, err.Error(), j.deps.Now())
		return err
	}
	if result.Skipped {
		_, err = j.deps.Exports.MarkSkipped(ctx, item.ID, string(kind), path, j.deps.Now())
		return err
	}
	_, err = j.deps.Exports.MarkPublished(ctx, item.ID, string(kind), path, j.deps.Now())
	return err
}
```

- [ ] **Step 4: Run worker test green**

Run:

```bash
go test ./tests/unit/worker -run TestDailyObsidianPublishJobWritesDigestAndDraft -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Add skipped-existing-file worker test**

Append to `tests/unit/worker/daily_obsidian_publish_test.go`:

```go
func TestDailyObsidianPublishJobSkipsExistingFiles(t *testing.T) {
	vault := t.TempDir()
	digestPath := filepath.Join(vault, "HotKey", "digests", "daily", "2026-07-07", "ai-regulation.md")
	if err := os.MkdirAll(filepath.Dir(digestPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(digestPath, []byte("edited draft"), 0o644); err != nil {
		t.Fatalf("seed digest: %v", err)
	}

	exports := &fakeExportRepo{}
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: vault,
		Monitors: &fakeMonitorLister{monitors: []monitor.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
		Reports:  &fakeDailyReportService{},
		Exports:  exports,
		Now:      func() time.Time { return time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC) },
	})
	targetDate := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	got, err := os.ReadFile(digestPath)
	if err != nil {
		t.Fatalf("read digest: %v", err)
	}
	if string(got) != "edited draft" {
		t.Fatalf("existing digest overwritten: %q", string(got))
	}
	foundSkipped := false
	for _, item := range exports.exports {
		if item.ExportKind == report.ExportKindDailyDigest && item.Status == report.ExportStatusSkipped {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Fatalf("expected skipped export record, got %+v", exports.exports)
	}
}
```

- [ ] **Step 6: Run full worker tests**

Run:

```bash
go test ./tests/unit/worker -count=1 -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/daily_obsidian_publish.go tests/unit/worker/daily_obsidian_publish_test.go
git commit -m "feat: add daily obsidian publish worker"
```

---

### Task 6: Active Monitor Query for Worker

**Files:**
- Modify: `internal/monitor/repository.go`
- Modify: `internal/monitor/service.go`
- Modify: `internal/repository/gormimpl/monitor_repo.go`
- Test: `tests/unit/monitor/service_test.go`

**Interfaces:**
- Consumes: `monitor.Monitor`
- Produces:
  - `monitor.Repository.ListActive(ctx context.Context) ([]Monitor, error)`
  - `(*monitor.Service).ListActive(ctx context.Context) ([]Monitor, error)`

- [ ] **Step 1: Write failing monitor service test**

Add to `tests/unit/monitor/service_test.go`:

```go
func TestServiceListActive(t *testing.T) {
	repo := &fakeMonitorRepo{
		active: []monitor.Monitor{{ID: 1, UserID: 7, Name: "AI", Status: "active"}},
	}
	svc := monitor.NewService(repo)

	got, err := svc.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive returned error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "AI" {
		t.Fatalf("ListActive = %+v", got)
	}
}
```

Extend the local fake repository in that test file:

```go
type fakeMonitorRepo struct {
	active []monitor.Monitor
}

func (r *fakeMonitorRepo) ListActive(ctx context.Context) ([]monitor.Monitor, error) {
	return r.active, nil
}
```

- [ ] **Step 2: Run monitor test red**

Run:

```bash
go test ./tests/unit/monitor -run TestServiceListActive -count=1 -v
```

Expected: FAIL because `ListActive` is not on the repository or service interface.

- [ ] **Step 3: Add interface and service method**

Modify `internal/monitor/repository.go`:

```go
ListActive(ctx context.Context) ([]Monitor, error)
```

Modify `internal/monitor/service.go`:

```go
func (s *Service) ListActive(ctx context.Context) ([]Monitor, error) {
	return s.repo.ListActive(ctx)
}
```

- [ ] **Step 4: Implement GORM query**

Add to `internal/repository/gormimpl/monitor_repo.go`:

```go
func (r *MonitorRepo) ListActive(ctx context.Context) ([]monitor.Monitor, error) {
	var models []KeywordMonitor
	if err := r.db.WithContext(ctx).Where("status = ?", "active").Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]monitor.Monitor, len(models))
	for i, model := range models {
		out[i] = monitor.Monitor{
			ID:                  model.ID,
			UserID:              model.UserID,
			Name:                model.Name,
			QueryText:           model.QueryText,
			Language:            model.Language,
			Region:              model.Region,
			Status:              model.Status,
			PollIntervalMinutes: model.PollIntervalMinutes,
			AlertEnabled:        model.AlertEnabled,
			LastPolledAt:        model.LastPolledAt,
			CreatedAt:           model.CreatedAt,
			UpdatedAt:           model.UpdatedAt,
		}
	}
	return out, nil
}
```

- [ ] **Step 5: Run monitor tests green**

Run:

```bash
go test ./tests/unit/monitor -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Update router test fakes**

Any fake implementing `monitor.Repository` must add:

```go
func (r *stubMonitorRepo) ListActive(_ context.Context) ([]monitor.Monitor, error) {
	return []monitor.Monitor{{ID: 1, UserID: 1, Name: "test", Status: "active"}}, nil
}
```

Run:

```bash
go test ./tests/unit/platform/http -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/monitor/repository.go internal/monitor/service.go internal/repository/gormimpl/monitor_repo.go tests/unit/monitor/service_test.go tests/unit/platform/http/router_test.go
git commit -m "feat: list active monitors for daily workers"
```

---

### Task 7: Wire Worker into Fx Lifecycle

**Files:**
- Modify: `internal/fxapp/app.go`
- Test: `tests/unit/platform/http/router_test.go`
- Test: no direct worker lifecycle test required; validation covers DI compile and startup wiring.

**Interfaces:**
- Consumes:
  - `*config.Config`
  - `*monitor.Service`
  - `*report.Service`
  - `report.ExportRepository`
  - `worker.NewDailyObsidianPublishJob`
- Produces:
  - Fx-provided `*worker.DailyObsidianPublishJob`
  - lifecycle goroutine that ticks every minute outside smoke tests

- [ ] **Step 1: Add provider function**

Modify `internal/fxapp/app.go`:

```go
func newDailyObsidianPublishJob(cfg *config.Config, monitorSvc *monitor.Service, reportSvc *report.Service, exportRepo report.ExportRepository) *worker.DailyObsidianPublishJob {
	return worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: cfg.ObsidianVaultPath,
		Monitors:  monitorSvc,
		Reports:   reportSvc,
		Exports:   exportRepo,
		Now:       time.Now,
	})
}
```

Add imports:

```go
import "github.com/StephenQiu30/hotkey-server/internal/worker"
```

Add to `fx.New`:

```go
fx.Provide(fx.Annotate(gormimpl.NewReportExportRepo, fx.As(new(report.ExportRepository)))),
fx.Provide(newDailyObsidianPublishJob),
```

- [ ] **Step 2: Extend lifecycle registration**

Change `registerHooks` signature:

```go
func registerHooks(lc fx.Lifecycle, srv *http.Server, db *gorm.DB, cfg *config.Config, dailyJob *worker.DailyObsidianPublishJob) {
```

Inside non-smoke `OnStart`, add:

```go
go func() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			shouldRun, targetDate, err := worker.ShouldRun(now, nil, worker.DailyScheduleConfig{
				Time:     cfg.DailyDigestTime,
				Timezone: cfg.DailyDigestTimezone,
				Target:   cfg.DailyDigestTarget,
			})
			if err != nil {
				log.Printf("daily obsidian scheduler error: %v", err)
				continue
			}
			if !shouldRun {
				continue
			}
			if err := dailyJob.RunOnce(context.Background(), targetDate); err != nil {
				log.Printf("daily obsidian publish failed: %v", err)
			}
		}
	}
}()
```

This is intentionally minimal. Task 8 will replace `nil` last-run tracking with `knowledge_runs` idempotency before production use.

- [ ] **Step 3: Run compile validation**

Run:

```bash
go test ./internal/fxapp ./tests/unit/platform/http -count=1
```

Expected: PASS.

- [ ] **Step 4: Run build**

Run:

```bash
go build ./cmd/hotkey
```

Expected: PASS. Remove the generated `hotkey` binary if it appears in the repo root:

```bash
rm -f hotkey
```

- [ ] **Step 5: Commit**

```bash
git add internal/fxapp/app.go
git commit -m "feat: wire daily obsidian worker"
```

---

### Task 8: `knowledge_runs` Idempotency

**Files:**
- Create: `internal/worker/run_repository.go`
- Create: `internal/repository/gormimpl/knowledge_run.go`
- Modify: `internal/worker/daily_obsidian_publish.go`
- Modify: `internal/fxapp/app.go`
- Test: `tests/unit/worker/daily_obsidian_publish_test.go`

**Interfaces:**
- Produces:
  - `worker.RunRepository`
  - `gormimpl.NewKnowledgeRunRepo(db *gorm.DB) *KnowledgeRunRepo`
  - `DailyObsidianPublishDeps.Runs worker.RunRepository`

- [ ] **Step 1: Define run repository interface**

Create `internal/worker/run_repository.go`:

```go
package worker

import (
	"context"
	"time"
)

type RunRepository interface {
	TryStart(ctx context.Context, runKey string, runType string, targetDate time.Time, startedAt time.Time) (bool, error)
	MarkFinished(ctx context.Context, runKey string, finishedAt time.Time) error
	MarkFailed(ctx context.Context, runKey string, message string, failedAt time.Time) error
}
```

- [ ] **Step 2: Write failing duplicate-run test**

Append to `tests/unit/worker/daily_obsidian_publish_test.go`:

```go
type fakeRunRepo struct {
	started map[string]bool
}

func (f *fakeRunRepo) TryStart(ctx context.Context, runKey string, runType string, targetDate time.Time, startedAt time.Time) (bool, error) {
	if f.started == nil {
		f.started = map[string]bool{}
	}
	if f.started[runKey] {
		return false, nil
	}
	f.started[runKey] = true
	return true, nil
}
func (f *fakeRunRepo) MarkFinished(ctx context.Context, runKey string, finishedAt time.Time) error { return nil }
func (f *fakeRunRepo) MarkFailed(ctx context.Context, runKey string, message string, failedAt time.Time) error { return nil }

func TestDailyObsidianPublishJobSkipsDuplicateRunKey(t *testing.T) {
	vault := t.TempDir()
	reportSvc := &fakeDailyReportService{}
	runs := &fakeRunRepo{}
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: vault,
		Monitors: &fakeMonitorLister{monitors: []monitor.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
		Reports:  reportSvc,
		Exports:  &fakeExportRepo{},
		Runs:     runs,
		Now:      func() time.Time { return time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC) },
	})
	targetDate := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("first RunOnce returned error: %v", err)
	}
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("second RunOnce returned error: %v", err)
	}
	if len(reportSvc.reports) != 1 {
		t.Fatalf("reports generated = %d, want 1", len(reportSvc.reports))
	}
}
```

- [ ] **Step 3: Run duplicate test red**

Run:

```bash
go test ./tests/unit/worker -run TestDailyObsidianPublishJobSkipsDuplicateRunKey -count=1 -v
```

Expected: FAIL because `DailyObsidianPublishDeps` has no `Runs` field or duplicate handling.

- [ ] **Step 4: Add idempotency to job**

Modify `internal/worker/daily_obsidian_publish.go`:

```go
type DailyObsidianPublishDeps struct {
	VaultRoot string
	Monitors  MonitorLister
	Reports   DailyReportService
	Exports   report.ExportRepository
	Runs      RunRepository
	Now       func() time.Time
}
```

At the start of `RunOnce` after the vault check:

```go
runKey := RunKeyForDate(targetDate)
if j.deps.Runs != nil {
	started, err := j.deps.Runs.TryStart(ctx, runKey, "daily-obsidian-publish", targetDate, j.deps.Now())
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	defer func() {
		if runErr != nil {
			_ = j.deps.Runs.MarkFailed(context.Background(), runKey, runErr.Error(), j.deps.Now())
			return
		}
		_ = j.deps.Runs.MarkFinished(context.Background(), runKey, j.deps.Now())
	}()
}
```

Change `RunOnce` to use a named return:

```go
func (j *DailyObsidianPublishJob) RunOnce(ctx context.Context, targetDate time.Time) (runErr error) {
```

- [ ] **Step 5: Implement GORM knowledge run repository**

Create `internal/repository/gormimpl/knowledge_run.go`:

```go
package gormimpl

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type KnowledgeRunRepo struct {
	db *gorm.DB
}

func NewKnowledgeRunRepo(db *gorm.DB) *KnowledgeRunRepo {
	return &KnowledgeRunRepo{db: db}
}

func (r *KnowledgeRunRepo) TryStart(ctx context.Context, runKey string, runType string, targetDate time.Time, startedAt time.Time) (bool, error) {
	model := KnowledgeRun{
		RunKey:     runKey,
		RunType:    runType,
		TargetDate: &targetDate,
		Status:     "running",
		StartedAt:  &startedAt,
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *KnowledgeRunRepo) MarkFinished(ctx context.Context, runKey string, finishedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&KnowledgeRun{}).Where("run_key = ?", runKey).Updates(map[string]any{
		"status":      "finished",
		"finished_at": finishedAt,
	}).Error
}

func (r *KnowledgeRunRepo) MarkFailed(ctx context.Context, runKey string, message string, failedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&KnowledgeRun{}).Where("run_key = ?", runKey).Updates(map[string]any{
		"status":        "failed",
		"error_message": message,
		"finished_at":   failedAt,
	}).Error
}
```

- [ ] **Step 6: Wire run repo into Fx**

Modify `internal/fxapp/app.go`:

```go
fx.Provide(fx.Annotate(gormimpl.NewKnowledgeRunRepo, fx.As(new(worker.RunRepository)))),
```

Update `newDailyObsidianPublishJob` signature:

```go
func newDailyObsidianPublishJob(cfg *config.Config, monitorSvc *monitor.Service, reportSvc *report.Service, exportRepo report.ExportRepository, runRepo worker.RunRepository) *worker.DailyObsidianPublishJob
```

Set `Runs: runRepo`.

- [ ] **Step 7: Run worker tests green**

Run:

```bash
go test ./tests/unit/worker -count=1 -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/worker/run_repository.go internal/worker/daily_obsidian_publish.go internal/repository/gormimpl/knowledge_run.go internal/fxapp/app.go tests/unit/worker/daily_obsidian_publish_test.go
git commit -m "feat: make daily obsidian worker idempotent"
```

---

### Task 9: Final Backend Verification

**Files:**
- Modify only if validation exposes issues.

**Interfaces:**
- Consumes all previous tasks.
- Produces a validated backend implementation.

- [ ] **Step 1: Run all tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run repository validation**

Run:

```bash
bash scripts/validate-repository.sh
```

Expected: PASS.

- [ ] **Step 3: Run vet**

Run:

```bash
go vet ./...
```

Expected: PASS.

- [ ] **Step 4: Run build**

Run:

```bash
go build ./cmd/hotkey
```

Expected: PASS.

If a `hotkey` binary appears in the repo root:

```bash
rm -f hotkey
```

- [ ] **Step 5: Review git diff**

Run:

```bash
git status --short
git diff --stat
git diff --check
```

Expected:

- only backend files and tests changed
- no whitespace errors
- no hotkey-web or hotkey-miniapp changes

- [ ] **Step 6: Commit final validation fixes only if needed**

If Step 1-5 required fixes, stage only the files changed during Task 9. For example, if the fix was in the worker and its test:

```bash
git add internal/worker/daily_obsidian_publish.go tests/unit/worker/daily_obsidian_publish_test.go
git commit -m "test: validate daily obsidian publish flow"
```

If no fixes were needed, run:

```bash
git status --short
```

Expected: no new uncommitted files from Task 9. Do not create an empty commit.
