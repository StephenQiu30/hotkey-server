package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	eventpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/postgres"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/postgres"
	searchapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/application"
	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	searchhttp "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/transport/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

const (
	searchCapacityVersion = "hotkey-search-capacity-v1"
	maximumResponseBytes  = 2 << 20
)

var (
	planNodeTypePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 ]{0,63}$`)
	planObjectPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,127}$`)
	environmentPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	fixedQueries        = []capacityQuery{
		newCapacityQuery("mixed-all", map[string]string{"q": "capacitytoken", "limit": "20"}),
		newCapacityQuery("multilingual-all", map[string]string{"q": "芯片", "limit": "20"}),
		newCapacityQuery("trigram-all", map[string]string{"q": "capacitytoke 芯片 release capacity-entity 2 multilingual PostgreSQL search capacity fixture 2", "types": "content", "limit": "20"}),
		newCapacityQuery("entity-content", map[string]string{"q": "capacitytoken", "types": "content", "entity": "capacity-entity", "limit": "20"}),
		newCapacityQuery("active-latest", map[string]string{"q": "capacitytoken", "status": "active", "sort": "latest", "limit": "20"}),
	}
)

type config struct {
	DSN, Output, Environment, Hardware, GitRevision, CacheState string
	RowsPerResource, Concurrency, Warmups, Samples              int
}

type report struct {
	Version             string              `json:"version"`
	Status              string              `json:"status"`
	Approval            string              `json:"approval"`
	GitRevision         string              `json:"git_revision"`
	Environment         string              `json:"environment"`
	Hardware            string              `json:"hardware"`
	CacheState          string              `json:"cache_state"`
	PercentileAlgorithm string              `json:"percentile_algorithm"`
	Runtime             runtimeFacts        `json:"runtime"`
	Corpus              corpusFacts         `json:"corpus"`
	API                 apiEvidence         `json:"api"`
	IndexUpdate         indexUpdateEvidence `json:"index_update"`
	Plans               []planEvidence      `json:"query_plans"`
	StartedAt           time.Time           `json:"started_at"`
	CompletedAt         time.Time           `json:"completed_at"`
	Exclusions          []string            `json:"exclusions"`
}

type runtimeFacts struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	CPUs   int    `json:"cpus"`
}

type corpusFacts struct {
	ContentRows   int `json:"content_rows"`
	EventRows     int `json:"event_rows"`
	KnowledgeRows int `json:"knowledge_rows"`
	TotalRows     int `json:"total_rows"`
}

type distribution struct {
	Samples        int     `json:"samples"`
	DurationMicros []int64 `json:"duration_micros"`
	P50Micros      int64   `json:"p50_micros"`
	P95Micros      int64   `json:"p95_micros"`
	P99Micros      int64   `json:"p99_micros"`
	Errors         int     `json:"errors"`
}

type apiEvidence struct {
	RouteShape  string          `json:"route_shape"`
	Stack       string          `json:"stack"`
	Concurrency int             `json:"concurrency"`
	Warmups     int             `json:"warmups"`
	Latency     distribution    `json:"latency"`
	Queries     []queryEvidence `json:"queries"`
}

type queryEvidence struct {
	ID                   string       `json:"id"`
	QueryDigest          string       `json:"query_digest"`
	ResultCount          int          `json:"result_count"`
	ResultIdentityDigest string       `json:"result_identity_digest"`
	Latency              distribution `json:"latency"`
}

type indexUpdateEvidence struct {
	ResourceType  string `json:"resource_type"`
	QueryDigest   string `json:"query_digest"`
	Visible       bool   `json:"visible"`
	Attempts      int    `json:"attempts"`
	LatencyMicros int64  `json:"latency_micros"`
}

type planEvidence struct {
	ResourceType string     `json:"resource_type"`
	PlanDigest   string     `json:"plan_digest"`
	Nodes        []planNode `json:"nodes"`
}

