package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

const (
	testToken = "test-agent-secret-0123456789abcdef0123456789abcdef"
	testHash  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestClientSendsVersionedAuthenticatedBoundedAnalysisRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/analyze" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-HotKey-Agent-Token") != testToken || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request headers = %#v", request.Header)
		}
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["contract_version"] != ContractVersion || input["task_type"] != string(TaskRelevance) {
			t.Fatalf("request contract = %#v", input)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"contract_version":"analysis.v1","task_id":"run-42","task_type":"relevance","status":"degraded","suggestions":[{"kind":"relevance","value":{"score":0.5},"confidence":0,"evidence_ids":["evidence-1"],"reason":"deterministic fallback"}],"runtime":{"name":"deterministic","version":"deterministic.v1","degraded":true}}`))
	}))
	defer server.Close()

	client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Analyze(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if response.Status != StatusDegraded || !response.Runtime.Degraded || len(response.Suggestions) != 1 {
		t.Fatalf("Analyze() = %#v", response)
	}
}

func TestClientRejectsInvalidConfigurationAndRequestBeforeNetwork(t *testing.T) {
	for _, options := range []Options{
		{},
		{BaseURL: "ftp://agent", AuthToken: testToken, MaxResponseBytes: 1024},
		{BaseURL: "http://user:pass@agent", AuthToken: testToken, MaxResponseBytes: 1024},
		{BaseURL: "http://agent/path", AuthToken: testToken, MaxResponseBytes: 1024},
		{BaseURL: "http://agent", AuthToken: "short", MaxResponseBytes: 1024},
		{BaseURL: "http://agent", AuthToken: testToken, MaxResponseBytes: 0},
	} {
		if _, err := NewClient(options); errorCode(err) != intelligencedomain.CodeAIModelProfileInvalid {
			t.Errorf("NewClient(%#v) error = %v", options, err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid request reached network")
	}))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest()
	request.InputHash = "not-a-hash"
	if _, err := client.Analyze(context.Background(), request); errorCode(err) != intelligencedomain.CodeAIModelProfileInvalid {
		t.Fatalf("Analyze() error = %v", err)
	}
	request = validRequest()
	request.Payload = json.RawMessage(`[]`)
	if _, err := client.Analyze(context.Background(), request); errorCode(err) != intelligencedomain.CodeAIModelProfileInvalid {
		t.Fatalf("Analyze(array payload) error = %v", err)
	}
	request = validRequest()
	request.Evidence = append(request.Evidence, request.Evidence[0])
	if _, err := client.Analyze(context.Background(), request); errorCode(err) != intelligencedomain.CodeAIModelProfileInvalid {
		t.Fatalf("Analyze(duplicate evidence) error = %v", err)
	}
}

func TestClientMapsStatusTimeoutAndMalformedResponsesToStableErrors(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantCode   int
		wantSecret string
	}{
		{name: "unauthorized", wantCode: intelligencedomain.CodeAIModelUnavailable, wantSecret: "provider secret", handler: func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, `{"error":{"code":"AGENT_UNAUTHORIZED","message":"provider secret"}}`, http.StatusUnauthorized)
		}},
		{name: "rate limited", wantCode: intelligencedomain.CodeAIProviderRateLimited, handler: func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "raw upstream", http.StatusTooManyRequests)
		}},
		{name: "transient", wantCode: intelligencedomain.CodeAIProviderTransient, handler: func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "raw upstream", http.StatusBadGateway)
		}},
		{name: "malformed", wantCode: intelligencedomain.CodeAIOutputInvalid, handler: func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"contract_version":"analysis.v1","unknown":true}`))
		}},
		{name: "oversized", wantCode: intelligencedomain.CodeAIOutputInvalid, handler: func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(strings.Repeat("x", 2048)))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 1024})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Analyze(context.Background(), validRequest())
			containsSecret := test.wantSecret != "" && strings.Contains(err.Error(), test.wantSecret)
			if errorCode(err) != test.wantCode || containsSecret || strings.Contains(err.Error(), "raw upstream") {
				t.Fatalf("Analyze() error = %v, want redacted code %d", err, test.wantCode)
			}
		})
	}

	timeoutServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = writer.Write([]byte(`{"status":"late"}`))
	}))
	defer timeoutServer.Close()
	client, err := NewClient(Options{BaseURL: timeoutServer.URL, AuthToken: testToken, HTTPClient: timeoutServer.Client(), MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := client.Analyze(ctx, validRequest()); errorCode(err) != intelligencedomain.CodeAIProviderTimeout {
		t.Fatalf("Analyze(timeout) error = %v", err)
	}
}

func TestClientRejectsIdentityAndEvidenceForgery(t *testing.T) {
	forgedResponses := []string{
		`{"contract_version":"analysis.v1","task_id":"other-run","task_type":"relevance","status":"degraded","suggestions":[],"runtime":{"name":"deterministic","version":"v1","degraded":true}}`,
		`{"contract_version":"analysis.v1","task_id":"run-42","task_type":"relevance","status":"degraded","suggestions":[{"kind":"relevance","value":{},"confidence":0,"evidence_ids":["forged"],"reason":"invalid"}],"runtime":{"name":"deterministic","version":"v1","degraded":true}}`,
		`{"contract_version":"analysis.v1","task_id":"run-42","task_type":"relevance","status":"degraded","suggestions":[{"kind":"relevance","value":[],"confidence":0,"evidence_ids":["evidence-1"],"reason":"wrong shape"}],"runtime":{"name":"deterministic","version":"v1","degraded":true}}`,
		`{"contract_version":"analysis.v1","task_id":"run-42","task_type":"relevance","status":"degraded","suggestions":[{"kind":"event_cluster","value":{},"confidence":0,"evidence_ids":["evidence-1"],"reason":"wrong task"}],"runtime":{"name":"deterministic","version":"v1","degraded":true}}`,
		`{"contract_version":"analysis.v1","task_id":"run-42","task_type":"relevance","status":"succeeded","suggestions":[],"runtime":{"name":"deterministic","version":"v1","degraded":true}}`,
		`{"contract_version":"analysis.v1","task_id":"run-42","task_type":"relevance","status":"degraded","suggestions":[{"kind":"relevance","value":{},"confidence":0,"evidence_ids":["evidence-1","evidence-1"],"reason":"duplicate evidence"}],"runtime":{"name":"deterministic","version":"v1","degraded":true}}`,
	}
	for _, payload := range forgedResponses {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(payload))
		}))
		client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 4096})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Analyze(context.Background(), validRequest())
		server.Close()
		if errorCode(err) != intelligencedomain.CodeAIOutputInvalid {
			t.Fatalf("Analyze(forged) error = %v", err)
		}
	}
}

