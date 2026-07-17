package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

// ClusteringWriteResult reports the only three safe outcomes of applying a
// deterministic clustering decision: attach/create an Event, or retain a
// review decision without changing membership.
type ClusteringWriteResult struct {
	Event         *domain.Event
	Created       bool
	PendingReview bool
}

type ClusteringWriter interface {
	ApplyClustering(context.Context, []domain.Decision) (ClusteringWriteResult, error)
}

type ClusteringExecutionInput struct {
	ContentID         int64
	ClusteringVersion string
	FeatureInputHash  string
	Scores            map[string]domain.ScoreBreakdown
	HardConflicts     map[string]bool
}

func (input ClusteringExecutionInput) Validate() error {
	return (ClusteringInput{
		ContentID:         input.ContentID,
		ClusteringVersion: input.ClusteringVersion,
		FeatureInputHash:  input.FeatureInputHash,
		Scores:            input.Scores,
	}).Validate()
}

type ClusteringExecutionResult struct {
	Decisions         []domain.Decision
	Event             *domain.Event
	Created           bool
	PendingReview     bool
	VectorUnavailable bool
}

type ClusteringExecutionService struct {
	recall     *RecallService
	clustering *ClusteringService
	writer     ClusteringWriter
}

func NewClusteringExecutionService(recall *RecallService, clustering *ClusteringService, writer ClusteringWriter) *ClusteringExecutionService {
	return &ClusteringExecutionService{recall: recall, clustering: clustering, writer: writer}
}

// Execute keeps candidate recall, deterministic scoring and the membership
// write in one use case. The repository is responsible for making the final
// decision log and Event mutation one PostgreSQL transaction.
func (service *ClusteringExecutionService) Execute(ctx context.Context, input ClusteringExecutionInput) (ClusteringExecutionResult, error) {
	if service == nil || service.recall == nil || service.clustering == nil || service.writer == nil {
		return ClusteringExecutionResult{}, fmt.Errorf("clustering execution dependencies are required")
	}
	if err := input.Validate(); err != nil {
		return ClusteringExecutionResult{}, err
	}
	recall, err := service.recall.Recall(ctx, RecallInput{ContentID: input.ContentID})
	if err != nil {
		return ClusteringExecutionResult{}, err
	}
	decisions, err := service.clustering.Evaluate(ctx, ClusteringInput{
		ContentID:         input.ContentID,
		ClusteringVersion: input.ClusteringVersion,
		FeatureInputHash:  input.FeatureInputHash,
		Candidates:        recall.Candidates,
		Scores:            input.Scores,
		HardConflicts:     input.HardConflicts,
	})
	if err != nil {
		return ClusteringExecutionResult{}, err
	}
	applied, err := service.writer.ApplyClustering(ctx, decisions)
	if err != nil {
		return ClusteringExecutionResult{}, err
	}
	return ClusteringExecutionResult{
		Decisions:         decisions,
		Event:             applied.Event,
		Created:           applied.Created,
		PendingReview:     applied.PendingReview,
		VectorUnavailable: recall.VectorUnavailable,
	}, nil
}
