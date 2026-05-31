package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
)

func TestScoreHotspotsHandlerProcessesClusterRun(t *testing.T) {
	repo := domainhotspot.NewMemoryRepository()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

	// Setup test data
	item := content.SourceItem{
		ID: "item-1", SourceID: "src-1", Title: "AI Agent 新突破", Snippet: "自动化能力增强",
		PublishedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.SaveItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveEmbedding(context.Background(), domainhotspot.Embedding{
		ItemID: item.ID, Model: "text-embedding-v2", Vector: []float64{1, 0, 0}, Status: domainhotspot.EmbeddingStatusSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	cluster := domainhotspot.Cluster{
		ID: "cluster-1", Title: "AI Agent 新突破", Keywords: []string{"ai"},
		Centroid: []float64{1, 0, 0}, WindowStart: now.Add(-24 * time.Hour), WindowEnd: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.ReplaceClusters(context.Background(),
		[]domainhotspot.Cluster{cluster},
		map[string][]domainhotspot.ClusterItem{
			"cluster-1": {{ClusterID: "cluster-1", ItemID: "item-1", Similarity: 1, CreatedAt: now}},
		},
	); err != nil {
		t.Fatal(err)
	}

	scoreRepo := servicehotspot.NewMemoryScoreRepository()
	svc := servicehotspot.NewScoringService(servicehotspot.ScoringConfig{}, repo, scoreRepo)
	svc.SetClock(func() time.Time { return now })

	handler := NewScoreHotspotsHandler(svc)

	payload, _ := json.Marshal(queue.ScoreHotspotsPayload{ClusterRunID: "run-1"})
	job := queue.Job{ID: "job-score-1", Type: queue.JobTypeScoreHotspots, Payload: payload}

	err := handler.Handle(context.Background(), job)
	if err != nil {
		t.Fatalf("expected handler to succeed, got %v", err)
	}

	// Verify scores were saved
	scores, err := svc.ListScores(context.Background())
	if err != nil {
		t.Fatalf("list scores failed: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("expected 1 score after handler, got %d", len(scores))
	}
}

func TestScoreHotspotsHandlerRejectsInvalidPayload(t *testing.T) {
	repo := domainhotspot.NewMemoryRepository()
	svc := servicehotspot.NewScoringService(servicehotspot.ScoringConfig{}, repo, nil)
	handler := NewScoreHotspotsHandler(svc)

	// Empty cluster_run_id
	payload, _ := json.Marshal(queue.ScoreHotspotsPayload{ClusterRunID: ""})
	job := queue.Job{ID: "job-invalid", Type: queue.JobTypeScoreHotspots, Payload: payload}

	err := handler.Handle(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for empty cluster_run_id")
	}
}
