package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestOpenAIProviderEmbedUsesModelNameAndReturnsLocalVersion(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("request = %s %s, want POST /v1/embeddings", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-only-openai-key" {
			t.Errorf("Authorization = %q, want dummy test key", got)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writeJSON(writer, embeddingAPIResponse("text-embedding-3-large"))
	}))
	t.Cleanup(server.Close)

	provider := newOpenAIProvider(t, server.URL)
	response, err := provider.Embed(context.Background(), intelligencedomain.EmbeddingRequest{
		ModelName:    "text-embedding-3-large",
		ModelVersion: "local-embedding-v7",
		Dimensions:   intelligencedomain.EmbeddingDimensions,
		Inputs:       []string{"first", "second"},
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if response.ModelVersion != "local-embedding-v7" || len(response.Vectors) != 2 || len(response.Vectors[0]) != intelligencedomain.EmbeddingDimensions || len(response.Vectors[1]) != intelligencedomain.EmbeddingDimensions {
		t.Fatalf("Embed() response = %#v, want local version and two 1024-vectors", response)
	}
	if response.Usage != (intelligencedomain.Usage{InputTokens: 1, OutputTokens: 0}) {
		t.Fatalf("Embed() usage = %#v, want prompt/total mapping 1/0", response.Usage)
	}
	if got, want := requestBody["model"], "text-embedding-3-large"; got != want {
		t.Errorf("request model = %#v, want %q", got, want)
	}
	if got, want := requestBody["dimensions"], float64(intelligencedomain.EmbeddingDimensions); got != want {
		t.Errorf("request dimensions = %#v, want %v", got, want)
	}
	if got, want := requestBody["input"], []any{"first", "second"}; !jsonValueEqual(got, want) {
		t.Errorf("request input = %#v, want %#v", got, want)
	}
	assertDoesNotContain(t, requestBody, "local-embedding-v7")
}

func TestOpenAIProviderMapsUsageAndRejectsInconsistentTotals(t *testing.T) {
	t.Run("structured usage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			payload := structuredAPIResponse("gpt-test", `{"terms":["hotkey"]}`)
			payload["usage"] = map[string]any{"input_tokens": 3, "output_tokens": 5, "total_tokens": 8}
			writeJSON(writer, payload)
		}))
		t.Cleanup(server.Close)

		response, err := newOpenAIProvider(t, server.URL).GenerateStructured(context.Background(), structuredRequest())
		if err != nil {
			t.Fatalf("GenerateStructured() error = %v", err)
		}
		if response.Usage != (intelligencedomain.Usage{InputTokens: 3, OutputTokens: 5}) {
			t.Fatalf("GenerateStructured() usage = %#v, want 3/5", response.Usage)
		}
	})

	t.Run("embedding total below prompt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			payload := embeddingAPIResponse("text-embedding-3-large")
			payload["usage"] = map[string]any{"prompt_tokens": 4, "total_tokens": 3}
			writeJSON(writer, payload)
		}))
		t.Cleanup(server.Close)

		_, err := newOpenAIProvider(t, server.URL).Embed(context.Background(), embeddingRequest())
		assertCode(t, err, intelligencedomain.CodeAIModelProfileInvalid)
	})

	t.Run("structured total mismatch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			payload := structuredAPIResponse("gpt-test", `{"terms":["hotkey"]}`)
			payload["usage"] = map[string]any{"input_tokens": 2, "output_tokens": 3, "total_tokens": 4}
			writeJSON(writer, payload)
		}))
		t.Cleanup(server.Close)

		_, err := newOpenAIProvider(t, server.URL).GenerateStructured(context.Background(), structuredRequest())
		assertCode(t, err, intelligencedomain.CodeAIModelProfileInvalid)
	})
}

func TestOpenAIProviderRejectsEmbeddingModelMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, embeddingAPIResponse("unexpected-model"))
	}))
	t.Cleanup(server.Close)

	response, err := newOpenAIProvider(t, server.URL).Embed(context.Background(), embeddingRequest())
	if response.ModelVersion != "" || response.Vectors != nil {
		t.Fatalf("Embed() response = %#v, want zero result on model mismatch", response)
	}
	assertCode(t, err, intelligencedomain.CodeAIModelProfileInvalid)
}

