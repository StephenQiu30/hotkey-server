package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"github.com/StephenQiu30/hotkey-server/internal/llm"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

// newKnowledgeSnapshotDelegate creates a PublishKnowledgeSnapshotJob from
// the same dependencies used by the legacy daily topics job.
func newKnowledgeSnapshotDelegate(
	digestSvc *digest.Service,
	llmClient llm.Client,
	exporter TopicExporter,
	vaultRoot string,
) *PublishKnowledgeSnapshotJob {
	return NewPublishKnowledgeSnapshotJob(
		&digestAdapter{digestSvc: digestSvc},
		&llmEventAdapter{llmClient: llmClient, exporter: exporter},
		&vaultExportAdapter{exporter: exporter, writer: &DefaultVaultWriter{}, vaultRoot: vaultRoot},
	)
}

// digestAdapter wraps digest.Service as a digestBuilder.
type digestAdapter struct {
	digestSvc *digest.Service
}

func (a *digestAdapter) BuildDigest(ctx context.Context, now time.Time) (DigestResult, error) {
	return DigestResult{}, nil
}

// llmEventAdapter is a stub event assembler for future implementation.
type llmEventAdapter struct {
	llmClient llm.Client
	exporter  TopicExporter
}

func (a *llmEventAdapter) BuildEvents(ctx context.Context, topics []TopicDigest) ([]EventSnapshot, error) {
	return nil, nil
}

// vaultExportAdapter is a stub knowledge exporter for future implementation.
type vaultExportAdapter struct {
	exporter  TopicExporter
	writer    VaultWriter
	vaultRoot string
}

func (a *vaultExportAdapter) Publish(ctx context.Context, digest DigestResult, events []EventSnapshot) (KnowledgeRunResult, error) {
	return KnowledgeRunResult{}, nil
}

// TopicExporter tracks per-topic export status for idempotency.
type TopicExporter interface {
	// IsExported reports whether the topic+date combination has already been exported.
	IsExported(ctx context.Context, topicID int64, date string) (bool, error)
	// MarkExported records a successful export.
	MarkExported(ctx context.Context, topicID int64, date string) error
	// MarkFailed records a failed export.
	MarkFailed(ctx context.Context, topicID int64, date string, reason string) error
}

// VaultWriter writes content to the Obsidian vault.
type VaultWriter interface {
	WriteAtomic(path, content string) error
}

// DefaultVaultWriter implements VaultWriter using the obsidian package.
type DefaultVaultWriter struct{}

// WriteAtomic delegates to obsidian.WriteAtomic.
func (w *DefaultVaultWriter) WriteAtomic(path, content string) error {
	return obsidian.WriteAtomic(path, content)
}

// PublishDailyTopicsJob orchestrates the daily digest publishing pipeline:
// digest selection → LLM summary → Obsidian markdown → vault write.
// Deprecated: Use PublishKnowledgeSnapshotJob for new implementations.
type PublishDailyTopicsJob struct {
	digestSvc  *digest.Service
	llmClient  llm.Client
	exporter   TopicExporter
	writer     VaultWriter
	vaultRoot  string
	monitor    MonitorConfig
	knowledgeDelegate *PublishKnowledgeSnapshotJob // STE-356 compatibility adapter
}

// MonitorConfig holds the monitor metadata needed for publishing.
type MonitorConfig struct {
	ID   int64
	Name string
	Slug string
}

// NewPublishDailyTopicsJob creates a new PublishDailyTopicsJob.
// Deprecated: Use NewPublishKnowledgeSnapshotJob for new implementations.
func NewPublishDailyTopicsJob(
	digestSvc *digest.Service,
	llmClient llm.Client,
	exporter TopicExporter,
	writer VaultWriter,
	vaultRoot string,
	monitor MonitorConfig,
) *PublishDailyTopicsJob {
	return &PublishDailyTopicsJob{
		digestSvc: digestSvc,
		llmClient: llmClient,
		exporter:  exporter,
		writer:    writer,
		vaultRoot: vaultRoot,
		monitor:   monitor,
	}
}

// NewPublishDailyTopicsJobWithDelegate creates a PublishDailyTopicsJob that
// delegates to the new PublishKnowledgeSnapshotJob.
func NewPublishDailyTopicsJobWithDelegate(
	digestSvc *digest.Service,
	llmClient llm.Client,
	exporter TopicExporter,
	writer VaultWriter,
	vaultRoot string,
	monitor MonitorConfig,
) *PublishDailyTopicsJob {
	j := NewPublishDailyTopicsJob(digestSvc, llmClient, exporter, writer, vaultRoot, monitor)
	j.knowledgeDelegate = newKnowledgeSnapshotDelegate(digestSvc, llmClient, exporter, vaultRoot)
	return j
}

