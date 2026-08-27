package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	sourcehttp "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/transport/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

const fixturePrefix = "collection-capacity-"

type config struct {
	DSN, Output, Environment, Hardware, GitRevision, CacheState string
	Monitors, Sources, Candidates, Jobs                         int
	WriteRows, WriteSamples, APISamples, APIConcurrency         int
}

type report struct {
	Version             string        `json:"version"`
	Status              string        `json:"status"`
	GitRevision         string        `json:"git_revision"`
	Environment         string        `json:"environment"`
	Hardware            string        `json:"hardware"`
	CacheState          string        `json:"cache_state"`
	PercentileAlgorithm string        `json:"percentile_algorithm"`
	Runtime             runtimeFacts  `json:"runtime"`
	Fixture             fixtureFacts  `json:"fixture"`
	Queue               queueEvidence `json:"queue"`
	Collection          writeEvidence `json:"collection_persistence"`
	API                 apiEvidence   `json:"api"`
	StartedAt           time.Time     `json:"started_at"`
	CompletedAt         time.Time     `json:"completed_at"`
	Exclusions          []string      `json:"exclusions"`
}

type runtimeFacts struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	CPUs   int    `json:"cpus"`
}

type fixtureFacts struct {
	ActiveMonitors           int `json:"active_monitors"`
	EnabledSourceConnections int `json:"enabled_source_connections"`
	CandidateItems           int `json:"candidate_items"`
	CollectionJobs           int `json:"collection_jobs"`
}

type distribution struct {
	Samples        int     `json:"samples"`
	DurationMicros []int64 `json:"duration_micros"`
	P50Micros      int64   `json:"p50_micros"`
	P95Micros      int64   `json:"p95_micros"`
	P99Micros      int64   `json:"p99_micros"`
	Errors         int     `json:"errors"`
}

type queueEvidence struct {
	Concurrency   int          `json:"concurrency"`
	CompletedJobs int          `json:"completed_jobs"`
	Wait          distribution `json:"wait"`
}

type writeEvidence struct {
	RowsPerSample           int          `json:"rows_per_sample"`
	CommitDuration          distribution `json:"commit_duration"`
	ThroughputRowsPerSecond float64      `json:"throughput_rows_per_second"`
}

type apiEvidence struct {
	Route       string       `json:"route"`
	Stack       string       `json:"stack"`
	Concurrency int          `json:"concurrency"`
	Warmups     int          `json:"warmups"`
	Latency     distribution `json:"latency"`
}

type measured struct {
	duration time.Duration
	err      error
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
		return errors.New("open PostgreSQL for collection capacity baseline")
	}
	defer runtimeDB.Close()
	// The database/sql facade is backed by the already-bounded pgx pool. Its
	// open limit must never exceed pgx MaxConns, otherwise concurrent callers
	// can retain every pgx connection while database/sql opens facade
	// connections that wait forever for a second-layer slot.
	poolLimit := int(runtimeDB.Pool.Config().MaxConns)
	runtimeDB.SQL.SetMaxOpenConns(poolLimit)
	runtimeDB.SQL.SetMaxIdleConns(poolLimit)

	facts, err := readFixtureFacts(ctx, runtimeDB.SQL)
	if err != nil {
		return err
	}
	if facts != (fixtureFacts{cfg.Monitors, cfg.Sources, cfg.Candidates, cfg.Jobs}) {
		return fmt.Errorf("collection capacity fixture = %+v, want monitors=%d sources=%d candidates=%d jobs=%d", facts, cfg.Monitors, cfg.Sources, cfg.Candidates, cfg.Jobs)
	}
	startedAt := time.Now().UTC()
	queueResult, err := measureQueue(ctx, runtimeDB, cfg.Jobs)
	if err != nil {
		return err
	}
	writeResult, err := measureCollectionWrites(ctx, runtimeDB.SQL, cfg.WriteRows, cfg.WriteSamples)
	if err != nil {
		return err
	}
	apiResult, err := measureAPI(ctx, runtimeDB, cfg.APISamples, cfg.APIConcurrency)
	if err != nil {
		return err
	}
	result := report{
		Version: "hotkey-collection-capacity-v1", Status: "measured", GitRevision: cfg.GitRevision,
		Environment: cfg.Environment, Hardware: cfg.Hardware, CacheState: cfg.CacheState, PercentileAlgorithm: "nearest-rank-ceiling",
		Runtime: runtimeFacts{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUs: runtime.NumCPU()}, Fixture: facts,
		Queue: queueResult, Collection: writeResult, API: apiResult, StartedAt: startedAt, CompletedAt: time.Now().UTC(),
		Exclusions: []string{"external_connector_network_latency", "provider_rate_limits", "public_network_tls", "production_identity_lookup"},
	}
	if queueResult.Wait.Errors > 0 || writeResult.CommitDuration.Errors > 0 || apiResult.Latency.Errors > 0 || queueResult.CompletedJobs != cfg.Jobs {
		result.Status = "failed"
	}
	if err := writeExclusiveJSON(cfg.Output, result); err != nil {
		return err
	}
	if result.Status != "measured" {
		return errors.New("collection capacity baseline completed with errors")
	}
	fmt.Printf("collection capacity evidence written to %s (queue p95=%d us, collection p95=%d us, API p95=%d us)\n", cfg.Output, result.Queue.Wait.P95Micros, result.Collection.CommitDuration.P95Micros, result.API.Latency.P95Micros)
	return nil
}

