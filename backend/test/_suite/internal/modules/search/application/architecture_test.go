package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestP0SearchArchitectureUsesOnlyPostgresLexicalPaths(t *testing.T) {
	productionFiles := []string{
		"service.go",
		filepath.Join("..", "domain", "query.go"),
		filepath.Join("..", "transport", "http", "handler.go"),
		filepath.Join("..", "transport", "http", "routes.go"),
		filepath.Join("..", "..", "ingestion", "infrastructure", "postgres", "lexical_search.go"),
		filepath.Join("..", "..", "event", "infrastructure", "postgres", "lexical_search.go"),
		filepath.Join("..", "..", "knowledge", "infrastructure", "postgres", "lexical_search.go"),
	}
	contents := make(map[string]string, len(productionFiles))
	for _, path := range productionFiles {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read production search path %s: %v", path, err)
		}
		contents[path] = strings.ToLower(string(body))
	}
	for path, body := range contents {
		for _, forbidden := range []string{
			"document_version_embeddings", "content_embeddings", "event_embeddings", "topic_embeddings", "monitor_embeddings",
			"internal/modules/intelligence", "infrastructure/provider", "elasticsearch", "opensearch", "pgvector", "halfvec", "nearestembeddings",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("P0 search path %s contains forbidden dependency %q", path, forbidden)
			}
		}
	}
	contentSQL := contents[filepath.Join("..", "..", "ingestion", "infrastructure", "postgres", "lexical_search.go")]
	for _, required := range []string{"document_version_search_indexes", "websearch_to_tsquery", "similarity(", "current_rights_action_allowed"} {
		if !strings.Contains(contentSQL, required) {
			t.Fatalf("content lexical query missing %q", required)
		}
	}
	for _, path := range productionFiles[len(productionFiles)-2:] {
		for _, required := range []string{"websearch_to_tsquery", "similarity("} {
			if !strings.Contains(contents[path], required) {
				t.Fatalf("lexical query %s missing %q", path, required)
			}
		}
	}
	getRoutes := contents[filepath.Join("..", "transport", "http", "routes.go")]
	if !strings.Contains(getRoutes, `api.get(""`) {
		t.Fatal("persisted search GET route is not the default read path")
	}
	instantRoute, err := os.ReadFile(filepath.Join("..", "..", "source", "transport", "http", "instant_search.go"))
	if err != nil || !strings.Contains(strings.ToLower(string(instantRoute)), "@router /api/v1/search [post]") {
		t.Fatalf("existing external instant-search POST route was not preserved: %v", err)
	}
}
