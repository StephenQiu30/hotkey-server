package bootstrap

import "testing"

func TestParseRawEvidenceRetentionFlagsRequiresExplicitBoundedApply(t *testing.T) {
	command, err := parseRawEvidenceRetentionFlags([]string{"--batch-size", "25", "--apply", "--confirm-delete"})
	if err != nil {
		t.Fatal(err)
	}
	if command.BatchSize != 25 || !command.Apply || !command.ConfirmDelete {
		t.Fatalf("unexpected raw evidence retention command: %#v", command)
	}
	for _, args := range [][]string{
		{"--batch-size", "0", "--apply", "--confirm-delete"},
		{"--batch-size", "101", "--apply", "--confirm-delete"},
		{"--batch-size", "1", "--confirm-delete"},
		{"--batch-size", "1", "--apply"},
	} {
		if _, err := parseRawEvidenceRetentionFlags(args); err == nil {
			t.Fatalf("parseRawEvidenceRetentionFlags(%v) error=nil", args)
		}
	}
}

func TestEvidenceLineageBackfillFlagsSeparateDryRunFromConfirmedApply(t *testing.T) {
	dryRun, err := parseEvidenceLineageBackfillFlags([]string{"--phase", "source", "--batch-size", "200", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !dryRun.DryRun || dryRun.Apply || dryRun.ConfirmNonEmpty {
		t.Fatalf("dry-run = %+v", dryRun)
	}

	apply, err := parseEvidenceLineageBackfillFlags([]string{
		"--phase", "source", "--batch-size", "200", "--apply", "--confirm-non-empty",
		"--operator-id", "operator-a", "--reviewer-id", "reviewer-b",
		"--backup-evidence-sha256", repeatMaintenanceFlagSHA("a"),
		"--rehearsal-evidence-sha256", repeatMaintenanceFlagSHA("b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !apply.Apply || apply.DryRun || !apply.ConfirmNonEmpty || apply.OperatorID != "operator-a" || apply.ReviewerID != "reviewer-b" {
		t.Fatalf("apply = %+v", apply)
	}
}

func TestEvidenceLineageBackfillFlagsRejectAmbiguousOrUnprovenMutation(t *testing.T) {
	for _, args := range [][]string{
		{"--phase", "source", "--dry-run", "--apply"},
		{"--phase", "source", "--apply", "--confirm-non-empty"},
		{"--phase", "unknown", "--dry-run"},
		{"--phase", "source", "--dry-run", "--resume", "--run-id", "1"},
	} {
		if _, err := parseEvidenceLineageBackfillFlags(args); err == nil {
			t.Fatalf("parseEvidenceLineageBackfillFlags(%v) error=nil", args)
		}
	}
}

func TestEvidenceLineageReconciliationFlagsSeparateReadOnlyInspectionFromConfirmedRepair(t *testing.T) {
	dryRun, err := parseEvidenceLineageReconciliationFlags([]string{
		"--scope", "all", "--batch-size", "200", "--grace-period-hours", "24", "--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dryRun.DryRun || dryRun.Apply || dryRun.Scope != "all" || dryRun.GracePeriodHours != 24 {
		t.Fatalf("dry-run = %+v", dryRun)
	}

	apply, err := parseEvidenceLineageReconciliationFlags([]string{
		"--scope", "pg-minio", "--batch-size", "20", "--grace-period-hours", "48",
		"--apply", "--confirm-non-empty", "--operator-id", "operator-a", "--reviewer-id", "reviewer-b",
		"--backup-evidence-sha256", repeatMaintenanceFlagSHA("c"),
		"--rehearsal-evidence-sha256", repeatMaintenanceFlagSHA("d"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !apply.Apply || apply.DryRun || !apply.ConfirmNonEmpty || apply.Scope != "pg-minio" || apply.BatchSize != 20 {
		t.Fatalf("apply = %+v", apply)
	}
}

func TestEvidenceLineageReconciliationFlagsRejectUnsafeOrAmbiguousRepair(t *testing.T) {
	for _, args := range [][]string{
		{"--scope", "all", "--dry-run", "--apply"},
		{"--scope", "all", "--apply", "--confirm-non-empty"},
		{"--scope", "unknown", "--dry-run"},
		{"--scope", "all", "--grace-period-hours", "0", "--dry-run"},
		{"--scope", "all", "--dry-run", "--operator-id", "operator-a"},
		{"--scope", "all", "--apply", "--confirm-non-empty", "--operator-id", "same", "--reviewer-id", "same", "--backup-evidence-sha256", repeatMaintenanceFlagSHA("e"), "--rehearsal-evidence-sha256", repeatMaintenanceFlagSHA("f")},
	} {
		if _, err := parseEvidenceLineageReconciliationFlags(args); err == nil {
			t.Fatalf("parseEvidenceLineageReconciliationFlags(%v) error=nil", args)
		}
	}
}

func repeatMaintenanceFlagSHA(value string) string {
	result := ""
	for range 64 {
		result += value
	}
	return result
}
