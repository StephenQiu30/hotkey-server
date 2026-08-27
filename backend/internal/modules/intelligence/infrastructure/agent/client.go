package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

const (
	ContractVersion        = "analysis.v1"
	StatusSucceeded        = "succeeded"
	StatusDegraded         = "degraded"
	maximumRequestBytes    = 256 << 10
	maximumEvidenceItems   = 32
	maximumSuggestionItems = 32
)

type TaskType string

const (
	TaskMonitorCompile TaskType = "monitor_compile"
	TaskRelevance      TaskType = "relevance"
	TaskEventCluster   TaskType = "event_cluster"
	TaskClaimEvidence  TaskType = "claim_evidence"
	TaskEventSummary   TaskType = "event_summary"
)

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
	hashPattern       = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Options struct {
	BaseURL          string
	AuthToken        string
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

type Client struct {
	endpoint         string
	authToken        string
	httpClient       *http.Client
	maxResponseBytes int64
}

type Evidence struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type AnalyzeRequest struct {
	TaskID          string
	TaskType        TaskType
	InputHash       string
	EvidenceSetHash string
	Payload         json.RawMessage
	Evidence        []Evidence
}

type Suggestion struct {
	Kind        string          `json:"kind"`
	Value       json.RawMessage `json:"value"`
	Confidence  float64         `json:"confidence"`
	EvidenceIDs []string        `json:"evidence_ids"`
	Reason      string          `json:"reason"`
}

type RuntimeInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Degraded bool   `json:"degraded"`
}

type AnalyzeResponse struct {
	ContractVersion string       `json:"contract_version"`
	TaskID          string       `json:"task_id"`
	TaskType        TaskType     `json:"task_type"`
	Status          string       `json:"status"`
	Suggestions     []Suggestion `json:"suggestions"`
	Runtime         RuntimeInfo  `json:"runtime"`
}

type analyzeWireRequest struct {
	ContractVersion string          `json:"contract_version"`
	TaskID          string          `json:"task_id"`
	TaskType        TaskType        `json:"task_type"`
	InputHash       string          `json:"input_hash"`
	EvidenceSetHash string          `json:"evidence_set_hash"`
	Payload         json.RawMessage `json:"payload"`
	Evidence        []Evidence      `json:"evidence"`
}

