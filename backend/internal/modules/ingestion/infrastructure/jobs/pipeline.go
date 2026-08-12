package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type ContentRepository interface {
	GetActive(context.Context, int64) (ingestiondomain.Content, error)
}

type RelevanceRepository interface {
	UpsertSnapshot(context.Context, ingestiondomain.RelevanceSnapshotInput) (ingestiondomain.RelevanceSnapshot, bool, error)
}

type EvaluateJobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

type capturedRunIngester interface {
	IngestRun(context.Context, ingestionapplication.IngestRunInput) (ingestionapplication.IngestRunResult, error)
}

// NormalizeHandler keeps the legacy Content projection current for API and
// metric compatibility. The evidence-owned document pipeline is scheduled at
// evidence commit time, so normalization must not fan out to the retired
// relevance/event pipeline.
type NormalizeHandler struct {
	service capturedRunIngester
}

func NewNormalizeHandler(service *ingestionapplication.Service) (*NormalizeHandler, error) {
	return newNormalizeHandler(service)
}

func newNormalizeHandler(service capturedRunIngester) (*NormalizeHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("normalize handler service is required")
	}
	return &NormalizeHandler{service: service}, nil
}

func (handler *NormalizeHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindNormalizeContent); err != nil {
		return queue.NewPermanentError(err)
	}
	for {
		result, err := handler.service.IngestRun(ctx, ingestionapplication.IngestRunInput{RunID: job.Payload.EntityID})
		if err != nil {
			return queue.ClassifyHandlerError(ctx, err)
		}
		if result.NextCursor == "" {
			return nil
		}
		if result.Processed == 0 {
			return queue.NewPermanentError(fmt.Errorf("normalize captured item pagination made no progress"))
		}
	}
}

// EvaluateHandler persists deterministic MonitorMatch snapshots. AI review is
// intentionally reached only by its existing Application facade in a later
// review step; this handler never places provider input in the queue.
type EvaluateHandler struct {
	contents   ContentRepository
	candidates *ingestionapplication.CandidateRecallService
	snapshots  RelevanceRepository
	jobs       EvaluateJobEnqueuer
	now        func() time.Time
}

func NewEvaluateHandler(contents ContentRepository, candidates *ingestionapplication.CandidateRecallService, snapshots RelevanceRepository, jobs *queue.Store) (*EvaluateHandler, error) {
	return NewEvaluateHandlerWithClock(contents, candidates, snapshots, jobs, func() time.Time { return time.Now().UTC() })
}

func NewEvaluateHandlerWithClock(contents ContentRepository, candidates *ingestionapplication.CandidateRecallService, snapshots RelevanceRepository, jobs EvaluateJobEnqueuer, now func() time.Time) (*EvaluateHandler, error) {
	if contents == nil || candidates == nil || snapshots == nil || jobs == nil || now == nil {
		return nil, fmt.Errorf("evaluate handler dependencies are required")
	}
	return &EvaluateHandler{contents: contents, candidates: candidates, snapshots: snapshots, jobs: jobs, now: now}, nil
}

func (handler *EvaluateHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindEvaluateRelevance); err != nil {
		return queue.NewPermanentError(err)
	}
	content, err := handler.contents.GetActive(ctx, job.Payload.EntityID)
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	language := content.Language
	if language == "" {
		language = "und"
	}
	results, err := handler.candidates.Score(ctx, ingestionapplication.RelevanceScoreRequest{Content: ingestionapplication.RelevanceContent{
		ID: content.ID, SourceConnectionID: content.SourceConnectionID, DedupeKey: content.ContentHash,
		Language: language, Title: content.Title, Excerpt: content.Excerpt, CanonicalURL: content.CanonicalURL,
		AuthorExternalID: content.Author.ExternalID, AuthorName: content.Author.DisplayName,
	}})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	for _, scored := range results {
		explanation, err := json.Marshal(map[string]any{
			"matched_terms": scored.MatchedTerms, "matched_entities": scored.MatchedEntities, "excluded_terms": scored.ExcludedTerms,
			"recall_paths": scored.RecallPaths, "reason_codes": scored.ReasonCodes,
			"scores":     map[string]float64{"semantic": optionalScore(scored.Factors.Semantic), "lexical": scored.Factors.Lexical, "entity": scored.Factors.Entity, "title": scored.Factors.Title, "preference": scored.Factors.Preference},
			"provenance": map[string]any{"scoring_version": scored.ScoringVersion},
		})
		if err != nil {
			return queue.NewPermanentError(err)
		}
		input := ingestiondomain.RelevanceSnapshotInput{
			MonitorID: scored.MonitorID, MonitorConfigVersionID: scored.MonitorConfigVersionID, ContentID: content.ID,
			InputHash: scored.InputHash, ScoringVersion: scored.ScoringVersion, RecallPaths: scored.RecallPaths, ReasonCodes: scored.ReasonCodes,
			RuleScore: scored.RuleScore, SemanticScore: scored.Factors.Semantic, FinalScore: scored.RuleScore, Decision: scored.Decision,
			DecisionOrigin: ingestiondomain.DecisionOriginRule, Explanation: explanation, Degraded: scored.Degraded,
		}
		if _, _, err := handler.snapshots.UpsertSnapshot(ctx, input); err != nil {
			return queue.ClassifyHandlerError(ctx, err)
		}
	}
	if !ingestiondomain.EligibleForNewEvent(content.PublishedAt, handler.now().UTC()) {
		return nil
	}
	clusterHash := queue.StableJobHash(queue.KindClusterContent, fmt.Sprint(content.ID), fmt.Sprint(content.Version), job.Payload.InputHash)
	_, _, err = handler.jobs.Enqueue(ctx, queue.Job{
		Kind:        queue.KindClusterContent,
		UniqueKey:   queue.StableJobKey(queue.KindClusterContent, content.ID, content.Version, clusterHash),
		Payload:     queue.Payload{EntityID: content.ID, EntityVersion: content.Version, WindowStart: job.Payload.WindowStart, WindowEnd: job.Payload.WindowEnd, InputHash: clusterHash},
		ScheduledAt: job.ScheduledAt, MaxAttempts: 3, Priority: 4,
	})
	return queue.ClassifyHandlerError(ctx, err)
}

func optionalScore(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
