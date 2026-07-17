package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type clusteringWriterFake struct {
	decisions []domain.Decision
	result    ClusteringWriteResult
	err       error
}

func (writer *clusteringWriterFake) ApplyClustering(_ context.Context, decisions []domain.Decision) (ClusteringWriteResult, error) {
	writer.decisions = append([]domain.Decision(nil), decisions...)
	return writer.result, writer.err
}

func TestClusteringExecutionCreatesEventFromEmptyRecall(t *testing.T) {
	event := domain.Event{ID: 7, Version: 1, EventKey: "evt_7", TitleZH: "事件", LifecycleStatus: domain.LifecycleDetected, FirstSeenAt: time.Now().UTC(), LastSeenAt: time.Now().UTC()}
	writer := &clusteringWriterFake{result: ClusteringWriteResult{Event: &event, Created: true}}
	service := NewClusteringExecutionService(NewRecallService(recallReaderFake{}), NewClusteringService(), writer)
	result, err := service.Execute(context.Background(), ClusteringExecutionInput{ContentID: 1, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("content", "1"), Scores: map[string]domain.ScoreBreakdown{}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Created || result.Event == nil || result.Event.ID != event.ID || len(writer.decisions) != 1 || writer.decisions[0].Decision != domain.DecisionNewEvent {
		t.Fatalf("Execute() = %#v, decisions = %#v", result, writer.decisions)
	}
}

func TestClusteringExecutionRetainsReviewWithoutMembership(t *testing.T) {
	candidate := domain.Candidate{EventID: 2, EventKey: "evt_2", Channel: domain.ChannelLexical, Score: 80}
	writer := &clusteringWriterFake{result: ClusteringWriteResult{PendingReview: true}}
	service := NewClusteringExecutionService(NewRecallService(recallReaderFake{all: []domain.Candidate{candidate}, vectorErr: errors.New("vector unavailable")}), NewClusteringService(), writer)
	result, err := service.Execute(context.Background(), ClusteringExecutionInput{
		ContentID: 1, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("content", "1"),
		Scores: map[string]domain.ScoreBreakdown{"evt_2": {EntityAction: 70, Semantic: 70, Temporal: 70, Location: 70, SourceContext: 70}},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.PendingReview || !result.VectorUnavailable || result.Event != nil || len(writer.decisions) != 1 || writer.decisions[0].Decision != domain.DecisionReview {
		t.Fatalf("Execute() = %#v, decisions = %#v", result, writer.decisions)
	}
}