type planNode struct {
	Depth    int    `json:"depth"`
	NodeType string `json:"node_type"`
	Relation string `json:"relation,omitempty"`
	Index    string `json:"index,omitempty"`
}

type rawPlanRoot struct {
	Plan rawPlanNode `json:"Plan"`
}

type rawPlanNode struct {
	NodeType string        `json:"Node Type"`
	Relation string        `json:"Relation Name"`
	Index    string        `json:"Index Name"`
	Plans    []rawPlanNode `json:"Plans"`
}

type capacityQuery struct {
	id    string
	route string
}

type resultIdentity struct {
	ResourceType string `json:"type"`
	ID           int64  `json:"id"`
}

type requestResult struct {
	queryID    string
	duration   time.Duration
	identities []resultIdentity
	digest     string
	err        error
}

type expectedResult struct {
	count  int
	digest string
}

type capacitySubjectReader struct{}

func (capacitySubjectReader) CurrentSearchSubject(context.Context, int64) (searchapplication.Subject, error) {
	return searchapplication.Subject{UserID: 1, Role: "viewer"}, nil
}

func (capacitySubjectReader) SearchScopeVisible(context.Context, searchapplication.Subject, searchdomain.Query) (bool, error) {
	return true, nil
}

func (capacitySubjectReader) SearchCandidateVisible(context.Context, searchapplication.Subject, searchdomain.Candidate) (bool, error) {
	return true, nil
}

type capacityAuthenticator struct{}

func (capacityAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleViewer}, nil
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
	defer cancel()
	runtimeDB, err := database.Open(ctx, cfg.DSN)
	if err != nil {
		return errors.New("open PostgreSQL for search capacity baseline")
	}
	defer runtimeDB.Close()
	poolLimit := int(runtimeDB.Pool.Config().MaxConns)
	runtimeDB.SQL.SetMaxOpenConns(poolLimit)
	runtimeDB.SQL.SetMaxIdleConns(poolLimit)

	corpus, err := readCorpusFacts(ctx, runtimeDB)
	if err != nil {
		return err
	}
	want := corpusFacts{ContentRows: cfg.RowsPerResource, EventRows: cfg.RowsPerResource, KnowledgeRows: cfg.RowsPerResource, TotalRows: cfg.RowsPerResource * 3}
	if corpus != want {
		return fmt.Errorf("search capacity corpus facts do not match configured scale")
	}

	contents := ingestionpostgres.NewContentRepository(runtimeDB)
	events, err := eventpostgres.NewMicroEventQueryPostgresRepository(runtimeDB)
	if err != nil {
		return errors.New("create event search capacity reader")
	}
	knowledge := knowledgepostgres.NewRepository(runtimeDB)
	service, err := searchapplication.NewService(searchapplication.Readers{Content: contents, Event: events, Knowledge: knowledge}, capacitySubjectReader{})
	if err != nil {
		return errors.New("create search capacity service")
	}

	startedAt := time.Now().UTC()
	plans, err := collectPlans(ctx, contents, events, knowledge)
	if err != nil {
		return err
	}
	api, err := measureAPI(ctx, service, cfg)
	if err != nil {
		return err
	}
	indexUpdate, err := measureIndexUpdate(ctx, runtimeDB, service)
	if err != nil {
		return err
	}
	result := report{
		Version: searchCapacityVersion, Status: "measured", Approval: "required", GitRevision: cfg.GitRevision,
		Environment: cfg.Environment, Hardware: cfg.Hardware, CacheState: cfg.CacheState,
		PercentileAlgorithm: "nearest-rank-ceiling",
		Runtime:             runtimeFacts{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUs: runtime.NumCPU()},
		Corpus:              corpus, API: api, IndexUpdate: indexUpdate, Plans: plans,
		StartedAt: startedAt, CompletedAt: time.Now().UTC(),
		Exclusions: []string{
			"fixture_generation", "public_network_tls", "production_identity_lookup", "browser_rendering",
			"query_text_title_snippet_body_and_host_paths_intentionally_omitted", "candidate_targets_require_human_approval",
		},
	}
	if api.Latency.Errors > 0 || !indexUpdate.Visible {
		result.Status = "failed"
	}
	if err := writeExclusiveJSON(cfg.Output, result); err != nil {
		return err
	}
	if result.Status != "measured" {
		return errors.New("search capacity baseline completed with errors")
	}
	fmt.Printf("search capacity evidence written (api p95=%d us; index update=%d us; approval remains required)\n", api.Latency.P95Micros, indexUpdate.LatencyMicros)
	return nil
}