// ExportResult holds the outcome of publishing a single topic.
type ExportResult struct {
	TopicID int64
	Title   string
	Status  string // "published" or "failed"
	Error   error
}

// Run executes the full daily digest publishing pipeline for a given date.
// It returns results for each topic processed.
// When knowledgeDelegate is set, also runs the knowledge sync baseline.
func (j *PublishDailyTopicsJob) Run(ctx context.Context, now time.Time, target string) ([]ExportResult, error) {
	exportDate := digest.ResolveExportDate(now, target)
	dateStr := exportDate.Format("2006-01-02")

	d, err := j.digestSvc.BuildDayDigest(ctx, j.monitor.ID, now, target, digest.DefaultTopN)
	if err != nil {
		return nil, fmt.Errorf("build digest: %w", err)
	}

	results := make([]ExportResult, 0, len(d.Topics))
	for _, td := range d.Topics {
		r := j.publishTopic(ctx, td, dateStr)
		results = append(results, r)
	}

	// Run the knowledge sync baseline when delegate is configured.
	if j.knowledgeDelegate != nil {
		if _, err := j.knowledgeDelegate.Run(ctx, now); err != nil {
			log.Printf("publish: knowledge delegate error: %v", err)
		}
	}

	return results, nil
}

func (j *PublishDailyTopicsJob) publishTopic(ctx context.Context, td digest.TopicDigest, dateStr string) ExportResult {
	result := ExportResult{
		TopicID: td.Topic.ID,
		Title:   td.Topic.Title,
	}

	// Check export status for error reporting; always re-render per S10
	if _, err := j.exporter.IsExported(ctx, td.Topic.ID, dateStr); err != nil {
		result.Status = "failed"
		result.Error = fmt.Errorf("check export status: %w", err)
		return result
	}

	// Build LLM input from topic + posts
	input := buildLLMInput(j.monitor.Name, td)
	summary, err := j.llmClient.SummarizeTopic(ctx, input)
	if err != nil {
		_ = j.exporter.MarkFailed(ctx, td.Topic.ID, dateStr, err.Error())
		log.Printf("publish: topic %d LLM error: %v", td.Topic.ID, err)
		result.Status = "failed"
		result.Error = err
		return result
	}

	// Render markdown
	posts := make([]obsidian.PostExcerpt, 0, len(td.Posts))
	for _, p := range td.Posts {
		posts = append(posts, obsidian.PostExcerpt{
			Author:  p.AuthorName,
			Excerpt: p.ContentExcerpt,
			URL:     p.PostURL,
		})
	}

	noteInput := obsidian.TopicNoteInput{
		Date:      dateStr,
		Monitor:   j.monitor.Name,
		MonitorID: j.monitor.ID,
		TopicID:   td.Topic.ID,
		TopicKey:  td.Topic.Title,
		Title:     td.Topic.Title,
		Heat:      td.Topic.Heat,
		Trend:     "stable",
		PostCount: len(td.Posts),
		Summary:   summary,
		Posts:     posts,
	}

	content := obsidian.RenderTopicNote(noteInput)
	topicSlug := obsidian.Slugify(td.Topic.Title)
	idStr := fmt.Sprintf("%d", td.Topic.ID)
	path := obsidian.BuildPath(j.vaultRoot, j.monitor.Slug, dateStr, idStr, topicSlug)

	// Write to vault
	if err := j.writer.WriteAtomic(path, content); err != nil {
		_ = j.exporter.MarkFailed(ctx, td.Topic.ID, dateStr, err.Error())
		log.Printf("publish: topic %d write error: %v", td.Topic.ID, err)
		result.Status = "failed"
		result.Error = err
		return result
	}

	// Mark success
	if err := j.exporter.MarkExported(ctx, td.Topic.ID, dateStr); err != nil {
		log.Printf("publish: topic %d mark exported error: %v", td.Topic.ID, err)
	}
	result.Status = "published"
	return result
}

func buildLLMInput(monitorName string, td digest.TopicDigest) llm.TopicSummaryInput {
	posts := make([]llm.PostInput, 0, len(td.Posts))
	for _, p := range td.Posts {
		posts = append(posts, llm.PostInput{
			Author:  p.AuthorName,
			Content: p.ContentExcerpt,
			URL:     p.PostURL,
		})
	}
	return llm.TopicSummaryInput{
		MonitorName: monitorName,
		TopicTitle:  td.Topic.Title,
		Heat:        td.Topic.Heat,
		Trend:       "stable",
		PostCount:   len(td.Posts),
		Posts:       posts,
	}
}
