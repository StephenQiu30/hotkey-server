package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNearestRankUsesCeilingOverSortedRawSamples(t *testing.T) {
	values := make([]int64, 100)
	for index := range values {
		values[index] = int64(index + 1)
	}
	for percentile, want := range map[int]int64{50: 50, 95: 95, 99: 99, 100: 100} {
		if got := nearestRank(values, percentile); got != want {
			t.Errorf("nearestRank(%d) = %d, want %d", percentile, got, want)
		}
	}
	if got := nearestRank([]int64{11, 22, 33}, 50); got != 22 {
		t.Fatalf("nearestRank(three, 50) = %d, want 22", got)
	}
	if got := nearestRank(nil, 95); got != 0 {
		t.Fatalf("nearestRank(nil, 95) = %d, want 0", got)
	}
}

func TestCapacityConfigRequiresReproducibilityMetadata(t *testing.T) {
	for key, value := range map[string]string{
		"HOTKEY_TEST_DSN":              "postgres://fixture.invalid/hotkey",
		"HOTKEY_CAPACITY_OUTPUT":       filepath.Join(t.TempDir(), "result.json"),
		"HOTKEY_CAPACITY_ENVIRONMENT":  "isolated-ci",
		"HOTKEY_CAPACITY_HARDWARE":     "4 cpu / 8 GiB / local SSD",
		"HOTKEY_CAPACITY_GIT_REVISION": "0123456789abcdef0123456789abcdef01234567",
		"HOTKEY_CAPACITY_CACHE_STATE":  "warm",
	} {
		t.Setenv(key, value)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if got.ExpectedRows != 1000 || got.Concurrency != 4 || got.Warmups != 10 || got.Samples != 100 {
		t.Fatalf("loadConfig() defaults = %#v", got)
	}
	t.Setenv("HOTKEY_CAPACITY_HARDWARE", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted missing hardware metadata")
	}
}

func TestCapacityEvidenceCannotOverwriteExistingRun(t *testing.T) {
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
