package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	alertapplication "github.com/StephenQiu30/hotkey-server/internal/modules/alert/application"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type evaluatorFake struct {
	refs   []alertapplication.EventUpdateRef
	result alertapplication.EvaluationResult
	err    error
}

func (fake *evaluatorFake) Evaluate(_ context.Context, ref alertapplication.EventUpdateRef) (alertapplication.EvaluationResult, error) {
	fake.refs = append(fake.refs, ref)
	return fake.result, fake.err
}

var _ AlertEvaluator = (*evaluatorFake)(nil)

func TestEvaluateJobUsesOnlyTheStableUpdateEnvelope(t *testing.T) {
	t.Parallel()
	ref := alertapplication.EventUpdateRef{ID: 41, Version: 3, EvidenceSetHash: strings.Repeat("a", 64)}
	scheduledAt := time.Date(2026, 8, 4, 5, 0, 0, 0, time.UTC)
	first, err := NewEvaluateJob(ref, scheduledAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEvaluateJob(ref, scheduledAt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Kind != queue.KindEvaluateEventAlerts || first.UniqueKey == "" {
		t.Fatalf("evaluate jobs are not stable: %#v / %#v", first, second)
	}
	encoded, err := json.Marshal(first.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 3 || payload["entity_id"] != float64(ref.ID) || payload["entity_version"] != float64(ref.Version) || payload["input_hash"] != ref.EvidenceSetHash {
		t.Fatalf("payload = %s, want only update id/version/hash", encoded)
	}
	for _, forbidden := range []string{"title", "summary", "body", "provider", "monitor", "score"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("payload leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestEvaluateHandlerTreatsRepositoryDuplicatesAsSuccess(t *testing.T) {
	t.Parallel()
	fake := &evaluatorFake{result: alertapplication.EvaluationResult{CandidateCount: 1, EligibleCount: 1, DuplicateCount: 1}}
	handler := NewEvaluateHandler(fake)
	job, err := NewEvaluateJob(alertapplication.EventUpdateRef{ID: 41, Version: 3, EvidenceSetHash: strings.Repeat("a", 64)}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("duplicate delivery error = %v", err)
	}
	if len(fake.refs) != 2 || fake.refs[0] != fake.refs[1] {
		t.Fatalf("replayed refs = %#v", fake.refs)
	}
}

func TestEvaluateHandlerHasNoSummaryDependencyAndClassifiesFailures(t *testing.T) {
	t.Parallel()
	job, err := NewEvaluateJob(alertapplication.EventUpdateRef{ID: 41, Version: 3, EvidenceSetHash: strings.Repeat("a", 64)}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	fake := &evaluatorFake{err: sharedrepository.ErrUnavailable}
	handler := NewEvaluateHandler(fake)
	if err := handler.Handle(context.Background(), job); !queue.IsRetryable(err) || !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("dependency failure = %v, want retryable unavailable", err)
	}
	wrongKind := job
	wrongKind.Kind = queue.KindGenerateEventSummary
	if err := handler.Handle(context.Background(), wrongKind); !queue.IsPermanent(err) {
		t.Fatalf("wrong-kind failure = %v, want permanent", err)
	}
	if len(fake.refs) != 1 {
		t.Fatalf("wrong-kind job reached evaluator; refs = %#v", fake.refs)
	}
}