func loadConfig() (config, error) {
	result := config{
		DSN: os.Getenv("HOTKEY_TEST_DSN"), Output: os.Getenv("HOTKEY_COLLECTION_CAPACITY_OUTPUT"),
		Environment: os.Getenv("HOTKEY_COLLECTION_CAPACITY_ENVIRONMENT"), Hardware: os.Getenv("HOTKEY_COLLECTION_CAPACITY_HARDWARE"),
		GitRevision: os.Getenv("HOTKEY_COLLECTION_CAPACITY_GIT_REVISION"), CacheState: os.Getenv("HOTKEY_COLLECTION_CAPACITY_CACHE_STATE"),
	}
	values := []struct {
		name     string
		fallback int
		target   *int
	}{
		{"HOTKEY_COLLECTION_CAPACITY_MONITORS", 50, &result.Monitors},
		{"HOTKEY_COLLECTION_CAPACITY_SOURCES", 100, &result.Sources},
		{"HOTKEY_COLLECTION_CAPACITY_CANDIDATES", 50000, &result.Candidates},
		{"HOTKEY_COLLECTION_CAPACITY_JOBS", 20, &result.Jobs},
		{"HOTKEY_COLLECTION_CAPACITY_WRITE_ROWS", 500, &result.WriteRows},
		{"HOTKEY_COLLECTION_CAPACITY_WRITE_SAMPLES", 20, &result.WriteSamples},
		{"HOTKEY_COLLECTION_CAPACITY_API_SAMPLES", 200, &result.APISamples},
		{"HOTKEY_COLLECTION_CAPACITY_API_CONCURRENCY", 20, &result.APIConcurrency},
	}
	for _, value := range values {
		parsed, err := positiveEnvironmentInteger(value.name, value.fallback)
		if err != nil {
			return config{}, err
		}
		*value.target = parsed
	}
	if strings.TrimSpace(result.DSN) == "" || strings.TrimSpace(result.Output) == "" || strings.TrimSpace(result.Environment) == "" || strings.TrimSpace(result.Hardware) == "" {
		return config{}, errors.New("HOTKEY_TEST_DSN, HOTKEY_COLLECTION_CAPACITY_OUTPUT, HOTKEY_COLLECTION_CAPACITY_ENVIRONMENT and HOTKEY_COLLECTION_CAPACITY_HARDWARE are required")
	}
	if len(result.GitRevision) != 40 || strings.Trim(result.GitRevision, "0123456789abcdef") != "" {
		return config{}, errors.New("HOTKEY_COLLECTION_CAPACITY_GIT_REVISION must be a 40-character lowercase commit SHA")
	}
	if result.CacheState != "warm" && result.CacheState != "cold" {
		return config{}, errors.New("HOTKEY_COLLECTION_CAPACITY_CACHE_STATE must be warm or cold")
	}
	if result.Jobs > 100 || result.APIConcurrency > 100 || result.APISamples > 100000 || result.WriteSamples > 10000 || result.WriteRows > 10000 {
		return config{}, errors.New("collection capacity configuration exceeds evidence tool bounds")
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

func readFixtureFacts(ctx context.Context, database *sql.DB) (fixtureFacts, error) {
	var result fixtureFacts
	err := database.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM monitors WHERE name LIKE 'collection-capacity-monitor-%' AND status='active' AND deleted_at IS NULL),
  (SELECT count(*) FROM source_connections WHERE name LIKE 'collection-capacity-source-%' AND enabled AND deleted_at IS NULL),
  (SELECT count(*) FROM collection_run_items item JOIN source_connections source ON source.id=item.source_connection_id WHERE source.name LIKE 'collection-capacity-source-%'),
  (SELECT count(*) FROM river_job WHERE kind='collect_source' AND convert_from(unique_key, 'UTF8') LIKE 'collection-capacity-job-%')`).
		Scan(&result.ActiveMonitors, &result.EnabledSourceConnections, &result.CandidateItems, &result.CollectionJobs)
	if err != nil {
		return fixtureFacts{}, errors.New("read collection capacity fixture facts")
	}
	return result, nil
}

func measureQueue(ctx context.Context, runtimeDB *database.Runtime, jobs int) (queueEvidence, error) {
	if _, err := runtimeDB.SQL.ExecContext(ctx, `
UPDATE river_job
SET state='available',attempt=0,attempted_at=NULL,finalized_at=NULL,
    scheduled_at=clock_timestamp(),created_at=clock_timestamp()
WHERE kind='collect_source' AND convert_from(unique_key, 'UTF8') LIKE 'collection-capacity-job-%'`); err != nil {
		return queueEvidence{}, errors.New("reset collection capacity jobs")
	}
	results := make(chan measured, jobs)
	var workers sync.WaitGroup
	for range jobs {
		workers.Add(1)
		go func() {
			defer workers.Done()
			started := time.Now()
			err := claimAndCompleteCapacityJob(ctx, runtimeDB)
			results <- measured{duration: time.Since(started), err: err}
		}()
	}
	workers.Wait()
	close(results)
	measuredClaims := make([]measured, 0, jobs)
	for result := range results {
		measuredClaims = append(measuredClaims, result)
	}
	var completed int
	if err := runtimeDB.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE kind='collect_source' AND convert_from(unique_key, 'UTF8') LIKE 'collection-capacity-job-%' AND state='completed'`).Scan(&completed); err != nil {
		return queueEvidence{}, errors.New("count completed collection capacity jobs")
	}
	rows, err := runtimeDB.SQL.QueryContext(ctx, `SELECT greatest(0,(extract(epoch FROM (attempted_at-created_at))*1000000)::bigint) FROM river_job WHERE kind='collect_source' AND convert_from(unique_key, 'UTF8') LIKE 'collection-capacity-job-%' ORDER BY id`)
	if err != nil {
		return queueEvidence{}, errors.New("read collection queue wait")
	}
	defer rows.Close()
	waits := make([]measured, 0, jobs)
	for rows.Next() {
		var micros int64
		if err := rows.Scan(&micros); err != nil {
			return queueEvidence{}, errors.New("scan collection queue wait")
		}
		waits = append(waits, measured{duration: time.Duration(micros) * time.Microsecond})
	}
	claimErrors := 0
	for _, claim := range measuredClaims {
		if claim.err != nil {
			claimErrors++
		}
	}
	dist := summarize(waits)
	dist.Errors += claimErrors
	return queueEvidence{Concurrency: jobs, CompletedJobs: completed, Wait: dist}, nil
}

