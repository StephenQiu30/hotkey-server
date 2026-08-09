package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGenerateSourceDocumentJobUsesOnlySemanticDurableArgs(t *testing.T) {
	t.Parallel()

	if KindGenerateSourceDocument != "generate_source_document" || !IsKnownKind(KindGenerateSourceDocument) {
		t.Fatalf("source document kind = %q known=%t", KindGenerateSourceDocument, IsKnownKind(KindGenerateSourceDocument))
	}
	args := json.RawMessage(`{"evidence_reference_id":71,"trace_id":"0123456789abcdef0123456789abcdef"}`)
	job := Job{
		Kind: KindGenerateSourceDocument, UniqueKey: "source-document-71", DurableArgs: args,
		ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 3,
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	encoded, err := job.encodedArgs()
	if err != nil {
		t.Fatalf("encodedArgs() error = %v", err)
	}
	if string(encoded) != string(args) || strings.Contains(string(encoded), "entity_id") || strings.Contains(string(encoded), "body") || strings.Contains(string(encoded), "payload") {
		t.Fatalf("durable args = %s", encoded)
	}
}

func TestJobKeepsGenericPayloadAndSemanticDurableArgsMutuallyExclusive(t *testing.T) {
	t.Parallel()

	base := Job{
		Kind: KindGenerateSourceDocument, UniqueKey: "source-document-71",
		ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 3,
	}
	validArgs := json.RawMessage(`{"evidence_reference_id":71,"trace_id":"0123456789abcdef0123456789abcdef"}`)
	tests := []struct {
		name string
		job  Job
	}{
		{name: "semantic kind without durable args", job: base},
		{name: "semantic kind with generic payload", job: func() Job {
			job := base
			job.Payload = Payload{EntityID: 71, EntityVersion: 1}
			return job
		}()},
		{name: "semantic kind with both shapes", job: func() Job {
			job := base
			job.Payload = Payload{EntityID: 71, EntityVersion: 1}
			job.DurableArgs = validArgs
			return job
		}()},
		{name: "generic kind with semantic args", job: func() Job {
			job := base
			job.Kind = KindCollectSource
			job.DurableArgs = validArgs
			return job
		}()},
		{name: "invalid JSON", job: func() Job {
			job := base
			job.DurableArgs = json.RawMessage(`{"evidence_reference_id":`)
			return job
		}()},
		{name: "array instead of one object", job: func() Job {
			job := base
			job.DurableArgs = json.RawMessage(`[{"evidence_reference_id":71}]`)
			return job
		}()},
		{name: "unknown kind", job: func() Job {
			job := base
			job.Kind = "generate_unknown_document"
			job.DurableArgs = validArgs
			return job
		}()},
		{name: "unbounded args", job: func() Job {
			job := base
			job.DurableArgs = json.RawMessage(`{"trace_id":"` + strings.Repeat("x", MaxPayloadBytes) + `"}`)
			return job
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.job.Validate(); err == nil {
				t.Fatal("Validate() accepted an incompatible durable-args shape")
			}
		})
	}
}

func TestAnalyzeMonitorIntentUsesSemanticDurableArgs(t *testing.T) {
	t.Parallel()
	if KindAnalyzeMonitorIntent != "analyze_monitor_intent" || !IsKnownKind(KindAnalyzeMonitorIntent) {
		t.Fatalf("intent analysis kind = %q known=%t", KindAnalyzeMonitorIntent, IsKnownKind(KindAnalyzeMonitorIntent))
	}
	args := json.RawMessage(`{"run_id":41,"kind":"preview","monitor_id":7,"draft_id":11,"draft_resource_version":3,"input_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","profile_version":"preview-v2","sample_limit":25}`)
	job := Job{
		Kind: KindAnalyzeMonitorIntent, UniqueKey: "monitor-intent-request",
		DurableArgs: args, ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 3,
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	encoded, err := job.encodedArgs()
	if err != nil || string(encoded) != string(args) {
		t.Fatalf("encodedArgs() = %s / %v", encoded, err)
	}
}