func loadConfig() (config, error) {
	result := config{
		DSN: os.Getenv("HOTKEY_TEST_DSN"), Output: os.Getenv("HOTKEY_SEARCH_CAPACITY_OUTPUT"),
		Environment: os.Getenv("HOTKEY_SEARCH_CAPACITY_ENVIRONMENT"), Hardware: os.Getenv("HOTKEY_SEARCH_CAPACITY_HARDWARE"),
		GitRevision: os.Getenv("HOTKEY_SEARCH_CAPACITY_GIT_REVISION"), CacheState: os.Getenv("HOTKEY_SEARCH_CAPACITY_CACHE_STATE"),
	}
	values := []struct {
		name     string
		fallback int
		target   *int
	}{
		{"HOTKEY_SEARCH_CAPACITY_ROWS_PER_RESOURCE", 1000, &result.RowsPerResource},
		{"HOTKEY_SEARCH_CAPACITY_CONCURRENCY", 4, &result.Concurrency},
		{"HOTKEY_SEARCH_CAPACITY_WARMUPS", 10, &result.Warmups},
		{"HOTKEY_SEARCH_CAPACITY_SAMPLES", 120, &result.Samples},
	}
	for _, value := range values {
		parsed, err := positiveEnvironmentInteger(value.name, value.fallback)
		if err != nil {
			return config{}, err
		}
		*value.target = parsed
	}
	if strings.TrimSpace(result.DSN) == "" || strings.TrimSpace(result.Output) == "" ||
		strings.TrimSpace(result.Environment) == "" || strings.TrimSpace(result.Hardware) == "" {
		return config{}, errors.New("search capacity DSN, output, environment and hardware metadata are required")
	}
	if !environmentPattern.MatchString(result.Environment) || len(result.Hardware) > 256 ||
		result.Hardware != strings.TrimSpace(result.Hardware) || strings.ContainsAny(result.Hardware, "\r\n\\") ||
		strings.Contains(result.Hardware, "/Users/") || strings.Contains(result.Hardware, "/home/") ||
		strings.Contains(strings.ToLower(result.Hardware), "file://") {
		return config{}, errors.New("search capacity environment or hardware metadata is invalid")
	}
	if len(result.GitRevision) != 40 || strings.Trim(result.GitRevision, "0123456789abcdef") != "" {
		return config{}, errors.New("HOTKEY_SEARCH_CAPACITY_GIT_REVISION must be a 40-character lowercase commit SHA")
	}
	if result.CacheState != "warm" && result.CacheState != "cold" {
		return config{}, errors.New("HOTKEY_SEARCH_CAPACITY_CACHE_STATE must be warm or cold")
	}
	if result.RowsPerResource > 100000 || result.Concurrency > 16 || result.Warmups > 10000 || result.Samples > 100000 || result.Samples < len(fixedQueries) {
		return config{}, errors.New("search capacity configuration exceeds evidence tool bounds")
	}
	return result, nil
}

