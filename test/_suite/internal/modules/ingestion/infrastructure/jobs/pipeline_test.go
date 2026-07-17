package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

func TestPipelineHandlersRejectInvalidEnvelopeBeforeDependencies(t *testing.T) {
	job := queue.Job{Kind: queue.KindEvaluateRelevance, UniqueKey: "x", ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1, Payload: queue.Payload{EntityID: 1, EntityVersion: 1}}
	if err := (&NormalizeHandler{}).Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("normalize invalid kind = %v", err)
	}
	wrongKind := job
	wrongKind.Kind = queue.KindClusterContent
	if err := (&EvaluateHandler{}).Handle(context.Background(), wrongKind); !queue.IsPermanent(err) {
		t.Fatalf("evaluate invalid kind = %v", err)
	}
}
