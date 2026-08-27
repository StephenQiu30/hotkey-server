package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

func TestShadowRunnerReportsSanitizedComparisonWithoutBlockingSubmit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input struct {
			TaskID string `json:"task_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode shadow request: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"contract_version":"analysis.v1","task_id":"` + input.TaskID + `","task_type":"event_summary","status":"degraded","suggestions":[{"kind":"event_summary","value":{"title_zh":"same","sentences":[]},"confidence":0,"evidence_ids":[],"reason":"safe fallback"}],"runtime":{"name":"deterministic","version":"deterministic.v1","degraded":true}}`))
	}))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	observations := make(chan ShadowObservation, 1)
	runner, err := NewShadowRunner(client, ShadowOptions{Timeout: time.Second, MaxConcurrency: 1, Observe: func(value ShadowObservation) { observations <- value }})
	if err != nil {
		t.Fatal(err)
	}
	request := shadowStructuredRequest()
	primary := intelligencedomain.StructuredResponse{ModelVersion: "deterministic.v1", JSON: json.RawMessage(`{"title_zh":"same","sentences":[]}`)}
	started := time.Now()
	runner.Submit(context.Background(), request, primary)
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("Submit blocked on Agent response")
	}
	select {
	case observation := <-observations:
		if observation.Result != "matched" || !observation.PrimaryJSONValid || !observation.ShadowJSONValid || observation.Dropped || observation.ErrorCode != 0 || observation.DurationMS < 0 || len(observation.PrimaryOutputSHA256) != 64 || len(observation.ShadowOutputSHA256) != 64 {
			t.Fatalf("shadow observation = %#v", observation)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for shadow observation")
	}
}

func TestShadowRunnerMapsAgentFailureAndBoundsConcurrency(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started <- struct{}{}
		<-release
		http.Error(writer, "not exposed", http.StatusBadGateway)
	}))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	observations := make(chan ShadowObservation, 2)
	runner, err := NewShadowRunner(client, ShadowOptions{Timeout: time.Second, MaxConcurrency: 1, Observe: func(value ShadowObservation) { observations <- value }})
	if err != nil {
		t.Fatal(err)
	}
	request := shadowStructuredRequest()
	primary := intelligencedomain.StructuredResponse{ModelVersion: "deterministic.v1", JSON: json.RawMessage(`{"title_zh":"primary","sentences":[]}`)}
	runner.Submit(context.Background(), request, primary)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first shadow request did not start")
	}
	runner.Submit(context.Background(), request, primary)
	select {
	case observation := <-observations:
		if observation.Result != "dropped" || !observation.Dropped {
			t.Fatalf("dropped observation = %#v", observation)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dropped shadow observation")
	}
	close(release)
	select {
	case observation := <-observations:
		if observation.Result != "agent_error" || observation.ErrorCode != intelligencedomain.CodeAIProviderTransient || observation.Dropped {
			t.Fatalf("failure observation = %#v", observation)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Agent failure observation")
	}
}

func TestCanonicalShadowComparisonIgnoresObjectOrderButPreservesLargeNumbers(t *testing.T) {
	if !canonicalJSONEqual([]byte(`{"b":2,"a":1}`), []byte(`{"a":1,"b":2}`)) {
		t.Fatal("equivalent JSON objects were classified as different")
	}
	if canonicalJSONEqual([]byte(`{"id":9007199254740992}`), []byte(`{"id":9007199254740993}`)) {
		t.Fatal("distinct large JSON integers were classified as matched")
	}
}

func shadowStructuredRequest() intelligencedomain.StructuredRequest {
	return intelligencedomain.StructuredRequest{
		ModelName: "agent", ModelVersion: "deterministic.v1", TaskType: intelligencedomain.TaskTypeEventSummary,
		SchemaName: "event-summary-output-v1", SchemaVersion: "v1", Instruction: "return JSON",
		InputSchema: json.RawMessage(`{"type":"object"}`), Schema: json.RawMessage(`{"type":"object"}`), Input: json.RawMessage(`{"event_id":7}`),
	}
}
