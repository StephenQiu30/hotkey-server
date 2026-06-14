package jobs

import (
	"context"
	"fmt"
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
	// Use BuildDayDigest which handles the full pipeline
	dayDigest, err := a.svc.BuildDayDigest(ctx, monitorID, time.Now(), "yesterday", topN)
	if err != nil {
		return nil, err
	}

	topics := make([]TopicCandidate, 0, len(dayDigest.Topics))
	for _, td := range dayDigest.Topics {
		posts := make([]RepresentativePost, 0, len(td.Posts))
		for _, p := range td.Posts {
			posts = append(posts, RepresentativePost{
				AuthorName: p.AuthorName,
				Text:       p.ContentExcerpt,
				URL:        p.PostURL,
			})
		}
		topics = append(topics, TopicCandidate{
			TopicID:        td.Topic.ID,
			TopicKey:       "", // Not available in TopicEntry
			Title:          td.Topic.Title,
			HeatScore:      td.Topic.Heat,
			TrendDirection: "", // Not available in TopicEntry
			PostCount:      len(td.Posts),
			Posts:          posts,
		})
	}
	return topics, nil
}

// ExportRepoAdapter wraps *database.DigestRepo to implement ExportRepository.
type ExportRepoAdapter struct {
	repo *database.DigestRepo
}

// NewExportRepoAdapter creates a new ExportRepoAdapter.
func NewExportRepoAdapter(repo *database.DigestRepo) *ExportRepoAdapter {
	return &ExportRepoAdapter{repo: repo}
}

// UpsertExport converts jobs.ExportRecord to digest.TopicDailyExport and delegates.
func (a *ExportRepoAdapter) UpsertExport(ctx context.Context, rec ExportRecord) (int64, error) {
	exportDate, err := time.Parse("2006-01-02", rec.ExportDate)
	if err != nil {
		return 0, fmt.Errorf("parse export date: %w", err)
	}

	status := digest.ExportStatus(rec.Status)
	if status == "" {
		status = digest.StatusPending
	}

	result, err := a.repo.Upsert(digest.TopicDailyExport{
		MonitorID:    rec.MonitorID,
		TopicID:      rec.TopicID,
		ExportDate:   exportDate,
		SummaryText:  rec.SummaryText,
		MarkdownPath: rec.MarkdownPath,
		Status:       status,
		ErrorMessage: rec.ErrorMessage,
	})
	if err != nil {
		return 0, err
	}
	return result.ID, nil
}

// GetLastRunDate retrieves the last run date from job_metadata table.
func (a *ExportRepoAdapter) GetLastRunDate(ctx context.Context) (string, error) {
	// This requires a separate query to job_metadata table
	// For now, we'll implement a simple version using the database directly
	// In a real implementation, this would be in the database package
	return "", nil // TODO: implement with job_metadata table
}

// SetLastRunDate persists the last run date to job_metadata table.
func (a *ExportRepoAdapter) SetLastRunDate(ctx context.Context, date string) error {
	// This requires a separate query to job_metadata table
	// For now, we'll implement a simple version using the database directly
	// In a real implementation, this would be in the database package
	return nil // TODO: implement with job_metadata table
}

// LLMClientAdapter wraps llm.Client interface to implement LLMClient.
type LLMClientAdapter struct {
	client llm.Client
}

// NewLLMClientAdapter creates a new LLMClientAdapter.
func NewLLMClientAdapter(client llm.Client) *LLMClientAdapter {
	return &LLMClientAdapter{client: client}
}

// SummarizeTopic delegates to the LLM client and converts types.
func (a *LLMClientAdapter) SummarizeTopic(ctx context.Context, in TopicSummaryInput) (string, error) {
	posts := make([]llm.PostInput, 0, len(in.Posts))
	for _, p := range in.Posts {
		posts = append(posts, llm.PostInput{
			Author:  p.AuthorName,
			Content: p.Text,
			URL:     p.URL,
		})
	}
	return a.client.SummarizeTopic(ctx, llm.TopicSummaryInput{
		TopicTitle: in.TopicTitle,
		TopicKey:   in.TopicKey,
		Heat:       in.HeatScore,
		Trend:      in.Trend,
		PostCount:  in.PostCount,
		Posts:      posts,
	})
}

// ObsidianWriterAdapter wraps obsidian functions to implement ObsidianWriter.
type ObsidianWriterAdapter struct{}

// NewObsidianWriterAdapter creates a new ObsidianWriterAdapter.
func NewObsidianWriterAdapter() *ObsidianWriterAdapter {
	return &ObsidianWriterAdapter{}
}

// WriteTopicNote renders and writes a topic note to the vault.
func (a *ObsidianWriterAdapter) WriteTopicNote(ctx context.Context, in ObsidianNoteInput) (string, error) {
	// Convert posts to obsidian format
	posts := make([]obsidian.PostExcerpt, 0, len(in.Posts))
	for _, p := range in.Posts {
		posts = append(posts, obsidian.PostExcerpt{
			Author:  p.AuthorName,
			Excerpt: p.Text,
			URL:     p.URL,
		})
	}

	// Render the note
	noteContent := obsidian.RenderTopicNote(obsidian.TopicNoteInput{
		Date:      in.Date,
		Monitor:   in.MonitorName,
		MonitorID: in.MonitorID,
		TopicID:   in.TopicID,
		TopicKey:  in.TopicKey,
		Title:     in.Title,
		Heat:      in.HeatScore,
		Trend:     in.Trend,
		PostCount: in.PostCount,
		Summary:   in.Summary,
		Posts:     posts,
	})

	// Build the file path
	monitorSlug := obsidian.Slugify(in.MonitorName)
	mdPath := obsidian.BuildPath(in.VaultPath, monitorSlug, in.Date, fmt.Sprintf("%d", in.TopicID), obsidian.Slugify(in.Title))

	// Write atomically
	if err := obsidian.WriteAtomic(mdPath, noteContent); err != nil {
		return "", err
	}

	return mdPath, nil
}
