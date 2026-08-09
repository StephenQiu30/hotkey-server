package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestRightsManagementSchemaPersistsIdempotencyActorAndDecisionBatchFacts(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	for _, table := range []string{
		"source_rights_policies",
		"source_rights_decision_batches",
		"source_rights_decisions",
		"audit_logs",
	} {
		if _, found := contract.Tables[table]; !found {
			t.Errorf("canonical catalog is missing %s", table)
		}
	}

	schema := strings.ToLower(canonicaldb.SchemaSQL)
	policyBlock := evidenceTableBlock(t, schema, "source_rights_policies")
	for _, required := range []string{
		"recorded_by_user_id bigint not null",
		"idempotency_key varchar(128) not null",
		"command_fingerprint char(64) not null",
	} {
		if !strings.Contains(policyBlock, required) {
			t.Errorf("rights policy is missing command receipt fact %q", required)
		}
	}

	batchBlock := evidenceTableBlock(t, schema, "source_rights_decision_batches")
	for _, required := range []string{
		"source_connection_id bigint not null",
		"policy_id bigint not null",
		"expected_policy_version bigint not null",
		"subject_type varchar(32) not null",
		"subject_key varchar(512) not null",
		"input_digest char(64) not null",
		"recorded_by_user_id bigint not null",
		"idempotency_key varchar(128) not null",
		"command_fingerprint char(64) not null",
		"decision_count smallint not null",
	} {
		if !strings.Contains(batchBlock, required) {
			t.Errorf("rights decision batch is missing immutable fact %q", required)
		}
	}

	decisionBlock := evidenceTableBlock(t, schema, "source_rights_decisions")
	for _, required := range []string{
		"decision_batch_id bigint not null",
		"policy_revision bigint not null",
	} {
		if !strings.Contains(decisionBlock, required) {
			t.Errorf("rights decision is missing semantic lineage fact %q", required)
		}
	}
	if strings.Contains(decisionBlock, "policy_version") {
		t.Error("rights decision uses ambiguous policy_version instead of policy_revision")
	}

	auditBlock := evidenceTableBlock(t, schema, "audit_logs")
	for _, required := range []string{
		"idempotency_key varchar(128)",
		"command_fingerprint char(64)",
	} {
		if !strings.Contains(auditBlock, required) {
			t.Errorf("audit log is missing idempotent mutation fact %q", required)
		}
	}

	for name, snippet := range map[string]string{
		"policy actor authority trigger":      "source_rights_policies_actor_authority",
		"decision actor authority trigger":    "source_rights_decision_batches_actor_authority",
		"policy idempotency uniqueness":       "source_rights_policies_idempotency_uq",
		"batch idempotency uniqueness":        "source_rights_decision_batches_idempotency_uq",
		"batch action uniqueness":             "unique (decision_batch_id, action)",
		"batch attribution binding":           "source_rights_decisions_batch_binding",
		"batch completeness deferred trigger": "source_rights_decision_batches_complete",
		"audit idempotency uniqueness":        "audit_logs_idempotency_uq",
	} {
		if !strings.Contains(schema, snippet) {
			t.Errorf("rights management schema is missing %s: %q", name, snippet)
		}
	}
}