func TestOpenAIProviderStructuredRequestUsesStrictSchemaAndBoundedRepair(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			t.Errorf("request = %s %s, want POST /v1/responses", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		body := map[string]any{}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		requestBody <- body
		writeJSON(writer, structuredAPIResponse("gpt-test", `{"terms":["hotkey"]}`))
	}))
	t.Cleanup(server.Close)

	response, err := newOpenAIProvider(t, server.URL).GenerateStructured(context.Background(), structuredRequest())
	if err != nil {
		t.Fatalf("GenerateStructured() error = %v", err)
	}
	if response.ModelVersion != "term-expansion-v3" || string(response.JSON) != `{"terms":["hotkey"]}` {
		t.Fatalf("GenerateStructured() response = %#v, want local model version and JSON result", response)
	}

	body := <-requestBody
	if got, want := body["model"], "gpt-test"; got != want {
		t.Errorf("request model = %#v, want %q", got, want)
	}
	if got, want := body["instructions"], "Return only approved terms."; got != want {
		t.Errorf("request instructions = %#v, want %q", got, want)
	}
	text, ok := body["text"].(map[string]any)
	if !ok {
		t.Fatalf("request text = %#v, want object", body["text"])
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		t.Fatalf("request text.format = %#v, want object", text["format"])
	}
	if got, want := format["type"], "json_schema"; got != want {
		t.Errorf("request format type = %#v, want %q", got, want)
	}
	if got, want := format["name"], "term_expansion_output"; got != want {
		t.Errorf("request schema name = %#v, want %q", got, want)
	}
	if got, want := format["strict"], true; got != want {
		t.Errorf("request schema strict = %#v, want %v", got, want)
	}
	wantSchema := map[string]any{"additionalProperties": false, "properties": map[string]any{"terms": map[string]any{"type": "array"}}, "type": "object"}
	if !jsonValueEqual(format["schema"], wantSchema) {
		t.Errorf("request schema = %#v, want %#v", format["schema"], wantSchema)
	}

	input, ok := body["input"].(string)
	if !ok {
		t.Fatalf("request input = %#v, want JSON string", body["input"])
	}
	var repairPayload struct {
		Input  json.RawMessage `json:"input"`
		Repair struct {
			PreviousOutput json.RawMessage `json:"previous_output"`
			Violations     []struct {
				InstancePath string `json:"instance_path"`
				Keyword      string `json:"keyword"`
			} `json:"violations"`
		} `json:"repair"`
	}
	if err := json.Unmarshal([]byte(input), &repairPayload); err != nil {
		t.Fatalf("decode repair payload: %v", err)
	}
	if string(repairPayload.Input) != `{"query":"openai"}` || string(repairPayload.Repair.PreviousOutput) != `{"terms":"bad"}` || len(repairPayload.Repair.Violations) != 1 || repairPayload.Repair.Violations[0].InstancePath != "/terms" || repairPayload.Repair.Violations[0].Keyword != "type" {
		t.Errorf("repair payload = %#v, want exact bounded repair content", repairPayload)
	}
	assertDoesNotContain(t, body, "term-expansion-v3")
}

func TestOpenAIProviderRejectsStructuredModelMismatchAndInvalidJSON(t *testing.T) {
	t.Run("model mismatch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writeJSON(writer, structuredAPIResponse("unexpected-model", `{"terms":["hotkey"]}`))
		}))
		t.Cleanup(server.Close)

		response, err := newOpenAIProvider(t, server.URL).GenerateStructured(context.Background(), structuredRequest())
		if response.ModelVersion != "" || response.JSON != nil {
			t.Fatalf("GenerateStructured() response = %#v, want zero result", response)
		}
		assertCode(t, err, intelligencedomain.CodeAIModelProfileInvalid)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writeJSON(writer, structuredAPIResponse("gpt-test", "not json"))
		}))
		t.Cleanup(server.Close)

		response, err := newOpenAIProvider(t, server.URL).GenerateStructured(context.Background(), structuredRequest())
		if response.ModelVersion != "" || response.JSON != nil {
			t.Fatalf("GenerateStructured() response = %#v, want zero result", response)
		}
		assertCode(t, err, intelligencedomain.CodeAIOutputInvalid)
	})
}