func TestClientAdaptsStructuredRequestsToVersionedAgentPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input struct {
			TaskID   string `json:"task_id"`
			TaskType string `json:"task_type"`
			Payload  struct {
				SchemaName string `json:"schema_name"`
				Input      struct {
					Evidence []map[string]any `json:"evidence"`
				} `json:"input"`
			} `json:"payload"`
			Evidence []Evidence `json:"evidence"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.TaskType != string(TaskEventSummary) || input.Payload.SchemaName != "event-summary-output-v1" || len(input.Payload.Input.Evidence) != 1 || len(input.Evidence) != 1 {
			t.Fatalf("structured Agent request = %#v", input)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(fmt.Sprintf(`{"contract_version":"analysis.v1","task_id":%q,"task_type":"event_summary","status":"degraded","suggestions":[{"kind":"event_summary","value":{"title_zh":"待分析事件","sentences":[]},"confidence":0,"evidence_ids":[],"reason":"safe fallback"}],"runtime":{"name":"deterministic","version":"deterministic.v1","degraded":true}}`, input.TaskID)))
	}))
	defer server.Close()

	client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.GenerateStructured(context.Background(), intelligencedomain.StructuredRequest{
		ModelName: "agent", ModelVersion: "deterministic.v1", TaskType: intelligencedomain.TaskTypeEventSummary,
		SchemaName: "event-summary-output-v1", SchemaVersion: "v1", Instruction: "return JSON",
		Schema: json.RawMessage(`{"type":"object"}`), Input: json.RawMessage(`{"evidence":[{"content_id":17,"locator":"body:1","excerpt":"trusted"}]}`),
	})
	if err != nil || string(response.JSON) != `{"title_zh":"待分析事件","sentences":[]}` || response.ModelVersion != "deterministic.v1" {
		t.Fatalf("GenerateStructured() = %#v / %v", response, err)
	}
}

func TestClientDoesNotPretendToProvideEmbeddings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("embedding request reached Agent")
	}))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, AuthToken: testToken, HTTPClient: server.Client(), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Embed(context.Background(), intelligencedomain.EmbeddingRequest{ModelName: "x", ModelVersion: "v", Dimensions: intelligencedomain.EmbeddingDimensions, Inputs: []string{"x"}})
	if errorCode(err) != intelligencedomain.CodeAIModelUnavailable {
		t.Fatalf("Embed() error = %v", err)
	}
}

func validRequest() AnalyzeRequest {
	return AnalyzeRequest{
		TaskID: "run-42", TaskType: TaskRelevance, InputHash: testHash, EvidenceSetHash: testHash,
		Payload:  json.RawMessage(`{"query":"HotKey"}`),
		Evidence: []Evidence{{ID: "evidence-1", Title: "HotKey", Text: "analysis"}},
	}
}

func errorCode(err error) int {
	code, _ := intelligencedomain.CodeOf(err)
	return code
}
