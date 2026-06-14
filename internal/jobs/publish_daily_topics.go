// Package jobs implements background job orchestration for the hotkey-server.
// PublishDailyTopicsJob orchestrates daily topic digest → LLM → obsidian export.
package jobs

import (
	"context"
	"fmt"
	"log"
	"time"
)

// TopicCandidate represents a topic eligible for daily export.
type TopicCandidate struct {
	TopicID        int64
	TopicKey       string
	Title          string
	HeatScore      float64
	TrendDirection string
	PostCount      int
	Posts          []RepresentativePost
}

// RepresentativePost holds a top post for a topic.
type RepresentativePost struct {
	AuthorName string
	Text       string
	URL        string
}

// DigestService selects eligible topics for a given day.
type DigestService interface {
	ListTopicsForDay(ctx context.Context, monitorID int64, exportDate time.Time, topN int) ([]TopicCandidate, error)
}

// LLMClient generates summaries for topics.
type LLMClient interface {
	SummarizeTopic(ctx context.Context, in TopicSummaryInput) (string, error)
}

// TopicSummaryInput is the input for LLM summarization.
type TopicSummaryInput struct {
	MonitorName string
	TopicTitle  string
	TopicKey    string
	HeatScore   float64
	Trend       string
	PostCount   int
	Posts       []RepresentativePost
}

// ObsidianWriter renders and writes a topic note to the vault.
type ObsidianWriter interface {
	WriteTopicNote(ctx context.Context, in ObsidianNoteInput) (string, error)
}

// ObsidianNoteInput is the input for writing an Obsidian note.
type ObsidianNoteInput struct {
	VaultPath    string
	MonitorID    int64
	MonitorName  string
	MonitorSlug  string
	TopicID      int64
	TopicKey     string
	Title        string
	Date         string // "2006-01-02"
	HeatScore    float64
	Trend        string
	PostCount    int
	Summary      string
	Posts        []RepresentativePost
}

// ExportRecord represents a row in topic_daily_exports.
type ExportRecord struct {
	ID           int64
	MonitorID    int64
	TopicID      int64
	ExportDate   string
	SummaryText  string
	MarkdownPath string
	Status       string
	ErrorMessage string
}

// ExportRepository manages topic_daily_exports persistence.
type ExportRepository interface {
	// UpsertExport inserts or updates an export record, returning the ID.
	UpsertExport(ctx context.Context, rec ExportRecord) (int64, error)
	// GetExportDate returns the last run date string, or "" if never run.
	GetLastRunDate(ctx context.Context) (string, error)
	// SetLastRunDate persists the last run date.
	SetLastRunDate(ctx context.Context, date string) error
}

// PublishDailyTopicsConfig holds configuration for the publish job.
type PublishDailyTopicsConfig struct {
	VaultPath string // OBSIDIAN_VAULT_PATH
	Target    string // "yesterday" or "today"
	TopN      int    // max topics per monitor
}

// PublishDailyTopicsJob orchestrates the daily topic publishing pipeline:
//
//	monitors → digest → LLM → export → obsidian write → status update
type PublishDailyTopicsJob struct {
	monitors   MonitorLister
	digest     DigestService
	llm        LLMClient
	writer     ObsidianWriter
	exports    ExportRepository
	scheduler  *DailyScheduler
	cfg        PublishDailyTopicsConfig
}

// NewPublishDailyTopicsJob creates a PublishDailyTopicsJob.
func NewPublishDailyTopicsJob(
	monitors MonitorLister,
	digest DigestService,
	llm LLMClient,
	writer ObsidianWriter,
	exports ExportRepository,
	scheduler *DailyScheduler,
	cfg PublishDailyTopicsConfig,
) *PublishDailyTopicsJob {
	if cfg.TopN <= 0 {
		cfg.TopN = 20
	}
	if cfg.Target == "" {
		cfg.Target = "yesterday"
	}
	return &PublishDailyTopicsJob{
		monitors:  monitors,
		digest:    digest,
		llm:       llm,
		writer:    writer,
		exports:   exports,
		scheduler: scheduler,
		cfg:       cfg,
	}
}

