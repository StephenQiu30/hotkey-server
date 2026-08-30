package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	reportVersion       = "hotkey-source-live-smoke-v1"
	defaultBaseURL      = "http://127.0.0.1:8866"
	defaultQuery        = "artificial intelligence"
	defaultEnvironment  = "local"
	maxResponseBytes    = 1 << 20
	defaultRequestLimit = 60 * time.Second
	totalExecutionLimit = 5 * time.Minute
)

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type config struct {
	baseURL     *url.URL
	token       string
	query       string
	environment string
	revision    string
	outputPath  string
	sources     []sourceInput
}

type sourceInput struct {
	SourceType string
	ID         int64
}

type report struct {
	Version     string         `json:"version"`
	Status      string         `json:"status"`
	Approval    string         `json:"approval"`
	Environment string         `json:"environment"`
	Revision    string         `json:"revision"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  time.Time      `json:"finished_at"`
	QueryBytes  int            `json:"query_bytes"`
	ErrorCode   string         `json:"error_code,omitempty"`
	Sources     []sourceResult `json:"sources"`
}

type sourceResult struct {
	SourceType           string       `json:"source_type"`
	SourceConnectionID   int64        `json:"source_connection_id"`
	Enabled              bool         `json:"enabled"`
	CredentialConfigured bool         `json:"credential_configured"`
	Preflight            probeResult  `json:"preflight"`
	Health               healthResult `json:"health"`
	Search               searchResult `json:"search"`
}

type probeResult struct {
	Passed     bool   `json:"passed"`
	DurationMS int64  `json:"duration_ms"`
	ErrorCode  string `json:"error_code,omitempty"`
}

type healthResult struct {
	Healthy    bool   `json:"healthy"`
	DurationMS int64  `json:"duration_ms"`
	ErrorCode  string `json:"error_code,omitempty"`
}

type searchResult struct {
	State       string `json:"state"`
	ResultCount int    `json:"result_count"`
	DurationMS  int64  `json:"duration_ms"`
	ErrorCode   string `json:"error_code,omitempty"`
}

type apiEnvelope[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

type sourceResponse struct {
	ID                   int64  `json:"id"`
	SourceType           string `json:"source_type"`
	Enabled              bool   `json:"enabled"`
	CredentialConfigured bool   `json:"credential_configured"`
	Deleted              bool   `json:"deleted"`
}

type sourcePageResponse struct {
	Items      []sourceResponse `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

type sourceHealthResponse struct {
	Healthy   bool   `json:"healthy"`
	ErrorCode string `json:"error_code"`
}

type instantSearchResponse struct {
	SourceStatuses []instantSearchStatus `json:"source_statuses"`
}

type instantSearchStatus struct {
	SourceType  string `json:"source_type"`
	State       string `json:"state"`
	ResultCount int    `json:"result_count"`
	ErrorCode   string `json:"error_code"`
}

type codedError struct{ code string }

func (err *codedError) Error() string { return err.code }

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "source live smoke configuration rejected: %s\n", err)
		os.Exit(2)
	}
	client := &http.Client{
		Timeout: defaultRequestLimit,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), totalExecutionLimit)
	defer cancel()
	result, passed := execute(ctx, client, cfg, time.Now)
	if err := writeReport(cfg.outputPath, result); err != nil {
		fmt.Fprintf(os.Stderr, "source live smoke report rejected: %s\n", err)
		os.Exit(2)
	}
	if !passed {
		fmt.Fprintln(os.Stderr, "source live smoke failed; inspect the sanitized report")
		os.Exit(1)
	}
	fmt.Printf("source live smoke passed: %s\n", cfg.outputPath)
}

func loadConfig(getenv func(string) string) (config, error) {
	baseURL, err := validateBaseURL(defaultBaseURL)
	if err != nil {
		return config{}, err
	}
	token := strings.TrimSpace(getenv("HOTKEY_SOURCE_LIVE_SMOKE_ADMIN_TOKEN"))
	if len(token) < 16 || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return config{}, errors.New("HOTKEY_SOURCE_LIVE_SMOKE_ADMIN_TOKEN is invalid")
	}
	query := defaultQuery
	if query == "" || utf8.RuneCountInString(query) > 200 || len(query) > 1024 {
		return config{}, errors.New("source live smoke query is invalid")
	}
	environment := defaultEnvironment
	if !safeIdentifier.MatchString(environment) {
		return config{}, errors.New("source live smoke environment is invalid")
	}
	revision := currentRevision()
	if !safeIdentifier.MatchString(revision) {
		return config{}, errors.New("source live smoke revision is invalid")
	}
	outputValue := filepath.Join(os.TempDir(), "hotkey-source-live-smoke-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
	outputPath, err := validateOutputPath(outputValue)
	if err != nil {
		return config{}, err
	}
	return config{baseURL: baseURL, token: token, query: query, environment: environment, revision: revision, outputPath: outputPath}, nil
}

func currentRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && safeIdentifier.MatchString(setting.Value) {
				return setting.Value
			}
		}
	}
	return "local"
}

func validateBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("HOTKEY_SOURCE_LIVE_SMOKE_BASE_URL must be an origin URL")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil, errors.New("HOTKEY_SOURCE_LIVE_SMOKE_BASE_URL must use HTTPS or loopback HTTP")
	}
	parsed.Path = "/"
	return parsed, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateOutputPath(raw string) (string, error) {
	path := filepath.Clean(strings.TrimSpace(raw))
	if path == "." || filepath.Ext(path) != ".json" {
		return "", errors.New("HOTKEY_SOURCE_LIVE_SMOKE_OUTPUT must name a JSON file")
	}
	if _, err := os.Stat(path); err == nil {
		return "", errors.New("HOTKEY_SOURCE_LIVE_SMOKE_OUTPUT already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("HOTKEY_SOURCE_LIVE_SMOKE_OUTPUT cannot be inspected")
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", errors.New("HOTKEY_SOURCE_LIVE_SMOKE_OUTPUT parent directory is unavailable")
	}
	return path, nil
}

func execute(ctx context.Context, client *http.Client, cfg config, now func() time.Time) (report, bool) {
	startedAt := now().UTC()
	result := report{
		Version: reportVersion, Status: "failed", Approval: "required",
		Environment: cfg.environment, Revision: cfg.revision, StartedAt: startedAt,
		QueryBytes: len(cfg.query),
		Sources:    make([]sourceResult, 0, len(cfg.sources)),
	}
	sources, err := discoverSources(ctx, client, cfg)
	if err != nil {
		result.ErrorCode = errorCode(err)
		result.FinishedAt = now().UTC()
		return result, false
	}
	cfg.sources = sources
	result.Sources = make([]sourceResult, 0, len(cfg.sources))
	preflightPassed := true
	for _, source := range cfg.sources {
		sourceResult := sourceResult{SourceType: source.SourceType, SourceConnectionID: source.ID}
		preflightStarted := now()
		var sourceData sourceResponse
		err := requestJSON(ctx, client, cfg, http.MethodGet, fmt.Sprintf("api/v1/source-connections/%d", source.ID), nil, &sourceData)
		sourceResult.Preflight.DurationMS = elapsedMilliseconds(preflightStarted, now())
		if err != nil {
			sourceResult.Preflight.ErrorCode = errorCode(err)
			preflightPassed = false
		} else {
			sourceResult.Enabled = sourceData.Enabled
			sourceResult.CredentialConfigured = sourceData.CredentialConfigured
			sourceResult.Preflight.Passed = sourceData.ID == source.ID && sourceData.SourceType == source.SourceType && sourceData.Enabled && !sourceData.Deleted && (source.SourceType != "x" || sourceData.CredentialConfigured)
			if !sourceResult.Preflight.Passed {
				sourceResult.Preflight.ErrorCode = "source_precondition_failed"
				preflightPassed = false
			}
		}
		result.Sources = append(result.Sources, sourceResult)
	}
	if !preflightPassed {
		for index := range result.Sources {
			result.Sources[index].Health.ErrorCode = "preflight_not_passed"
			result.Sources[index].Search.State = "failed"
			result.Sources[index].Search.ErrorCode = "preflight_not_passed"
		}
		result.FinishedAt = now().UTC()
		return result, false
	}

	healthPassed := true
	for index, source := range cfg.sources {
		healthStarted := now()
		var healthData sourceHealthResponse
		err := requestJSON(ctx, client, cfg, http.MethodPost, fmt.Sprintf("api/v1/source-connections/%d/health", source.ID), nil, &healthData)
		result.Sources[index].Health.DurationMS = elapsedMilliseconds(healthStarted, now())
		if err != nil {
			result.Sources[index].Health.ErrorCode = errorCode(err)
			healthPassed = false
		} else {
			result.Sources[index].Health.Healthy = healthData.Healthy
			result.Sources[index].Health.ErrorCode = stableProviderCode(healthData.ErrorCode)
			if !healthData.Healthy {
				healthPassed = false
			}
		}
	}
	if !healthPassed {
		for index := range result.Sources {
			result.Sources[index].Search.State = "failed"
			result.Sources[index].Search.ErrorCode = "health_not_passed"
		}
		result.FinishedAt = now().UTC()
		return result, false
	}

	passed := true
	searchStarted := now()
	var searchData instantSearchResponse
	searchRequest := map[string]any{"query": cfg.query, "source_types": []string{"rss", "hacker_news", "x"}, "limit": 20}
	searchErr := requestJSON(ctx, client, cfg, http.MethodPost, "api/v1/search", searchRequest, &searchData)
	searchDuration := elapsedMilliseconds(searchStarted, now())
	statuses := map[string][]instantSearchStatus{}
	for _, status := range searchData.SourceStatuses {
		if status.SourceType == "rss" || status.SourceType == "hacker_news" || status.SourceType == "x" {
			statuses[status.SourceType] = append(statuses[status.SourceType], status)
		}
	}
	for index := range result.Sources {
		current := &result.Sources[index]
		current.Search.DurationMS = searchDuration
		if searchErr != nil {
			current.Search.State = "failed"
			current.Search.ErrorCode = errorCode(searchErr)
			passed = false
			continue
		}
		matches := statuses[current.SourceType]
		if len(matches) != 1 {
			current.Search.State = "failed"
			current.Search.ErrorCode = "ambiguous_source_status"
			passed = false
			continue
		}
		current.Search.State = stableSearchState(matches[0].State)
		current.Search.ResultCount = matches[0].ResultCount
		current.Search.ErrorCode = stableProviderCode(matches[0].ErrorCode)
		if current.Search.State != "success" || current.Search.ResultCount <= 0 {
			passed = false
		}
	}
	result.FinishedAt = now().UTC()
	if passed {
		result.Status = "passed"
	}
	return result, passed
}

func discoverSources(ctx context.Context, client *http.Client, cfg config) ([]sourceInput, error) {
	var page sourcePageResponse
	if err := requestJSON(ctx, client, cfg, http.MethodGet, "api/v1/source-connections?limit=100", nil, &page); err != nil {
		return nil, err
	}
	if page.NextCursor != "" {
		return nil, &codedError{code: "source_selection_ambiguous"}
	}
	wanted := []string{"rss", "hacker_news", "x"}
	byType := make(map[string][]sourceInput, len(wanted))
	for _, item := range page.Items {
		if item.Enabled && !item.Deleted && (item.SourceType == "rss" || item.SourceType == "hacker_news" || item.SourceType == "x") {
			byType[item.SourceType] = append(byType[item.SourceType], sourceInput{SourceType: item.SourceType, ID: item.ID})
		}
	}
	sources := make([]sourceInput, 0, len(wanted))
	for _, sourceType := range wanted {
		if len(byType[sourceType]) != 1 {
			return nil, &codedError{code: "source_selection_ambiguous"}
		}
		sources = append(sources, byType[sourceType][0])
	}
	return sources, nil
}

func requestJSON(ctx context.Context, client *http.Client, cfg config, method, path string, payload any, output any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return &codedError{code: "request_encoding_failed"}
		}
		body = bytes.NewReader(encoded)
	}
	reference, err := url.Parse(path)
	if err != nil {
		return &codedError{code: "request_construction_failed"}
	}
	target := cfg.baseURL.ResolveReference(reference)
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return &codedError{code: "request_construction_failed"}
	}
	request.Header.Set("Authorization", "Bearer "+cfg.token)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return &codedError{code: "request_failed"}
	}
	defer response.Body.Close()
	limited, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return &codedError{code: "response_read_failed"}
	}
	if len(limited) > maxResponseBytes {
		return &codedError{code: "response_too_large"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &codedError{code: fmt.Sprintf("http_%d", response.StatusCode)}
	}
	envelope := apiEnvelope[json.RawMessage]{}
	if err := json.Unmarshal(limited, &envelope); err != nil || envelope.Code != 0 {
		return &codedError{code: "api_response_rejected"}
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return &codedError{code: "api_data_invalid"}
	}
	return nil
}

func elapsedMilliseconds(startedAt, finishedAt time.Time) int64 {
	duration := finishedAt.Sub(startedAt)
	if duration <= 0 {
		return 0
	}
	return duration.Milliseconds()
}

func errorCode(err error) string {
	var coded *codedError
	if errors.As(err, &coded) {
		return coded.code
	}
	return "internal_error"
}

func stableProviderCode(value string) string {
	switch strings.TrimSpace(value) {
	case "":
		return ""
	case "invalid_source_connection", "request_failed", "upstream_status", "connector_unavailable",
		"destination_not_permitted", "credential_unavailable", "rate_limited", "admission_unavailable",
		"policy_not_permitted", "probe_failed", "authentication", "temporary", "parse", "unavailable",
		"not_configured":
		return strings.TrimSpace(value)
	default:
		return "upstream_error"
	}
}

func stableSearchState(value string) string {
	switch value {
	case "success", "empty", "partial", "failed", "unavailable":
		return value
	default:
		return "failed"
	}
}

func writeReport(path string, value report) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("output must not already exist")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		return errors.New("output could not be written")
	}
	return nil
}