func NewClient(options Options) (*Client, error) {
	baseURL := strings.TrimSpace(options.BaseURL)
	token := strings.TrimSpace(options.AuthToken)
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || len([]byte(token)) < 32 || options.MaxResponseBytes <= 0 || options.MaxResponseBytes > 8<<20 {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{
		endpoint:         baseURL + "/v1/analyze",
		authToken:        token,
		httpClient:       &clientCopy,
		maxResponseBytes: options.MaxResponseBytes,
	}, nil
}

func (client *Client) Analyze(ctx context.Context, input AnalyzeRequest) (AnalyzeResponse, error) {
	if client == nil || client.httpClient == nil || validateRequest(input) != nil {
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	payload, err := json.Marshal(analyzeWireRequest{
		ContractVersion: ContractVersion,
		TaskID:          input.TaskID,
		TaskType:        input.TaskType,
		InputHash:       input.InputHash,
		EvidenceSetHash: input.EvidenceSetHash,
		Payload:         append(json.RawMessage(nil), input.Payload...),
		Evidence:        append([]Evidence(nil), input.Evidence...),
	})
	if err != nil || len(payload) > maximumRequestBytes {
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(payload))
	if err != nil {
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-HotKey-Agent-Token", client.authToken)

	response, err := client.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
		}
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, client.maxResponseBytes+1))
		return AnalyzeResponse{}, intelligencedomain.NewError(statusCode(response.StatusCode))
	}
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, client.maxResponseBytes+1))
	if err != nil || int64(len(responsePayload)) > client.maxResponseBytes {
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	var output AnalyzeResponse
	decoder := json.NewDecoder(bytes.NewReader(responsePayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	if !validResponse(input, output) {
		return AnalyzeResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	return output, nil
}

func validateRequest(input AnalyzeRequest) error {
	if !identifierPattern.MatchString(input.TaskID) || !input.TaskType.valid() || !hashPattern.MatchString(input.InputHash) || !hashPattern.MatchString(input.EvidenceSetHash) || !validJSONObject(input.Payload) || len(input.Evidence) > maximumEvidenceItems {
		return errors.New("invalid request")
	}
	seen := make(map[string]struct{}, len(input.Evidence))
	for _, evidence := range input.Evidence {
		if !identifierPattern.MatchString(evidence.ID) || len(strings.TrimSpace(evidence.Title)) == 0 || len(evidence.Title) > 500 || len(strings.TrimSpace(evidence.Text)) == 0 || len(evidence.Text) > 20_000 {
			return errors.New("invalid evidence")
		}
		if _, duplicate := seen[evidence.ID]; duplicate {
			return errors.New("duplicate evidence")
		}
		seen[evidence.ID] = struct{}{}
	}
	return nil
}

func validResponse(input AnalyzeRequest, output AnalyzeResponse) bool {
	if output.ContractVersion != ContractVersion || output.TaskID != input.TaskID || output.TaskType != input.TaskType || (output.Status != StatusSucceeded && output.Status != StatusDegraded) || !identifierPattern.MatchString(output.Runtime.Name) || len(output.Runtime.Version) == 0 || len(output.Runtime.Version) > 32 || (output.Status == StatusDegraded) != output.Runtime.Degraded || len(output.Suggestions) > maximumSuggestionItems {
		return false
	}
	allowedEvidence := make(map[string]struct{}, len(input.Evidence))
	for _, evidence := range input.Evidence {
		allowedEvidence[evidence.ID] = struct{}{}
	}
	for _, suggestion := range output.Suggestions {
		if suggestion.Kind != string(input.TaskType) || !validJSONObject(suggestion.Value) || math.IsNaN(suggestion.Confidence) || math.IsInf(suggestion.Confidence, 0) || suggestion.Confidence < 0 || suggestion.Confidence > 1 || len(strings.TrimSpace(suggestion.Reason)) == 0 || len(suggestion.Reason) > 1000 || len(suggestion.EvidenceIDs) > maximumEvidenceItems {
			return false
		}
		seenEvidence := make(map[string]struct{}, len(suggestion.EvidenceIDs))
		for _, evidenceID := range suggestion.EvidenceIDs {
			if _, allowed := allowedEvidence[evidenceID]; !allowed {
				return false
			}
			if _, duplicate := seenEvidence[evidenceID]; duplicate {
				return false
			}
			seenEvidence[evidenceID] = struct{}{}
		}
	}
	return true
}

func validJSONObject(value json.RawMessage) bool {
	normalized := bytes.TrimSpace(value)
	return len(normalized) > 0 && normalized[0] == '{' && json.Valid(normalized)
}

func (task TaskType) valid() bool {
	return task == TaskMonitorCompile || task == TaskRelevance || task == TaskEventCluster || task == TaskClaimEvidence || task == TaskEventSummary
}

func statusCode(value int) int {
	switch {
	case value == http.StatusUnauthorized || value == http.StatusForbidden || value == http.StatusServiceUnavailable:
		return intelligencedomain.CodeAIModelUnavailable
	case value == http.StatusTooManyRequests:
		return intelligencedomain.CodeAIProviderRateLimited
	case value == http.StatusRequestTimeout || value == http.StatusGatewayTimeout:
		return intelligencedomain.CodeAIProviderTimeout
	case value >= 500:
		return intelligencedomain.CodeAIProviderTransient
	case value == http.StatusBadRequest || value == http.StatusRequestEntityTooLarge || value == http.StatusUnprocessableEntity:
		return intelligencedomain.CodeAIModelProfileInvalid
	default:
		return intelligencedomain.CodeAIOutputInvalid
	}
}