func positiveEnvironmentInteger(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func readCorpusFacts(ctx context.Context, runtimeDB *database.Runtime) (corpusFacts, error) {
	var result corpusFacts
	err := runtimeDB.SQL.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM contents AS content JOIN source_connections AS source ON source.id=content.source_connection_id WHERE source.name='search-capacity-source'),
  (SELECT count(*) FROM micro_events WHERE clustering_profile_version='search-capacity-v1'),
  (SELECT count(*) FROM knowledge_documents WHERE vault_path LIKE 'events/search-capacity-%')`).
		Scan(&result.ContentRows, &result.EventRows, &result.KnowledgeRows)
	if err != nil {
		return corpusFacts{}, errors.New("read search capacity corpus facts")
	}
	result.TotalRows = result.ContentRows + result.EventRows + result.KnowledgeRows
	return result, nil
}

func collectPlans(ctx context.Context, contents *ingestionpostgres.ContentRepository, events *eventpostgres.MicroEventQueryPostgresRepository, knowledge *knowledgepostgres.Repository) ([]planEvidence, error) {
	definitions := []struct {
		resourceType string
		load         func() ([]byte, error)
	}{
		{resourceType: "content", load: func() ([]byte, error) {
			return contents.ExplainSearch(ctx, searchdomain.Query{Keyword: "capacitytoken", Types: []searchdomain.ResourceType{searchdomain.ResourceContent}, Limit: 20})
		}},
		{resourceType: "event", load: func() ([]byte, error) {
			return events.ExplainSearch(ctx, searchdomain.Query{Keyword: "capacitytoken", Types: []searchdomain.ResourceType{searchdomain.ResourceEvent}, Limit: 20})
		}},
		{resourceType: "knowledge", load: func() ([]byte, error) {
			return knowledge.ExplainSearch(ctx, searchdomain.Query{Keyword: "capacitytoken", Types: []searchdomain.ResourceType{searchdomain.ResourceKnowledge}, Limit: 20})
		}},
	}
	result := make([]planEvidence, 0, len(definitions))
	for _, definition := range definitions {
		raw, err := definition.load()
		if err != nil {
			return nil, errors.New("load search capacity query plan")
		}
		nodes, err := sanitizePlan(raw)
		if err != nil || len(nodes) == 0 {
			return nil, errors.New("sanitize search capacity query plan")
		}
		payload, err := json.Marshal(nodes)
		if err != nil {
			return nil, errors.New("encode sanitized search capacity query plan")
		}
		result = append(result, planEvidence{ResourceType: definition.resourceType, PlanDigest: digestBytes(payload), Nodes: nodes})
	}
	return result, nil
}

func sanitizePlan(raw []byte) ([]planNode, error) {
	var roots []rawPlanRoot
	if !json.Valid(raw) || json.Unmarshal(raw, &roots) != nil || len(roots) != 1 {
		return nil, errors.New("invalid PostgreSQL JSON plan")
	}
	result := make([]planNode, 0)
	var walk func(rawPlanNode, int) error
	walk = func(node rawPlanNode, depth int) error {
		if !planNodeTypePattern.MatchString(node.NodeType) ||
			node.Relation != "" && !planObjectPattern.MatchString(node.Relation) ||
			node.Index != "" && !planObjectPattern.MatchString(node.Index) || depth > 64 {
			return errors.New("query plan contains an unsafe structural value")
		}
		result = append(result, planNode{Depth: depth, NodeType: node.NodeType, Relation: node.Relation, Index: node.Index})
		for _, child := range node.Plans {
			if err := walk(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(roots[0].Plan, 0); err != nil {
		return nil, err
	}
	return result, nil
}

func measureAPI(ctx context.Context, service *searchapplication.Service, cfg config) (apiEvidence, error) {
	server := newSearchServer(service)
	defer server.Close()
	client := server.Client()
	expected := make(map[string]expectedResult, len(fixedQueries))
	for _, query := range fixedQueries {
		measured := executeSearchRequest(ctx, client, server.URL, query)
		if measured.err != nil {
			return apiEvidence{}, fmt.Errorf("establish fixed search query %s result: %w", query.id, measured.err)
		}
		if len(measured.identities) == 0 {
			return apiEvidence{}, fmt.Errorf("establish fixed search query %s result: empty result", query.id)
		}
		expected[query.id] = expectedResult{count: len(measured.identities), digest: measured.digest}
	}
	for index := range cfg.Warmups {
		query := fixedQueries[index%len(fixedQueries)]
		measured := executeSearchRequest(ctx, client, server.URL, query)
		want := expected[query.id]
		if measured.err != nil || measured.digest != want.digest || len(measured.identities) != want.count {
			return apiEvidence{}, errors.New("search capacity warmup was unstable")
		}
	}
	measurements := executeConcurrent(cfg.Samples, cfg.Concurrency, func(index int) requestResult {
		query := fixedQueries[index%len(fixedQueries)]
		measured := executeSearchRequest(ctx, client, server.URL, query)
		want := expected[query.id]
		if measured.err == nil && (measured.digest != want.digest || len(measured.identities) != want.count) {
			measured.err = errors.New("search result identity or order changed")
		}
		return measured
	})
	queries := make([]queryEvidence, 0, len(fixedQueries))
	for _, query := range fixedQueries {
		filtered := make([]requestResult, 0)
		for _, measured := range measurements {
			if measured.queryID == query.id {
				filtered = append(filtered, measured)
			}
		}
		want := expected[query.id]
		queries = append(queries, queryEvidence{
			ID: query.id, QueryDigest: digestString(query.route), ResultCount: want.count,
			ResultIdentityDigest: want.digest, Latency: summarize(filtered),
		})
	}
	return apiEvidence{
		RouteShape:  "/api/v1/search?<bounded-filter-set>",
		Stack:       "httptest_http+gin+authn+authz+search_application+owner_postgres+dto+json",
		Concurrency: cfg.Concurrency, Warmups: cfg.Warmups, Latency: summarize(measurements), Queries: queries,
	}, nil
}

func newSearchServer(service *searchapplication.Service) *httptest.Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	searchhttp.RegisterRoutes(router, service, capacityAuthenticator{})
	return httptest.NewServer(router)
}

func executeConcurrent(count, concurrency int, execute func(int) requestResult) []requestResult {
	jobs := make(chan int)
	results := make(chan requestResult, count)
	var workers sync.WaitGroup
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				results <- execute(index)
			}
		}()
	}
	go func() {
		for index := range count {
			jobs <- index
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()
	measured := make([]requestResult, 0, count)
	for result := range results {
		measured = append(measured, result)
	}
	return measured
}

func executeSearchRequest(parent context.Context, client *http.Client, baseURL string, query capacityQuery) requestResult {
	result := requestResult{queryID: query.id}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+query.route, nil)
	if err != nil {
		result.err = err
		return result
	}
	request.Header.Set("Authorization", "Bearer search-capacity-fixture")
	startedAt := time.Now()
	response, err := client.Do(request)
	if err != nil {
		result.err = err
		return result
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	closeErr := response.Body.Close()
	result.duration = time.Since(startedAt)
	if readErr != nil || closeErr != nil || len(payload) > maximumResponseBytes || response.StatusCode != http.StatusOK {
		result.err = errors.New("search capacity response transport contract failed")
		return result
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				Type string `json:"type"`
				ID   int64  `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if !json.Valid(payload) || json.Unmarshal(payload, &envelope) != nil || envelope.Code != 0 || len(envelope.Data.Items) > searchdomain.MaximumLimit {
		result.err = errors.New("search capacity response JSON contract failed")
		return result
	}
	result.identities = make([]resultIdentity, 0, len(envelope.Data.Items))
	seen := make(map[resultIdentity]struct{}, len(envelope.Data.Items))
	for _, item := range envelope.Data.Items {
		identity := resultIdentity{ResourceType: item.Type, ID: item.ID}
		if item.ID <= 0 || searchdomain.ResourceType(item.Type).Valid() == false {
			result.err = errors.New("search capacity response contains invalid identity")
			return result
		}
		if _, exists := seen[identity]; exists {
			result.err = errors.New("search capacity response contains duplicate identity")
			return result
		}
		seen[identity] = struct{}{}
		result.identities = append(result.identities, identity)
	}
	result.digest = resultIdentityDigest(result.identities)
	return result
}

