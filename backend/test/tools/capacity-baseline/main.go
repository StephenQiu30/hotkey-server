package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const workloadQuery = `
SELECT id,published_at
FROM contents
WHERE source_connection_id=$1
ORDER BY published_at DESC,id DESC
LIMIT 50`

type config struct {
	DSN, Output, Environment, Hardware, GitRevision, CacheState string
	ExpectedRows, Concurrency, Warmups, Samples                 int
}

type report struct {
	Version        string        `json:"version"`
	Status         string        `json:"status"`
	GitRevision    string        `json:"git_revision"`
	Environment    string        `json:"environment"`
	Hardware       string        `json:"hardware"`
	Runtime        runtimeFacts  `json:"runtime"`
	Workload       workloadFacts `json:"workload"`
	StartedAt      time.Time     `json:"started_at"`
	CompletedAt    time.Time     `json:"completed_at"`
	DurationMicros []int64       `json:"duration_micros"`
	P50Micros      int64         `json:"p50_micros"`
	P95Micros      int64         `json:"p95_micros"`
	P99Micros      int64         `json:"p99_micros"`
	Errors         int           `json:"errors"`
	Exclusions     []string      `json:"exclusions"`
}

type runtimeFacts struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	CPUs   int    `json:"cpus"`
}

type workloadFacts struct {
	Name                string `json:"name"`
	QueryShape          string `json:"query_shape"`
	CacheState          string `json:"cache_state"`
	PercentileAlgorithm string `json:"percentile_algorithm"`
	FixtureRows         int    `json:"fixture_rows"`
	Concurrency         int    `json:"concurrency"`
	Warmups             int    `json:"warmups"`
	Samples             int    `json:"samples"`
}

type sample struct {
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
	database, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return errors.New("open PostgreSQL for capacity baseline")
	}
	defer func() { _ = database.Close() }()
	database.SetMaxOpenConns(cfg.Concurrency + 2)
	database.SetMaxIdleConns(cfg.Concurrency + 2)

	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		return errors.New("capacity PostgreSQL is unavailable")
	}
	sourceID, rows, err := fixtureIdentity(ctx, database)
	if err != nil {
		return err
	}
	if rows != cfg.ExpectedRows {
		return fmt.Errorf("capacity fixture row count = %d, want %d", rows, cfg.ExpectedRows)
	}
	for range cfg.Warmups {
		if _, err := executeSample(ctx, database, sourceID); err != nil {
			return errors.New("capacity warmup query failed")
		}
	}

	startedAt := time.Now().UTC()
	samples := executeSamples(ctx, database, sourceID, cfg.Concurrency, cfg.Samples)
	completedAt := time.Now().UTC()
	durations := make([]int64, 0, len(samples))
	failures := 0
	for _, measured := range samples {
		if measured.err != nil {
			failures++
			continue
		}
		durations = append(durations, measured.duration.Microseconds())
	}
	sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })
	result := report{
		Version: "hotkey-capacity-baseline-v1", Status: "measured", GitRevision: cfg.GitRevision,
		Environment: cfg.Environment, Hardware: cfg.Hardware,
		Runtime: runtimeFacts{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUs: runtime.NumCPU()},
		Workload: workloadFacts{
			Name: "contents-keyset-page", QueryShape: "published_at_desc_id_desc_limit_50",
			CacheState: cfg.CacheState, PercentileAlgorithm: "nearest-rank-ceiling",
			FixtureRows: cfg.ExpectedRows, Concurrency: cfg.Concurrency, Warmups: cfg.Warmups, Samples: cfg.Samples,
		},
		StartedAt: startedAt, CompletedAt: completedAt, DurationMicros: durations, Errors: failures,
		Exclusions: []string{"fixture_generation", "client_startup", "network_outside_database_connection"},
	}
	if len(durations) > 0 {
		result.P50Micros = nearestRank(durations, 50)
		result.P95Micros = nearestRank(durations, 95)
		result.P99Micros = nearestRank(durations, 99)
	}
	if failures > 0 || len(durations) != cfg.Samples {
		result.Status = "failed"
	}
	if err := writeExclusiveJSON(cfg.Output, result); err != nil {
		return err
	}
	if failures > 0 || len(durations) != cfg.Samples {
		return fmt.Errorf("capacity baseline completed with %d failed samples", failures)
	}
	fmt.Printf("capacity evidence written to %s (%d samples, p95=%d us)\n", cfg.Output, len(durations), result.P95Micros)
	return nil
}

