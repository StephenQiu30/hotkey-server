package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestP0LexicalRecallUsesOnlyAuditablePostgresFTS(t *testing.T) {
	root := repositoryRoot(t)
	reader := readRepositoryFile(t, root, "internal/modules/ingestion/infrastructure/postgres/hybrid_recall_reader.go")
	lexicalMethod := sourceSection(t, reader,
		"func (reader *HybridDocumentRecallReader) RecallLexical",
		"func (reader *HybridDocumentRecallReader) RecallStructured",
	)
	lexicalSQL := sourceSection(t, reader,
		"const lexicalDocumentRecallSQL",
		"const structuredDocumentRecallSQL",
	)

	for _, fragment := range []string{
		"lexicalDocumentRecallSQL",
		"compileDocumentRecallFilters",
	} {
		if !strings.Contains(lexicalMethod, fragment) {
			t.Errorf("lexical recall method is missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"document_version_search_indexes",
		"plainto_tsquery('simple'",
		"ts_rank_cd(",
		"show_trgm(",
		"current_rights_action_allowed(",
		"search.lifecycle_state='active'",
		"search.retention_until>now()",
		"ORDER BY raw_score DESC,document_version_id ASC",
	} {
		if !strings.Contains(lexicalSQL, fragment) {
			t.Errorf("lexical PostgreSQL search contract is missing %q", fragment)
		}
	}

	for _, forbidden := range []string{
		"document_version_embeddings",
		"content_embeddings",
		"halfvec",
		"<=>",
		"pgvector",
		"ai_model_profiles",
		"semanticDocument",
	} {
		if strings.Contains(lexicalMethod, forbidden) || strings.Contains(lexicalSQL, forbidden) {
			t.Errorf("P0 lexical recall depends on forbidden semantic/vector path %q", forbidden)
		}
	}
}

func TestP0FTSProjectionAndSchemaRemainRebuildableAndIndexed(t *testing.T) {
	root := repositoryRoot(t)
	writer := readRepositoryFile(t, root, "internal/modules/ingestion/infrastructure/postgres/document_recall_projection_writer.go")
	projectionSQL := sourceSection(t, writer,
		"const persistDocumentSearchProjectionSQL",
		"const persistDocumentEmbeddingReceiptSQL",
	)
	for _, fragment := range []string{
		"to_tsvector('simple'",
		"show_trgm(",
		"INSERT INTO document_version_search_indexes",
		"normalization_profile_version",
	} {
		if !strings.Contains(projectionSQL, fragment) {
			t.Errorf("rebuildable FTS projection is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"document_version_embeddings", "halfvec", "pgvector"} {
		if strings.Contains(projectionSQL, forbidden) {
			t.Errorf("P0 FTS projection depends on forbidden vector path %q", forbidden)
		}
	}

	schema := readRepositoryFile(t, root, "db/schema.sql")
	for _, fragment := range []string{
		"title_search_vector tsvector",
		"body_search_vector tsvector",
		"document_version_search_title_fts_idx",
		"document_version_search_body_fts_idx",
		"USING gin (title_search_vector) WHERE lifecycle_state = 'active'",
		"USING gin (body_search_vector) WHERE lifecycle_state = 'active'",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("PostgreSQL FTS schema is missing %q", fragment)
		}
	}
}

func TestExistingEmbeddingPathRemainsAnExplicitMigrationInventory(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"docs/prd/001-HotKey产品需求分析与总体架构.md": {
			"这些是迁移现状，不表示已经清理",
			"不在本 PRD 中宣称现有 Provider、Embedding 或 pgvector 已删除",
		},
		"docs/plans/001-HotKey产品需求分析与总体架构计划.md": {
			"现有 Embedding 路径仅标记为待迁移，不在此任务删除",
			"删除另行评审",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s no longer records the controlled Embedding migration: missing %q", relative, fragment)
			}
		}
	}
}

func sourceSection(t *testing.T, source, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("source is missing start marker %q", startMarker)
	}
	end := strings.Index(source[start:], endMarker)
	if end < 0 {
		t.Fatalf("source is missing end marker %q after %q", endMarker, startMarker)
	}
	return source[start : start+end]
}
