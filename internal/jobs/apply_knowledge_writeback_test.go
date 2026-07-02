package jobs

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/knowledge"
)

func TestApplyKnowledgeWritebackJob_Run(t *testing.T) {
	job := NewApplyKnowledgeWritebackJob(
		&mockChangeScanner{},
		&mockKnowledgeService{},
	)
	result, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("run writeback job: %v", err)
	}
	if result.AppliedCount != 1 {
		t.Fatalf("got AppliedCount %d, want 1", result.AppliedCount)
	}
}

// --- mocks ---

type mockChangeScanner struct{}

func (m *mockChangeScanner) Scan(ctx context.Context) ([]knowledge.WritebackChange, error) {
	return []knowledge.WritebackChange{
		{
			ObjectType:  "topic",
			ObjectID:    101,
			FieldName:   "manual_tags",
			Value:       []string{"ai监管"},
			SourcePath:  "HotKey/topics/test-101.md",
		},
	}, nil
}

type mockKnowledgeService struct{}

func (m *mockKnowledgeService) ApplyChange(ctx context.Context, change knowledge.WritebackChange, conflict knowledge.ConflictInput) error {
	return nil
}
