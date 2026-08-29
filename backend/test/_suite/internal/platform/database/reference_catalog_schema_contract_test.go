package database

import (
	"strings"
	"testing"

	canonicaldb "github.com/StephenQiu30/hotkey-server/backend/db"
)

func TestRetentionPoliciesAreAFixedSevenItemReferenceCatalog(t *testing.T) {
	normalized := strings.Join(strings.Fields(strings.ToLower(canonicaldb.SchemaSQL)), " ")
	want := "constraint retention_policies_data_class_check check (data_class in ('captured_items','content_metric_snapshots','event_metric_snapshots','sessions','delivery_attempts','job_attempts','audit_logs'))"
	if !strings.Contains(normalized, want) {
		t.Fatalf("retention policy schema is missing fixed seven-item catalog constraint %q", want)
	}

	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract(): %v", err)
	}
	retentionPolicies := contract.Tables["retention_policies"]
	if got, want := retentionPolicies.Constraints.Check, 3; got != want {
		t.Fatalf("retention_policies CHECK count = %d, want %d", got, want)
	}
}
