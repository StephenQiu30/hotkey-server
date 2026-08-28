package bootstrap

import (
	"strings"
	"testing"
)

func TestParseProjectionRecoveryFlagsKeepsDryRunReadOnly(t *testing.T) {
	command, err := parseProjectionRecoveryFlags([]string{"--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !command.DryRun || command.Apply || command.ConfirmIsolated || command.ProductionEgressDisabled {
		t.Fatalf("command=%+v", command)
	}
	if _, err := parseProjectionRecoveryFlags([]string{"--dry-run", "--operator-id", "operator-a"}); err == nil {
		t.Fatal("dry-run accepted mutation evidence")
	}
}

func TestParseProjectionRecoveryFlagsRequiresIsolatedDualControlApply(t *testing.T) {
	hash := strings.Repeat("a", 64)
	arguments := []string{
		"--apply", "--confirm-isolated", "--production-egress-disabled",
		"--operator-id", "operator-a", "--reviewer-id", "reviewer-b",
		"--run-sha256", hash, "--backup-evidence-sha256", hash, "--rehearsal-evidence-sha256", hash,
	}
	command, err := parseProjectionRecoveryFlags(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if !command.Apply || !command.ConfirmIsolated || !command.ProductionEgressDisabled || command.OperatorID == command.ReviewerID {
		t.Fatalf("command=%+v", command)
	}
	if _, err := parseProjectionRecoveryFlags(arguments[1:]); err == nil {
		t.Fatal("apply accepted without --apply")
	}
	arguments[6] = "operator-a"
	if _, err := parseProjectionRecoveryFlags(arguments); err == nil {
		t.Fatal("apply accepted the same operator and reviewer")
	}
}