func claimAndCompleteCapacityJob(ctx context.Context, runtimeDB *database.Runtime) error {
	var jobID int64
	err := runtimeDB.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if err := transaction.SQL.QueryRowContext(transactionCtx, `
SELECT id FROM river_job
WHERE state='available' AND scheduled_at<=now() AND kind='collect_source'
  AND convert_from(unique_key, 'UTF8') LIKE 'collection-capacity-job-%'
ORDER BY priority,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&jobID); err != nil {
			return err
		}
		_, err := transaction.SQL.ExecContext(transactionCtx, `UPDATE river_job SET state='running',attempt=attempt+1,attempted_at=clock_timestamp() WHERE id=$1`, jobID)
		return err
	})
	if err != nil {
		return err
	}
	_, err = runtimeDB.SQL.ExecContext(ctx, `UPDATE river_job SET state='completed',finalized_at=clock_timestamp() WHERE id=$1 AND state='running'`, jobID)
	return err
}

func measureCollectionWrites(ctx context.Context, database *sql.DB, rowsPerSample, samples int) (writeEvidence, error) {
	var sourceID, monitorSourceID, configID int64
	if err := database.QueryRowContext(ctx, `
SELECT source.id,monitor_source.id,monitor_source.config_version_id
FROM source_connections source
JOIN monitor_sources monitor_source ON monitor_source.source_connection_id=source.id
WHERE source.name LIKE 'collection-capacity-source-%'
ORDER BY source.name,monitor_source.id LIMIT 1`).Scan(&sourceID, &monitorSourceID, &configID); err != nil {
		return writeEvidence{}, errors.New("find collection write fixture target")
	}
	measurements := make([]measured, 0, samples)
	for sampleIndex := range samples {
		started := time.Now()
		runID, err := persistCollectionSample(ctx, database, sourceID, monitorSourceID, configID, rowsPerSample, sampleIndex)
		measurements = append(measurements, measured{duration: time.Since(started), err: err})
		if err == nil {
			if _, cleanupErr := database.ExecContext(ctx, `DELETE FROM collection_runs WHERE id=$1`, runID); cleanupErr != nil {
				return writeEvidence{}, errors.New("clean collection write sample")
			}
		}
	}
	dist := summarize(measurements)
	totalMicros := int64(0)
	for _, duration := range dist.DurationMicros {
		totalMicros += duration
	}
	throughput := 0.0
	if totalMicros > 0 {
		throughput = float64(rowsPerSample*len(dist.DurationMicros)) / (float64(totalMicros) / 1_000_000)
	}
	return writeEvidence{RowsPerSample: rowsPerSample, CommitDuration: dist, ThroughputRowsPerSecond: throughput}, nil
}

func persistCollectionSample(ctx context.Context, database *sql.DB, sourceID, monitorSourceID, configID int64, rows, sampleIndex int) (int64, error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", time.Now().UnixNano(), sampleIndex, sourceID)))
	signature := hex.EncodeToString(digest[:])
	windowStart := time.Now().UTC().Add(time.Duration(sampleIndex+1) * time.Hour)
	windowEnd := windowStart.Add(30 * time.Minute)
	var runID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO collection_runs (
  source_connection_id,query_signature,window_start,window_end,trigger_type,
  scheduled_at,started_at,status,candidate_count,accepted_count,rejected_count
) VALUES ($1,$2,$3,$4,'reconcile',clock_timestamp(),clock_timestamp(),'running',0,0,0)
RETURNING id`, sourceID, signature, windowStart, windowEnd).Scan(&runID); err != nil {
		return 0, err
	}
	var targetID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO collection_run_targets (collection_run_id,monitor_source_id,monitor_config_version_id,target_status)
VALUES ($1,$2,$3,'running') RETURNING id`, runID, monitorSourceID, configID).Scan(&targetID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
WITH inserted AS (
  INSERT INTO collection_run_items (
    run_id,source_code,external_id,content_type,captured_item_version,captured_item,
    payload_hash,raw_payload_disposition,outcome,observed_at,source_connection_id
  )
  SELECT $1,'rss','write-sample-' || ordinal,'article','source-captured-v1',
         jsonb_build_object('fixture',true,'ordinal',ordinal),$2,'captured_item_only','captured',clock_timestamp(),$3
  FROM generate_series(1,$4::integer) AS ordinal
  RETURNING id
)
INSERT INTO collection_run_target_items (
  collection_run_id,collection_run_target_id,collection_run_item_id,outcome
)
SELECT $1,$5,id,'captured' FROM inserted`, runID, signature, sourceID, rows, targetID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collection_run_targets SET target_status='succeeded',candidate_count=$2,accepted_count=$2,updated_at=clock_timestamp() WHERE id=$1`, targetID, rows); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collection_runs SET status='succeeded',candidate_count=$2,accepted_count=$2,finished_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`, runID, rows); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return runID, nil
}

