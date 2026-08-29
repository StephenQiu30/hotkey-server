package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestP0UserListCursorsUseSignedExpiringCodec(t *testing.T) {
	root := repositoryRoot(t)
	contracts := map[string][]string{
		"internal/modules/event/infrastructure/postgres/micro_event_query_repository.go": {
			`codec.Seal("micro_event_list"`, `codec.Open(value, "micro_event_list"`,
			`codec.Seal("micro_event_evidence_list"`, `codec.Open(value, "micro_event_evidence_list"`,
		},
		"internal/modules/ingestion/transport/http/relevance.go": {
			`codec.Seal("relevance_match_list"`, `codec.Open(raw, "relevance_match_list"`,
			`codec.Seal("relevance_suggestion_list"`, `codec.Open(raw, "relevance_suggestion_list"`,
		},
		"internal/modules/ingestion/infrastructure/postgres/repository.go": {
			"NewContentRepositoryWithCursorCodec", `cursorCodec.Seal("content_list"`, `cursorCodec.Open(query.Cursor, "content_list"`,
		},
		"internal/modules/ingestion/infrastructure/postgres/document_match_query_repository.go": {
			`document-match-list-v1:%d:%s`, `cursorCodec.Decode(query.Cursor, "document_match_decision_id", true, fingerprint)`,
			`"document_match_decision_id", true, fingerprint`,
		},
		"internal/modules/monitor/infrastructure/postgres/repository.go": {
			"NewRepositoryWithCursorCodec", `cursorCodec.Seal("monitor_list"`, `cursorCodec.Open(query.Cursor, "monitor_list"`,
		},
		"internal/modules/source/infrastructure/postgres/repository.go": {
			"NewRepositoryWithCursorCodec", `cursorCodec.Seal("source_connection_list"`, `cursorCodec.Open(query.Cursor, "source_connection_list"`,
		},
		"internal/modules/source/infrastructure/postgres/collection_repository.go": {
			"NewCollectionRepositoryWithCursorCodec", `cursorCodec.Seal("collection_run_list"`, `cursorCodec.Open(query.Cursor, "collection_run_list"`,
			`cursorCodec.Seal("captured_item_list"`, `cursorCodec.Open(query.Cursor, "captured_item_list"`,
		},
		"internal/modules/source/infrastructure/postgres/rights_management_projection_repository.go": {
			`rightsProjectionPageParameters("policies"`, `rightsProjectionPageParameters("decision-batches"`,
			`cursorCodec.Decode(encodedCursor, "id", true, fingerprint)`, `cursorCodec.Encode("id", true, fingerprint`,
		},
		"internal/modules/report/infrastructure/postgres/repository.go": {
			"NewRepositoryWithCursorCodec", `cursorCodec.Decode(query.Cursor, "id", true, reportListFingerprint(query))`,
			`cursorCodec.Encode("id", true, reportListFingerprint(query)`,
		},
		"internal/modules/search/application/service.go": {
			`cursorCodec.Seal("search_result_list"`, `cursorCodec.Open(value, "search_result_list"`,
		},
		"internal/modules/operations/infrastructure/postgres/jobs.go": {
			"NewJobRepositoryWithCursorCodec", `cursorCodec.Seal("operations_job_list"`, `cursorCodec.Open(value, "operations_job_list"`,
		},
		"internal/modules/operations/infrastructure/postgres/governance_repository.go": {
			"NewGovernanceRepositoryWithCursorCodec", `cursorCodec.Seal("operations_audit_list"`, `cursorCodec.Open(value, "operations_audit_list"`,
		},
		"internal/shared/pagination/cursor.go": {
			"hmac.Equal", "ErrExpiredCursor", "ErrStaleCursor", "maximumEncodedCursorSize",
		},
		"internal/bootstrap/pagination.go": {
			"pagination.NewCodec", "NewRepositoryWithCursorCodec", "NewMicroEventQueryPostgresRepositoryWithCursorCodec", "newRightsManagementRepository", "newContentRepository", "newReportRepository", "newJobRepository", "newGovernanceRepository",
		},
		"internal/bootstrap/monitor_intent_expansion.go": {
			"newDocumentMatchRepository", "NewDocumentMatchRepositoryWithCursorCodec",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, root, filepath.ToSlash(relative))
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing signed cursor contract %q", relative, fragment)
			}
		}
		if relative != "internal/shared/pagination/cursor.go" && strings.Contains(payload, "base64.RawURLEncoding") {
			t.Errorf("%s reintroduced an unsigned Base64 user cursor", relative)
		}
	}

	bootstrapRoot := filepath.Join(root, "internal", "bootstrap")
	if violations := productionGoFilesContaining(t, bootstrapRoot, []string{"pagination.NewTestCodec"}); len(violations) > 0 {
		t.Fatalf("production bootstrap uses test-only cursor keys: %s", strings.Join(violations, ", "))
	}
}
