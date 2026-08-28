package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNearestRankUsesCeilingOverSortedSearchSamples(t *testing.T) {
	values := make([]int64, 100)
	for index := range values {
		values[index] = int64(index + 1)
	}
	for percentile, want := range map[int]int64{50: 50, 95: 95, 99: 99, 100: 100} {
		if got := nearestRank(values, percentile); got != want {
			t.Errorf("nearestRank(%d) = %d, want %d", percentile, got, want)
		}
	}
	if got := nearestRank(nil, 95); got != 0 {
		t.Fatalf("nearestRank(nil, 95) = %d, want 0", got)
	}
}

func TestSearchCapacityConfigRequiresReproducibilityMetadata(t *testing.T) {
	for key, value := range map[string]string{
		"HOTKEY_TEST_DSN":                     "postgres://fixture.invalid/hotkey",
		"HOTKEY_SEARCH_CAPACITY_OUTPUT":       filepath.Join(t.TempDir(), "result.json"),
		"HOTKEY_SEARCH_CAPACITY_ENVIRONMENT":  "isolated-ci",
		"HOTKEY_SEARCH_CAPACITY_HARDWARE":     "4 cpu / 8 GiB / local SSD",
		"HOTKEY_SEARCH_CAPACITY_GIT_REVISION": "0123456789abcdef0123456789abcdef01234567",
		"HOTKEY_SEARCH_CAPACITY_CACHE_STATE":  "warm",
	} {
		t.Setenv(key, value)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if got.RowsPerResource != 1000 || got.Concurrency != 4 || got.Warmups != 10 || got.Samples != 120 {
		t.Fatalf("loadConfig() defaults = %#v", got)
	}
	t.Setenv("HOTKEY_SEARCH_CAPACITY_HARDWARE", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted missing hardware metadata")
	}
	t.Setenv("HOTKEY_SEARCH_CAPACITY_HARDWARE", "/Users/private/search-runner")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted a host path in report metadata")
	}
	t.Setenv("HOTKEY_SEARCH_CAPACITY_HARDWARE", "4 cpu / 8 GiB / local SSD")
	t.Setenv("HOTKEY_SEARCH_CAPACITY_SAMPLES", "4")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted fewer samples than the fixed query set")
	}
}

func TestSanitizePlanKeepsOnlyLowCardinalityStructure(t *testing.T) {
	raw := []byte(`[{"Plan":{"Node Type":"Limit","Plans":[{"Node Type":"Bitmap Heap Scan","Relation Name":"contents","Alias":"private-title-sentinel","Filter":"body = 'private-body-sentinel'","Plans":[{"Node Type":"Bitmap Index Scan","Index Name":"contents_search_active_trgm_idx","Index Cond":"private-query-sentinel"}]}]}}]`)
	nodes, err := sanitizePlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []planNode{
		{Depth: 0, NodeType: "Limit"},
		{Depth: 1, NodeType: "Bitmap Heap Scan", Relation: "contents"},
		{Depth: 2, NodeType: "Bitmap Index Scan", Index: "contents_search_active_trgm_idx"},
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Fatalf("sanitizePlan() = %#v, want %#v", nodes, want)
	}
	payload, err := json.Marshal(nodes)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"private-title-sentinel", "private-body-sentinel", "private-query-sentinel", "Filter", "Index Cond"} {
		if strings.Contains(string(payload), sentinel) {
			t.Fatalf("sanitized plan leaked %q: %s", sentinel, payload)
		}
	}
}

func TestResultIdentityDigestIsStableAndOrderSensitive(t *testing.T) {
	left := []resultIdentity{{ResourceType: "content", ID: 1}, {ResourceType: "event", ID: 2}}
	right := []resultIdentity{{ResourceType: "event", ID: 2}, {ResourceType: "content", ID: 1}}
	if resultIdentityDigest(left) != resultIdentityDigest(append([]resultIdentity(nil), left...)) {
		t.Fatal("equivalent ordered results produced different digests")
	}
	if resultIdentityDigest(left) == resultIdentityDigest(right) {
		t.Fatal("different result order produced the same digest")
	}
}

func TestSearchCapacityEvidenceCannotOverwriteExistingRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence", "baseline.json")
	want := map[string]string{"status": "measured"}
	if err := writeExclusiveJSON(path, want); err != nil {
		t.Fatalf("writeExclusiveJSON(first) error = %v", err)
	}
	if err := writeExclusiveJSON(path, want); err == nil {
		t.Fatal("writeExclusiveJSON(second) overwrote immutable evidence")
	}
	payload, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(string(payload), "{\n  \"status\": \"measured\"\n}\n") {
		t.Fatalf("evidence payload = %q / %v", payload, err)
	}
}

func TestSearchCapacityEvidenceOmitsPrivateSearchTextAndHostPaths(t *testing.T) {
	payload, err := json.Marshal(report{
		Version:  searchCapacityVersion,
		Status:   "measured",
		Approval: "required",
		API: apiEvidence{Queries: []queryEvidence{{
			ID: "fixed-query", QueryDigest: strings.Repeat("a", 64),
			ResultIdentityDigest: strings.Repeat("b", 64),
		}}},
		Plans: []planEvidence{{ResourceType: "content", PlanDigest: strings.Repeat("c", 64)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"query_text"`, `"title"`, `"snippet"`, `"body"`, `"host_path"`,
		"private-query-sentinel", "private-title-sentinel", "private-body-sentinel", "/Users/",
	} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("search capacity evidence leaked %q: %s", forbidden, payload)
		}
	}
}