func loadConfig() (config, error) {
	result := config{
		DSN: os.Getenv("HOTKEY_TEST_DSN"), Output: os.Getenv("HOTKEY_CAPACITY_OUTPUT"),
		Environment: os.Getenv("HOTKEY_CAPACITY_ENVIRONMENT"), Hardware: os.Getenv("HOTKEY_CAPACITY_HARDWARE"),
		GitRevision: os.Getenv("HOTKEY_CAPACITY_GIT_REVISION"), CacheState: os.Getenv("HOTKEY_CAPACITY_CACHE_STATE"),
	}
	var err error
	if result.ExpectedRows, err = positiveEnvironmentInteger("HOTKEY_CAPACITY_ROWS", 1000); err != nil {
		return config{}, err
	}
	if result.Concurrency, err = positiveEnvironmentInteger("HOTKEY_CAPACITY_CONCURRENCY", 4); err != nil {
		return config{}, err
	}
	if result.Warmups, err = positiveEnvironmentInteger("HOTKEY_CAPACITY_WARMUPS", 10); err != nil {
		return config{}, err
	}
	if result.Samples, err = positiveEnvironmentInteger("HOTKEY_CAPACITY_SAMPLES", 100); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(result.DSN) == "" || strings.TrimSpace(result.Output) == "" ||
		strings.TrimSpace(result.Environment) == "" || strings.TrimSpace(result.Hardware) == "" {
		return config{}, errors.New("HOTKEY_TEST_DSN, HOTKEY_CAPACITY_OUTPUT, HOTKEY_CAPACITY_ENVIRONMENT and HOTKEY_CAPACITY_HARDWARE are required")
	}
	if len(result.GitRevision) != 40 || strings.Trim(result.GitRevision, "0123456789abcdef") != "" {
		return config{}, errors.New("HOTKEY_CAPACITY_GIT_REVISION must be a 40-character lowercase commit SHA")
	}
	if result.CacheState != "warm" && result.CacheState != "cold" {
		return config{}, errors.New("HOTKEY_CAPACITY_CACHE_STATE must be warm or cold")
	}
	if result.Concurrency > 100 || result.Samples > 100000 || result.Warmups > 10000 {
		return config{}, errors.New("capacity concurrency, samples or warmups exceed the evidence tool bounds")
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

func fixtureIdentity(ctx context.Context, database *sql.DB) (int64, int, error) {
	var sourceID int64
	if err := database.QueryRowContext(ctx, `SELECT id FROM source_connections WHERE name='capacity-fixture-source' AND deleted_at IS NULL`).Scan(&sourceID); err != nil {
		return 0, 0, errors.New("capacity fixture source is missing")
	}
	var rows int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM contents WHERE source_connection_id=$1`, sourceID).Scan(&rows); err != nil {
		return 0, 0, errors.New("count capacity fixture rows")
	}
	return sourceID, rows, nil
}

func executeSamples(ctx context.Context, database *sql.DB, sourceID int64, concurrency, count int) []sample {
	jobs := make(chan struct{})
	results := make(chan sample, count)
	var workers sync.WaitGroup
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range jobs {
				duration, err := executeSample(ctx, database, sourceID)
				results <- sample{duration: duration, err: err}
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
	measured := make([]sample, 0, count)
	for result := range results {
		measured = append(measured, result)
	}
	return measured
}

func executeSample(parent context.Context, database *sql.DB, sourceID int64) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	startedAt := time.Now()
	rows, err := database.QueryContext(ctx, workloadQuery, sourceID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var id int64
		var publishedAt sql.NullTime
		if err := rows.Scan(&id, &publishedAt); err != nil {
			return 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if count != 50 {
		return 0, fmt.Errorf("capacity page row count = %d", count)
	}
	return time.Since(startedAt), nil
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
		return errors.New("capacity output path is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		return errors.New("create capacity evidence directory")
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("capacity evidence file already exists or cannot be created")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		return errors.New("write capacity evidence")
	}
	return nil
}