func TestOpenAIProviderMapsTransportFailuresWithoutLeakingProviderBodies(t *testing.T) {
	for _, testCase := range []struct {
		name string
		http int
		code int
	}{
		{name: "rate limited", http: http.StatusTooManyRequests, code: intelligencedomain.CodeAIProviderRateLimited},
		{name: "transient", http: http.StatusServiceUnavailable, code: intelligencedomain.CodeAIProviderTransient},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(testCase.http)
				_, _ = writer.Write([]byte(`{"error":{"message":"provider diagnostic must never cross boundary"}}`))
			}))
			t.Cleanup(server.Close)

			_, err := newOpenAIProvider(t, server.URL).Embed(context.Background(), embeddingRequest())
			assertCode(t, err, testCase.code)
			if strings.Contains(err.Error(), "provider diagnostic") {
				t.Fatalf("mapped error leaks provider body: %v", err)
			}
		})
	}

	t.Run("deadline", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			select {
			case <-request.Context().Done():
			case <-time.After(100 * time.Millisecond):
			}
		}))
		t.Cleanup(server.Close)

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		_, err := newOpenAIProvider(t, server.URL).Embed(ctx, embeddingRequest())
		assertCode(t, err, intelligencedomain.CodeAIProviderTimeout)
	})
}

// newOpenAIProvider is an internal package test seam. Production construction
// exposes no SDK options or endpoint override.
func newOpenAIProvider(t *testing.T, baseURL string) *OpenAIProvider {
	t.Helper()
	return &OpenAIProvider{client: openai.NewClient(
		option.WithBaseURL(baseURL+"/v1"),
		option.WithAPIKey("test-only-openai-key"),
		option.WithMaxRetries(0),
	)}
}

func embeddingRequest() intelligencedomain.EmbeddingRequest {
	return intelligencedomain.EmbeddingRequest{
		ModelName:    "text-embedding-3-large",
		ModelVersion: "local-embedding-v7",
		Dimensions:   intelligencedomain.EmbeddingDimensions,
		Inputs:       []string{"one input"},
	}
}

func structuredRequest() intelligencedomain.StructuredRequest {
	return intelligencedomain.StructuredRequest{
		ModelName:     "gpt-test",
		ModelVersion:  "term-expansion-v3",
		SchemaName:    "term_expansion_output",
		SchemaVersion: "v1",
		TaskType:      intelligencedomain.TaskTypeTermExpansion,
		Instruction:   "Return only approved terms.",
		Schema:        json.RawMessage(`{"type":"object","properties":{"terms":{"type":"array"}},"additionalProperties":false}`),
		Input:         json.RawMessage(`{"query":"openai"}`),
		Repair: &intelligencedomain.RepairInput{
			PreviousOutput: json.RawMessage(`{"terms":"bad"}`),
			Violations:     []intelligencedomain.SchemaViolation{{InstancePath: "/terms", Keyword: "type"}},
		},
	}
}

func embeddingAPIResponse(model string) map[string]any {
	vector := make([]float64, intelligencedomain.EmbeddingDimensions)
	return map[string]any{
		"object": "list",
		"model":  model,
		"data": []map[string]any{
			{"object": "embedding", "index": 0, "embedding": vector},
			{"object": "embedding", "index": 1, "embedding": vector},
		},
		"usage": map[string]any{"prompt_tokens": 1, "total_tokens": 1},
	}
}

func structuredAPIResponse(model, output string) map[string]any {
	return map[string]any{
		"id":     "resp_test",
		"model":  model,
		"status": "completed",
		"output": []map[string]any{{
			"id":      "msg_test",
			"type":    "message",
			"role":    "assistant",
			"status":  "completed",
			"content": []map[string]any{{"type": "output_text", "text": output, "annotations": []any{}}},
		}},
	}
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func assertCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %d", want)
	}
	got, ok := intelligencedomain.CodeOf(err)
	if !ok || got != want {
		t.Fatalf("error code = %d, %v, want %d", got, err, want)
	}
}

func assertDoesNotContain(t *testing.T, value any, forbidden string) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	if strings.Contains(string(payload), forbidden) {
		t.Fatalf("serialized value contains forbidden local metadata %q: %s", forbidden, payload)
	}
}

func jsonValueEqual(got, want any) bool {
	gotJSON, gotErr := json.Marshal(got)
	wantJSON, wantErr := json.Marshal(want)
	return gotErr == nil && wantErr == nil && string(gotJSON) == string(wantJSON)
}