type inertConnectorRegistry struct{}

func (inertConnectorRegistry) Resolve(context.Context, domain.SourceConnection) (domain.Connector, error) {
	return nil, errors.New("connector resolution is outside the API capacity workload")
}

type inertRetryActivator struct{}

func (inertRetryActivator) Reactivate(context.Context, domain.CollectionRunRetry) error { return nil }

type capacityAuthenticator struct{}

func (capacityAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleEditor}, nil
}

func measureAPI(ctx context.Context, runtimeDB *database.Runtime, samples, concurrency int) (apiEvidence, error) {
	runs := sourcepostgres.NewCollectionRepository(runtimeDB)
	service, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtimeDB, Sources: sourcepostgres.NewRepository(runtimeDB), Runs: runs,
		Connectors: inertConnectorRegistry{}, Retries: inertRetryActivator{},
	})
	if err != nil {
		return apiEvidence{}, err
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	sourcehttp.RegisterCollectionRoutes(router, service, capacityAuthenticator{})
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	route := "/api/v1/collection-runs?limit=50"
	for range 10 {
		if _, err := executeAPIRequest(ctx, client, server.URL+route); err != nil {
			return apiEvidence{}, errors.New("collection API warmup failed")
		}
	}
	measurements := executeConcurrent(samples, concurrency, func() measured {
		duration, err := executeAPIRequest(ctx, client, server.URL+route)
		return measured{duration: duration, err: err}
	})
	return apiEvidence{
		Route: route, Stack: "httptest_http+gin+authz+application+postgres+dto+json", Concurrency: concurrency, Warmups: 10,
		Latency: summarize(measurements),
	}, nil
}

