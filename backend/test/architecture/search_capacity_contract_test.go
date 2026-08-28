package architecture_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchCapacityEvidenceUsesIsolatedFixedCorpusAndProductionOwners(t *testing.T) {
	root := repositoryRoot(t)
	fixture := readRepositoryFile(t, root, "test/tools/generate-search-capacity-fixture.sh")
	for _, fragment := range []string{
		"BEGIN;",
		"search-capacity-source",
		"source_rights_policies",
		"source_rights_decision_batches",
		"source_rights_decisions",
		"document_version_search_indexes",
		"clustering_profile_version",
		"search-capacity-v1",
		"events/search-capacity-",
		"ANALYZE contents",
		"ANALYZE micro_events",
		"ANALYZE knowledge_change_proposals",
		"use a fresh isolated database",
	} {
		if !strings.Contains(fixture, fragment) {
			t.Errorf("search capacity fixture is missing %q", fragment)
		}
	}
	if strings.Contains(fixture, "session_replication_role") {
		t.Fatal("search capacity fixture bypasses production integrity triggers")
	}

	baseline := readRepositoryFile(t, root, "test/tools/search-capacity-baseline/main.go")
	for _, fragment := range []string{
		"ingestionpostgres.NewContentRepository",
		"eventpostgres.NewMicroEventQueryPostgresRepository",
		"knowledgepostgres.NewRepository",
		"Status: \"measured\"",
		"Approval: \"required\"",
		"writeExclusiveJSON",
		"query_text_title_snippet_body_and_host_paths_intentionally_omitted",
	} {
		if !strings.Contains(baseline, fragment) {
			t.Errorf("search capacity baseline is missing %q", fragment)
		}
	}
	owners := strings.Join([]string{
		readRepositoryFile(t, root, "internal/modules/ingestion/infrastructure/postgres/lexical_search.go"),
		readRepositoryFile(t, root, "internal/modules/event/infrastructure/postgres/lexical_search.go"),
		readRepositoryFile(t, root, "internal/modules/knowledge/infrastructure/postgres/lexical_search.go"),
	}, "\n")
	if strings.Count(owners, "EXPLAIN (FORMAT JSON,COSTS FALSE)") != 3 {
		t.Fatal("all three search owners must expose non-ANALYZE JSON plans for their exact production query")
	}
	for _, forbidden := range []string{"EXPLAIN ANALYZE", "Elasticsearch", "vector.New", "pgvector"} {
		if strings.Contains(baseline, forbidden) {
			t.Errorf("search capacity baseline contains forbidden dependency %q", forbidden)
		}
	}
	if strings.Contains(baseline, "evidence written to %s") {
		t.Fatal("search capacity command output exposes its host evidence path")
	}

	makefile := readRepositoryFile(t, root, "Makefile")
	for _, fragment := range []string{
		"search-capacity-fixture: test-env",
		"sh test/tools/generate-search-capacity-fixture.sh",
		"search-capacity-baseline: test-env",
		"$(GO) run ./test/tools/search-capacity-baseline",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("Makefile search capacity contract is missing %q", fragment)
		}
	}
}

func TestCommittedSearchCapacityEvidenceRemainsMeasuredAndSanitized(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	payload := readRepositoryFile(t, repository, "docs/acceptance/evidence/004/search-capacity-macos-arm64-53ffaf4b.json")
	var evidence struct {
		Version     string `json:"version"`
		Status      string `json:"status"`
		Approval    string `json:"approval"`
		GitRevision string `json:"git_revision"`
		CacheState  string `json:"cache_state"`
		Corpus      struct {
			ContentRows   int `json:"content_rows"`
			EventRows     int `json:"event_rows"`
			KnowledgeRows int `json:"knowledge_rows"`
			TotalRows     int `json:"total_rows"`
		} `json:"corpus"`
		API struct {
			Concurrency int `json:"concurrency"`
			Warmups     int `json:"warmups"`
			Latency     struct {
				Samples   int   `json:"samples"`
				Errors    int   `json:"errors"`
				P50Micros int64 `json:"p50_micros"`
				P95Micros int64 `json:"p95_micros"`
				P99Micros int64 `json:"p99_micros"`
			} `json:"latency"`
			Queries []struct {
				Latency struct {
					Samples int `json:"samples"`
					Errors  int `json:"errors"`
				} `json:"latency"`
			} `json:"queries"`
		} `json:"api"`
		IndexUpdate struct {
			Visible       bool  `json:"visible"`
			Attempts      int   `json:"attempts"`
			LatencyMicros int64 `json:"latency_micros"`
		} `json:"index_update"`
		Plans []struct {
			ResourceType string `json:"resource_type"`
		} `json:"query_plans"`
	}
	if err := json.Unmarshal([]byte(payload), &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Version != searchCapacityEvidenceVersion || evidence.Status != "measured" || evidence.Approval != "required" ||
		evidence.GitRevision != "53ffaf4b19dc901aeda32474d821648b77a166f1" || evidence.CacheState != "warm" {
		t.Fatalf("search capacity evidence identity = %#v", evidence)
	}
	if evidence.Corpus.ContentRows != 1000 || evidence.Corpus.EventRows != 1000 ||
		evidence.Corpus.KnowledgeRows != 1000 || evidence.Corpus.TotalRows != 3000 {
		t.Fatalf("search capacity corpus = %#v", evidence.Corpus)
	}
	if evidence.API.Concurrency != 4 || evidence.API.Warmups != 10 || evidence.API.Latency.Samples != 120 ||
		evidence.API.Latency.Errors != 0 || evidence.API.Latency.P50Micros <= 0 ||
		evidence.API.Latency.P95Micros <= 0 || evidence.API.Latency.P99Micros <= 0 ||
		evidence.API.Latency.P95Micros > 800000 || len(evidence.API.Queries) != 5 {
		t.Fatalf("search capacity API evidence = %#v", evidence.API)
	}
	for _, query := range evidence.API.Queries {
		if query.Latency.Samples != 24 || query.Latency.Errors != 0 {
			t.Fatalf("search capacity query distribution = %#v", query.Latency)
		}
	}
	if !evidence.IndexUpdate.Visible || evidence.IndexUpdate.Attempts <= 0 || evidence.IndexUpdate.LatencyMicros <= 0 || len(evidence.Plans) != 3 {
		t.Fatalf("search capacity index/plan evidence = %#v / %#v", evidence.IndexUpdate, evidence.Plans)
	}
	for index, want := range []string{"content", "event", "knowledge"} {
		if evidence.Plans[index].ResourceType != want {
			t.Fatalf("search capacity plan %d = %#v, want %q", index, evidence.Plans[index], want)
		}
	}
	for _, forbidden := range []string{
		`"query_text"`, `"title"`, `"snippet"`, `"body"`, `"host_path"`,
		"/Users/", "/home/", "postgres://", "password", "private-query-sentinel",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("committed search capacity evidence leaked %q", forbidden)
		}
	}
}

const searchCapacityEvidenceVersion = "hotkey-search-capacity-v1"
