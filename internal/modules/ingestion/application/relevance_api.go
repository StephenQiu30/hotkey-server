package application

import (
	"context"
	"errors"
	"fmt"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// RelevanceMatchDetail joins an auditable match with its currently active,
// safe Content projection. Application performs the join so HTTP does not
// gain a repository or a way to inspect deleted Content.
type RelevanceMatchDetail struct {
	Snapshot ingestiondomain.RelevanceSnapshot
	Content  ingestiondomain.Content
}

// RelevancePreviewItem is an in-memory, zero-write result for one recent
// Content item. It has no AI run or persisted monitor_match identity.
type RelevancePreviewItem struct {
	ContentID  int64
	Candidates []ScoredRelevanceCandidate
}

type relevanceAPISnapshotRepository interface {
	ListLatestSnapshots(context.Context, int64, ingestiondomain.RelevanceSnapshotListQuery) (ingestiondomain.RelevanceSnapshotPage, error)
	GetActiveSnapshot(context.Context, int64, int64) (ingestiondomain.RelevanceSnapshot, error)
	CurrentPublishedMonitorConfig(context.Context, int64) (int64, error)
	UpsertFeedback(context.Context, ingestiondomain.RelevanceFeedbackInput) (ingestiondomain.RelevanceFeedback, error)
	UpsertFalseNegativeFeedback(context.Context, ingestiondomain.RelevanceFeedbackInput) (ingestiondomain.RelevanceFeedback, error)
	RefreshSuggestions(context.Context, int64) (int, error)
	ListSuggestions(context.Context, int64, ingestiondomain.RelevanceSuggestionListQuery) (ingestiondomain.RelevanceSuggestionPage, error)
	ReviewSuggestion(context.Context, int64, int64, int64, int64, ingestiondomain.SuggestionStatus) (ingestiondomain.RelevanceSuggestion, error)
	FeedbackEvaluations(context.Context, int64) ([]ingestiondomain.RelevanceEvaluation, error)
}

type RelevanceAPIServiceDependencies struct {
	Snapshots  relevanceAPISnapshotRepository
	Contents   ingestiondomain.ContentRepository
	Candidates ingestiondomain.RelevanceCandidateReader
}

// RelevanceAPIService is the sole use-case boundary for the PLAN-009 public
// routes. Preview deliberately creates a candidate scorer without embeddings
// or a review executor, which makes its zero-AI, zero-write contract explicit.
type RelevanceAPIService struct {
	snapshots relevanceAPISnapshotRepository
	contents  ingestiondomain.ContentRepository
	preview   *CandidateRecallService
}

func NewRelevanceAPIService(dependencies RelevanceAPIServiceDependencies) (*RelevanceAPIService, error) {
	if dependencies.Snapshots == nil || dependencies.Contents == nil || dependencies.Candidates == nil {
		return nil, fmt.Errorf("relevance API dependencies are required")
	}
	preview, err := NewCandidateRecallService(dependencies.Candidates, nil)
	if err != nil {
		return nil, err
	}
	return &RelevanceAPIService{snapshots: dependencies.Snapshots, contents: dependencies.Contents, preview: preview}, nil
}

func (service *RelevanceAPIService) ListMatches(ctx context.Context, monitorID int64, query ingestiondomain.RelevanceSnapshotListQuery) (ingestiondomain.RelevanceSnapshotPage, error) {
	if service == nil || service.snapshots == nil {
		return ingestiondomain.RelevanceSnapshotPage{}, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return ingestiondomain.RelevanceSnapshotPage{}, err
	}
	page, err := service.snapshots.ListLatestSnapshots(ctx, monitorID, query)
	if err != nil {
		return ingestiondomain.RelevanceSnapshotPage{}, relevanceError(err)
	}
	return page, nil
}

func (service *RelevanceAPIService) GetMatch(ctx context.Context, monitorID, matchID int64) (RelevanceMatchDetail, error) {
	if service == nil || service.snapshots == nil || service.contents == nil {
		return RelevanceMatchDetail{}, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return RelevanceMatchDetail{}, err
	}
	snapshot, err := service.snapshots.GetActiveSnapshot(ctx, monitorID, matchID)
	if err != nil {
		return RelevanceMatchDetail{}, relevanceError(err)
	}
	content, err := service.contents.GetActive(ctx, snapshot.ContentID)
	if err != nil {
		return RelevanceMatchDetail{}, relevanceError(err)
	}
	return RelevanceMatchDetail{Snapshot: snapshot, Content: content}, nil
}

// Preview scores at most twenty active Content items using the deterministic,
// bounded scorer. It never persists a snapshot and has no review executor,
// therefore it cannot create an ai_run or make a Provider call.
func (service *RelevanceAPIService) Preview(ctx context.Context, monitorID int64) ([]RelevancePreviewItem, error) {
	if service == nil || service.snapshots == nil || service.contents == nil || service.preview == nil {
		return nil, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return nil, err
	}
	contents, err := service.contents.ListActive(ctx, ingestiondomain.ContentListQuery{Limit: 20})
	if err != nil {
		return nil, relevanceError(err)
	}
	items := make([]RelevancePreviewItem, 0, len(contents.Items))
	for _, content := range contents.Items {
		candidates, err := service.preview.Score(ctx, RelevanceScoreRequest{Content: RelevanceContent{
			ID: content.ID, SourceConnectionID: content.SourceConnectionID, DedupeKey: content.ContentHash,
			Language: content.Language, Title: content.Title, Excerpt: content.Excerpt, CanonicalURL: content.CanonicalURL,
			AuthorExternalID: content.Author.ExternalID, AuthorName: content.Author.DisplayName,
		}})
		if err != nil {
			return nil, relevanceError(err)
		}
		selected := make([]ScoredRelevanceCandidate, 0, 1)
		for _, candidate := range candidates {
			if candidate.MonitorID == monitorID {
				selected = append(selected, candidate)
			}
		}
		items = append(items, RelevancePreviewItem{ContentID: content.ID, Candidates: selected})
	}
	return items, nil
}

func (service *RelevanceAPIService) UpsertMatchFeedback(ctx context.Context, actorUserID, monitorID, matchID int64, feedbackType ingestiondomain.FeedbackType, expectedVersion *int64) (ingestiondomain.RelevanceFeedback, error) {
	if service == nil || service.snapshots == nil {
		return ingestiondomain.RelevanceFeedback{}, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return ingestiondomain.RelevanceFeedback{}, err
	}
	snapshot, err := service.snapshots.GetActiveSnapshot(ctx, monitorID, matchID)
	if err != nil {
		return ingestiondomain.RelevanceFeedback{}, relevanceError(err)
	}
	feedback, err := service.snapshots.UpsertFeedback(ctx, ingestiondomain.RelevanceFeedbackInput{
		MonitorID: monitorID, MonitorConfigVersionID: snapshot.MonitorConfigVersionID, ContentID: snapshot.ContentID,
		MonitorMatchID: &snapshot.ID, ActorUserID: actorUserID, ExpectedVersion: expectedVersion, FeedbackType: feedbackType,
	})
	if err != nil {
		return ingestiondomain.RelevanceFeedback{}, relevanceError(err)
	}
	return feedback, nil
}

// UpsertFalseNegativeContentFeedback records the only feedback accepted for a
// Content that has no relevance snapshot in the current published config.
// Keeping the feedback type out of this public use case prevents a caller from
// turning the unmatched-content route into a general match-feedback endpoint.
func (service *RelevanceAPIService) UpsertFalseNegativeContentFeedback(ctx context.Context, actorUserID, monitorID, contentID int64, expectedVersion *int64) (ingestiondomain.RelevanceFeedback, error) {
	if service == nil || service.snapshots == nil || service.contents == nil {
		return ingestiondomain.RelevanceFeedback{}, relevanceUnavailable()
	}
	configID, err := service.activePublishedMonitor(ctx, monitorID)
	if err != nil {
		return ingestiondomain.RelevanceFeedback{}, err
	}
	if _, err := service.contents.GetActive(ctx, contentID); err != nil {
		return ingestiondomain.RelevanceFeedback{}, relevanceError(err)
	}
	feedback, err := service.snapshots.UpsertFalseNegativeFeedback(ctx, ingestiondomain.RelevanceFeedbackInput{
		MonitorID: monitorID, MonitorConfigVersionID: configID, ContentID: contentID,
		ActorUserID: actorUserID, ExpectedVersion: expectedVersion, FeedbackType: ingestiondomain.FeedbackTypeFalseNegative,
	})
	if err != nil {
		return ingestiondomain.RelevanceFeedback{}, relevanceError(err)
	}
	return feedback, nil
}

func (service *RelevanceAPIService) RefreshSuggestions(ctx context.Context, monitorID int64) (int, error) {
	if service == nil || service.snapshots == nil {
		return 0, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return 0, err
	}
	count, err := service.snapshots.RefreshSuggestions(ctx, monitorID)
	if err != nil {
		return 0, relevanceError(err)
	}
	return count, nil
}

func (service *RelevanceAPIService) ListSuggestions(ctx context.Context, monitorID int64, query ingestiondomain.RelevanceSuggestionListQuery) (ingestiondomain.RelevanceSuggestionPage, error) {
	if service == nil || service.snapshots == nil {
		return ingestiondomain.RelevanceSuggestionPage{}, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return ingestiondomain.RelevanceSuggestionPage{}, err
	}
	page, err := service.snapshots.ListSuggestions(ctx, monitorID, query)
	if err != nil {
		return ingestiondomain.RelevanceSuggestionPage{}, relevanceError(err)
	}
	return page, nil
}

func (service *RelevanceAPIService) ReviewSuggestion(ctx context.Context, actorUserID, monitorID, suggestionID, expectedVersion int64, status ingestiondomain.SuggestionStatus) (ingestiondomain.RelevanceSuggestion, error) {
	if service == nil || service.snapshots == nil {
		return ingestiondomain.RelevanceSuggestion{}, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return ingestiondomain.RelevanceSuggestion{}, err
	}
	suggestion, err := service.snapshots.ReviewSuggestion(ctx, monitorID, suggestionID, actorUserID, expectedVersion, status)
	if err != nil {
		return ingestiondomain.RelevanceSuggestion{}, relevanceError(err)
	}
	return suggestion, nil
}

func (service *RelevanceAPIService) Evaluations(ctx context.Context, monitorID int64) ([]ingestiondomain.RelevanceEvaluation, error) {
	if service == nil || service.snapshots == nil {
		return nil, relevanceUnavailable()
	}
	if _, err := service.activePublishedMonitor(ctx, monitorID); err != nil {
		return nil, err
	}
	values, err := service.snapshots.FeedbackEvaluations(ctx, monitorID)
	if err != nil {
		return nil, relevanceError(err)
	}
	return values, nil
}

func (service *RelevanceAPIService) activePublishedMonitor(ctx context.Context, monitorID int64) (int64, error) {
	configID, err := service.snapshots.CurrentPublishedMonitorConfig(ctx, monitorID)
	if err != nil {
		return 0, relevanceError(err)
	}
	return configID, nil
}

func relevanceUnavailable() error { return sharederrors.New(sharederrors.CodeUnavailable, 503, "") }

func relevanceError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput):
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, 404, "")
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.New(sharederrors.CodeConflict, 409, "")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return relevanceUnavailable()
	default:
		return fmt.Errorf("relevance API operation: %w", err)
	}
}
