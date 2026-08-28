package architecture_test

import (
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
