package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type ContentRepository interface {
	GetActive(context.Context, int64) (ingestiondomain.Content, error)
}

type RelevanceRepository interface {
	UpsertSnapshot(context.Context, ingestiondomain.RelevanceSnapshotInput) (ingestiondomain.RelevanceSnapshot, bool, error)
}

// NormalizeHandler consumes only Source-owned captured items and schedules the
// next deterministic stage for each Content fact produced by the use case.
type NormalizeHandler struct {
	service *ingestionapplication.Service
	jobs    *queue.Store
}

func NewNormalizeHandler(service *ingestionapplication.Service, jobs *queue.Store) (*NormalizeHandler, error) {
	if service == nil || jobs == nil {
		return nil, fmt.Errorf("normalize handler dependencies are required")
	}
	return &NormalizeHandler{service: service, jobs: jobs}, nil
}

func (handler *NormalizeHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindNormalizeContent); err != nil {
		return queue.NewPermanentError(err)
	}
	_, err := handler.service.IngestRunWithHook(ctx, ingestionapplication.IngestRunInput{RunID: job.Payload.EntityID}, func(transactionCtx context.Context, contentID int64) error {
		inputHash := queue.StableJobHash(queue.KindEvaluateRelevance, fmt.Sprint(contentID), fmt.Sprint(job.Payload.EntityVersion), job.Payload.InputHash)
		_, _, err := handler.jobs.Enqueue(transactionCtx, queue.Job{
			Kind:        queue.KindEvaluateRelevance,
			UniqueKey:   queue.StableJobKey(queue.KindEvaluateRelevance, contentID, job.Payload.EntityVersion, inputHash),
			Payload:     queue.Payload{EntityID: contentID, EntityVersion: job.Payload.EntityVersion, WindowStart: job.Payload.WindowStart, WindowEnd: job.Payload.WindowEnd, InputHash: inputHash},
			ScheduledAt: job.ScheduledAt, MaxAttempts: 3, Priority: 3,
		})
		return err
	})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	return nil
}

// EvaluateHandler persists deterministic MonitorMatch snapshots. AI review is
// intentionally reached only by its existing Application facade in a later
// review step; this handler never places provider input in the queue.
type EvaluateHandler struct {
	contents   ContentRepository
	candidates *ingestionapplication.CandidateRecallService
	snapshots  RelevanceRepository
	jobs       *queue.Store
}

func NewEvaluateHandler(contents ContentRepository, candidates *ingestionapplication.CandidateRecallService, snapshots RelevanceRepository, jobs *queue.Store) (*EvaluateHandler, error) {
	if contents == nil || candidates == nil || snapshots == nil || jobs == nil {
		return nil, fmt.Errorf("evaluate handler dependencies are required")
	}
	return &EvaluateHandler{contents: contents, candidates: candidates, snapshots: snapshots, jobs: jobs}, nil
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