func measureIndexUpdate(ctx context.Context, runtimeDB *database.Runtime, service *searchapplication.Service) (indexUpdateEvidence, error) {
	var contentID int64
	err := runtimeDB.SQL.QueryRowContext(ctx, `
UPDATE contents
SET title=replace(title,'capacityindexstale','capacityindexfresh'),version=version+1,updated_at=clock_timestamp()
WHERE id=(
  SELECT content.id FROM contents AS content
  JOIN source_connections AS source ON source.id=content.source_connection_id
  WHERE source.name='search-capacity-source' AND strpos(content.title,'capacityindexstale')>0
  ORDER BY content.id LIMIT 1
)
RETURNING id`).Scan(&contentID)
	if err != nil {
		return indexUpdateEvidence{}, errors.New("prepare search index update fixture")
	}
	defer func() {
		_, _ = runtimeDB.SQL.ExecContext(context.Background(), `
UPDATE contents
SET title=replace(title,'capacityindexfresh','capacityindexstale'),version=version+1,updated_at=clock_timestamp()
WHERE id=$1 AND strpos(title,'capacityindexfresh')>0`, contentID)
	}()
	query := newCapacityQuery("index-update-content", map[string]string{"q": "capacityindexfresh", "types": "content", "limit": "10"})
	server := newSearchServer(service)
	defer server.Close()
	startedAt := time.Now()
	result := indexUpdateEvidence{ResourceType: "content", QueryDigest: digestString(query.route)}
	for result.Attempts < 100 && time.Since(startedAt) < 5*time.Second {
		result.Attempts++
		measured := executeSearchRequest(ctx, server.Client(), server.URL, query)
		if measured.err != nil {
			return indexUpdateEvidence{}, errors.New("search index update visibility request failed")
		}
		for _, identity := range measured.identities {
			if identity.ResourceType == "content" && identity.ID == contentID {
				result.Visible = true
				result.LatencyMicros = time.Since(startedAt).Microseconds()
				return result, nil
			}
		}
		select {
		case <-ctx.Done():
			return indexUpdateEvidence{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	result.LatencyMicros = time.Since(startedAt).Microseconds()
	return result, errors.New("search index update did not become visible within the bounded window")
}

func newCapacityQuery(id string, values map[string]string) capacityQuery {
	parameters := make(url.Values, len(values))
	for key, value := range values {
		parameters.Set(key, value)
	}
	return capacityQuery{id: id, route: "/api/v1/search?" + parameters.Encode()}
}

func resultIdentityDigest(values []resultIdentity) string {
	payload, _ := json.Marshal(values)
	return digestBytes(payload)
}

func digestString(value string) string { return digestBytes([]byte(value)) }

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func summarize(values []requestResult) distribution {
	result := distribution{Samples: len(values)}
	for _, value := range values {
		if value.err != nil {
			result.Errors++
			continue
		}
		result.DurationMicros = append(result.DurationMicros, value.duration.Microseconds())
	}
	sort.Slice(result.DurationMicros, func(left, right int) bool { return result.DurationMicros[left] < result.DurationMicros[right] })
	result.P50Micros = nearestRank(result.DurationMicros, 50)
	result.P95Micros = nearestRank(result.DurationMicros, 95)
	result.P99Micros = nearestRank(result.DurationMicros, 99)
	return result
}

func nearestRank(sortedValues []int64, percentile int) int64 {
	if len(sortedValues) == 0 || percentile < 1 || percentile > 100 {
		return 0
	}
	index := int(math.Ceil(float64(percentile)*float64(len(sortedValues))/100.0)) - 1
	return sortedValues[index]
}

func writeExclusiveJSON(path string, value any) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return errors.New("search capacity output path is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		return errors.New("create search capacity evidence directory")
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("search capacity evidence file already exists or cannot be created")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		return errors.New("write search capacity evidence")
	}
	return nil
}
