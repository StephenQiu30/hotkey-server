package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestTextQuoteSelectorSchemaBindsExactDocumentArtifactsAndQuoteRights(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	if _, found := contract.Tables["document_text_quote_selectors"]; !found {
		t.Fatal("canonical catalog is missing document_text_quote_selectors")
	}
	schema := strings.ToLower(canonicaldb.SchemaSQL)
	block := documentMatchTableBlock(t, schema, "document_text_quote_selectors")
	for name, snippet := range map[string]string{
		"exact document version":    "document_version_id bigint not null references document_versions(id)",
		"exact plaintext artifact":  "plaintext_artifact_id bigint not null references derived_artifacts(id)",
		"exact markdown artifact":   "markdown_artifact_id bigint not null references derived_artifacts(id)",
		"quote decision":            "quote_rights_decision_id bigint not null",
		"retain decision":           "retain_rights_decision_id bigint not null",
		"bounded exact quote":       "octet_length(exact_quote) between 1 and 4096",
		"UTF-8 position":            "utf8_byte_end > utf8_byte_start",
		"quote digest":              "quote_sha256 char(64) not null",
		"plaintext identity":        "plaintext_sha256 char(64) not null",
		"normalization version":     "normalization_version varchar(64) not null",
		"selector version":          "selector_version varchar(96) not null",
		"deterministic idempotency": "unique (document_version_id, plaintext_sha256, utf8_byte_start, utf8_byte_end, quote_sha256)",
	} {
		if !strings.Contains(block, snippet) {
			t.Errorf("missing %s contract: %q", name, snippet)
		}
	}
	for _, snippet := range []string{
		"create trigger document_text_quote_selectors_integrity",
		"create trigger document_text_quote_selectors_immutable",
		"current_rights_action_allowed(",
		"'quote'",
		"'retain'",
	} {
		if !strings.Contains(schema, snippet) {
			t.Errorf("missing selector safety contract %q", snippet)
		}
	}
}