// Run executes the daily publish pipeline. It returns nil without action
// if the scheduler gate determines it is not time to run.
func (j *PublishDailyTopicsJob) Run(ctx context.Context, now time.Time) error {
	lastRunDate, err := j.exports.GetLastRunDate(ctx)
	if err != nil {
		return fmt.Errorf("get last run date: %w", err)
	}

	if !j.scheduler.ShouldRun(now, lastRunDate) {
		return nil
	}

	log.Printf("publish_daily_topics: starting for %s", j.scheduler.MarkRun(now))

	exportDate := ResolveExportDate(now, j.cfg.Target)
	monitorIDs, err := j.monitors.ListActiveIDs(ctx)
	if err != nil {
		return fmt.Errorf("list monitors: %w", err)
	}

	var lastErr error
	for _, monitorID := range monitorIDs {
		if err := j.publishForMonitor(ctx, monitorID, exportDate); err != nil {
			log.Printf("publish_daily_topics: monitor %d error: %v", monitorID, err)
			lastErr = err
			// Continue with other monitors
		}
	}

	runDate := j.scheduler.MarkRun(now)
	if err := j.exports.SetLastRunDate(ctx, runDate); err != nil {
		return fmt.Errorf("set last run date: %w", err)
	}

	log.Printf("publish_daily_topics: completed for %s", runDate)
	return lastErr
}

func (j *PublishDailyTopicsJob) publishForMonitor(ctx context.Context, monitorID int64, exportDate time.Time) error {
	topics, err := j.digest.ListTopicsForDay(ctx, monitorID, exportDate, j.cfg.TopN)
	if err != nil {
		return fmt.Errorf("list topics: %w", err)
	}

	for _, tc := range topics {
		if err := j.publishTopic(ctx, monitorID, exportDate, tc); err != nil {
			log.Printf("publish_daily_topics: topic %d error: %v", tc.TopicID, err)
			// Record failure but continue
			_, _ = j.exports.UpsertExport(ctx, ExportRecord{
				MonitorID:  monitorID,
				TopicID:    tc.TopicID,
				ExportDate: exportDate.Format("2006-01-02"),
				Status:     "failed",
				ErrorMessage: err.Error(),
			})
		}
	}

	return nil
}

func (j *PublishDailyTopicsJob) publishTopic(ctx context.Context, monitorID int64, exportDate time.Time, tc TopicCandidate) error {
	// LLM summary
	summary, err := j.llm.SummarizeTopic(ctx, TopicSummaryInput{
		TopicTitle:  tc.Title,
		TopicKey:    tc.TopicKey,
		HeatScore:   tc.HeatScore,
		Trend:       tc.TrendDirection,
		PostCount:   tc.PostCount,
		Posts:       tc.Posts,
	})
	if err != nil {
		return fmt.Errorf("llm summarize: %w", err)
	}

	// Upsert export record (pending)
	exportRec := ExportRecord{
		MonitorID:   monitorID,
		TopicID:     tc.TopicID,
		ExportDate:  exportDate.Format("2006-01-02"),
		SummaryText: summary,
		Status:      "pending",
	}
	exportID, err := j.exports.UpsertExport(ctx, exportRec)
	if err != nil {
		return fmt.Errorf("upsert export: %w", err)
	}

	// Write to Obsidian vault
	noteInput := ObsidianNoteInput{
		VaultPath:   j.cfg.VaultPath,
		MonitorID:   monitorID,
		TopicID:     tc.TopicID,
		TopicKey:    tc.TopicKey,
		Title:       tc.Title,
		Date:        exportDate.Format("2006-01-02"),
		HeatScore:   tc.HeatScore,
		Trend:       tc.TrendDirection,
		PostCount:   tc.PostCount,
		Summary:     summary,
		Posts:       tc.Posts,
	}
	mdPath, err := j.writer.WriteTopicNote(ctx, noteInput)
	if err != nil {
		return fmt.Errorf("write note: %w", err)
	}

	// Update export to published
	exportRec.ID = exportID
	exportRec.MarkdownPath = mdPath
	exportRec.Status = "published"
	if _, err := j.exports.UpsertExport(ctx, exportRec); err != nil {
		return fmt.Errorf("update export status: %w", err)
	}

	return nil
}

// ResolveExportDate computes the export date based on the target setting.
// "yesterday" returns the previous day in CST; "today" returns the current day.
func ResolveExportDate(now time.Time, target string) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	local := now.In(loc)
	year, month, day := local.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, loc)

	if target == "yesterday" {
		return today.AddDate(0, 0, -1)
	}
	return today
}