func executeAPIRequest(parent context.Context, client *http.Client, url string) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer capacity-fixture")
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	payload, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	duration := time.Since(started)
	if readErr != nil {
		return duration, readErr
	}
	if closeErr != nil {
		return duration, closeErr
	}
	if response.StatusCode != http.StatusOK || !json.Valid(payload) || bytesContainSensitiveCollectionFields(payload) {
		return duration, fmt.Errorf("collection API response contract failed with status %d", response.StatusCode)
	}
	return duration, nil
}

func bytesContainSensitiveCollectionFields(payload []byte) bool {
	text := string(payload)
	for _, field := range []string{"source_connection_id", "query_signature", "request_cursor", "credential", "\"endpoint\""} {
		if strings.Contains(text, field) {
			return true
		}
	}
	return false
}

func executeConcurrent(count, concurrency int, execute func() measured) []measured {
	jobs := make(chan struct{})
	results := make(chan measured, count)
	var workers sync.WaitGroup
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range jobs {
				results <- execute()
			}
		}()
	}
	go func() {
		for range count {
			jobs <- struct{}{}
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()
	measuredResults := make([]measured, 0, count)
	for result := range results {
		measuredResults = append(measuredResults, result)
	}
	return measuredResults
}

func summarize(values []measured) distribution {
	result := distribution{Samples: len(values), DurationMicros: make([]int64, 0, len(values))}
	for _, value := range values {
		if value.err != nil {
			result.Errors++
			continue
		}
		result.DurationMicros = append(result.DurationMicros, value.duration.Microseconds())
	}
	sort.Slice(result.DurationMicros, func(left, right int) bool { return result.DurationMicros[left] < result.DurationMicros[right] })
	if len(result.DurationMicros) > 0 {
		result.P50Micros = nearestRank(result.DurationMicros, 50)
		result.P95Micros = nearestRank(result.DurationMicros, 95)
		result.P99Micros = nearestRank(result.DurationMicros, 99)
	}
	return result
}

func nearestRank(sorted []int64, percentile int) int64 {
	index := (len(sorted)*percentile + 99) / 100
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}

func writeExclusiveJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.New("create collection capacity evidence directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create collection capacity evidence without overwrite: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return errors.New("encode collection capacity evidence")
	}
	if err := file.Close(); err != nil {
		return errors.New("close collection capacity evidence")
	}
	return nil
}
