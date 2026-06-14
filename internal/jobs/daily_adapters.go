package jobs

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"github.com/StephenQiu30/hotkey-server/internal/llm"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

// DigestServiceAdapter wraps *digest.Service to implement DigestService.
type DigestServiceAdapter struct {
	svc *digest.Service
}

// NewDigestServiceAdapter creates a new DigestServiceAdapter.
func NewDigestServiceAdapter(svc *digest.Service) *DigestServiceAdapter {
	return &DigestServiceAdapter{svc: svc}
}

// ListTopicsForDay delegates to digest.Service and converts results.
func (a *DigestServiceAdapter) ListTopicsForDay(ctx context.Context, monitorID int64, exportDate time.Time, topN int) ([]TopicCandidate, error) {
	digestTopics, err := a.svc.ListTopicsForDay(ctx, monitorID, exportDate, topN)
	if err != nil {
		return nil, err
	}

	topics := make([]TopicCandidate, 0, len(digestTopics))
	for _, dt := range digestTopics {
		posts := make([]RepresentativePost, 0, len(dt.Posts))
		for _, p := range dt.Posts {
			posts = append(posts, RepresentativePost{
				AuthorName: p.AuthorName,
				Text:       p.Text,
				URL:        p.URL,
			})
		}
		topics = append(topics, TopicCandidate{
			TopicID:        dt.TopicID,
			TopicKey:       dt.TopicKey,
			Title:          dt.Title,
			HeatScore:      dt.HeatScore,
			TrendDirection: dt.TrendDirection,
			PostCount:      dt.PostCount,
			Posts:          posts,
		})
	}
	return topics, nil
}

// ExportRepoAdapter wraps *database.DigestExportRepo to implement ExportRepository.
type ExportRepoAdapter struct {
	repo *database.DigestExportRepo
}

// NewExportRepoAdapter creates a new ExportRepoAdapter.
func NewExportRepoAdapter(repo *database.DigestExportRepo) *ExportRepoAdapter {
	return &ExportRepoAdapter{repo: repo}
}

// UpsertExport converts jobs.ExportRecord to database.ExportRecord and delegates.
func (a *ExportRepoAdapter) UpsertExport(ctx context.Context, rec ExportRecord) (int64, error) {
	return a.repo.UpsertExport(ctx, database.ExportRecord{
		ID:           rec.ID,
		MonitorID:    rec.MonitorID,
		TopicID:      rec.TopicID,
		ExportDate:   rec.ExportDate,
		SummaryText:  rec.SummaryText,
		MarkdownPath: rec.MarkdownPath,
		Status:       rec.Status,
		ErrorMessage: rec.ErrorMessage,
	})
}

// GetLastRunDate delegates to the database repo.
func (a *ExportRepoAdapter) GetLastRunDate(ctx context.Context) (string, error) {
	return a.repo.GetLastRunDate(ctx)
}

// SetLastRunDate delegates to the database repo.
func (a *ExportRepoAdapter) SetLastRunDate(ctx context.Context, date string) error {
	return a.repo.SetLastRunDate(ctx, date)
}

// LLMClientAdapter wraps *llm.Client to implement LLMClient.
type LLMClientAdapter struct {
	client *llm.Client
}

// NewLLMClientAdapter creates a new LLMClientAdapter.
func NewLLMClientAdapter(client *llm.Client) *LLMClientAdapter {
	return &LLMClientAdapter{client: client}
}

// SummarizeTopic delegates to the LLM client and converts types.
func (a *LLMClientAdapter) SummarizeTopic(ctx context.Context, in TopicSummaryInput) (string, error) {
	posts := make([]llm.Post, 0, len(in.Posts))
	for _, p := range in.Posts {
		posts = append(posts, llm.Post{
			AuthorName: p.AuthorName,
			Text:       p.Text,
			URL:        p.URL,
		})
	}
	return a.client.SummarizeTopic(ctx, llm.SummarizeInput{
		MonitorName: in.MonitorName,
		TopicTitle:  in.TopicTitle,
		TopicKey:    in.TopicKey,
		HeatScore:   in.HeatScore,
		Trend:       in.Trend,
		PostCount:   in.PostCount,
		Posts:       posts,
	})
}

// ObsidianWriterAdapter wraps *obsidian.Writer to implement ObsidianWriter.
type ObsidianWriterAdapter struct {
	writer *obsidian.Writer
}

// NewObsidianWriterAdapter creates a new ObsidianWriterAdapter.
func NewObsidianWriterAdapter(writer *obsidian.Writer) *ObsidianWriterAdapter {
	return &ObsidianWriterAdapter{writer: writer}
}

// WriteTopicNote delegates to the Obsidian writer and converts types.
func (a *ObsidianWriterAdapter) WriteTopicNote(ctx context.Context, in ObsidianNoteInput) (string, error) {
	posts := make([]obsidian.Post, 0, len(in.Posts))
	for _, p := range in.Posts {
		posts = append(posts, obsidian.Post{
			AuthorName: p.AuthorName,
			Text:       p.Text,
			URL:        p.URL,
		})
	}
	return a.writer.WriteTopicNote(ctx, obsidian.NoteInput{
		VaultPath:   in.VaultPath,
		MonitorID:   in.MonitorID,
		MonitorName: in.MonitorName,
		MonitorSlug: in.MonitorSlug,
		TopicID:     in.TopicID,
		TopicKey:    in.TopicKey,
		Title:       in.Title,
		Date:        in.Date,
		HeatScore:   in.HeatScore,
		Trend:       in.Trend,
		PostCount:   in.PostCount,
		Summary:     in.Summary,
		Posts:       posts,
	})
}
