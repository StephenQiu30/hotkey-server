package main

import (
	"strings"
	"testing"
	"time"
)

func TestVerifyRecoveryEvidenceRequiresAllFactsAndComputesMeasuredTargets(t *testing.T) {
	evidence := validRecoveryManifest()
	result, err := verify(evidence)
	if err != nil {
		t.Fatalf("verify() error = %v", err)
	}
	if result.Status != "reconciled" || result.RPOSeconds != 10*60 || result.RTOSeconds != 90*60 ||
		!result.CandidateRPOMet || !result.CandidateRTOMet || len(result.Assets) != 5 || result.Assets[0].Name != "minio_evidence" {
		t.Fatalf("verify() = %#v", result)
	}
}

func TestVerifyRecoveryEvidenceRejectsMissingManualVaultAndUnexplainedDifference(t *testing.T) {
	evidence := validRecoveryManifest()
	evidence.Assets = evidence.Assets[:4]
	if _, err := verify(evidence); err == nil || !strings.Contains(err.Error(), "vault_manual_regions") {
		t.Fatalf("verify(missing manual Vault) error = %v", err)
	}

	evidence = validRecoveryManifest()
	evidence.Differences = []string{"one River attempt was not reconciled"}
	if _, err := verify(evidence); err == nil || !strings.Contains(err.Error(), "unexplained differences") {
		t.Fatalf("verify(difference) error = %v", err)
	}
}

func TestVerifyRecoveryEvidenceReportsCandidateMissWithoutFalsifyingReconciliation(t *testing.T) {
	evidence := validRecoveryManifest()
	evidence.RecoveryPointAt = evidence.IncidentCutoffAt.Add(-16 * time.Minute)
	evidence.ServicesReadableAt = evidence.DrillStartedAt.Add(121 * time.Minute)
	evidence.ReconciliationCompletedAt = evidence.ServicesReadableAt.Add(time.Minute)
	result, err := verify(evidence)
	if err != nil {
		t.Fatalf("verify() error = %v", err)
	}
	if result.CandidateRPOMet || result.CandidateRTOMet || result.Status != "reconciled" {
		t.Fatalf("candidate misses were hidden: %#v", result)
	}
}

func validRecoveryManifest() manifest {
	incident := time.Date(2026, time.August, 27, 8, 0, 0, 0, time.UTC)
	started := incident.Add(5 * time.Minute)
	digest := strings.Repeat("a", 64)
	assets := make([]assetResult, 0, 5)
	for index, name := range []string{
		"postgres_facts", "minio_evidence", "vault_all_files", "river_jobs_attempts", "vault_manual_regions",
	} {
		assets = append(assets, assetResult{
			Name: name, ExpectedCount: int64(index + 1), ActualCount: int64(index + 1),
			ExpectedSHA256: digest, ActualSHA256: digest,
		})
	}
	return manifest{
		Version: "hotkey-recovery-manifest-v1", GitRevision: "0123456789abcdef0123456789abcdef01234567",
		Environment: "isolated-recovery-fixture", Isolated: true, ProductionEgressDisabled: true,
		IncidentCutoffAt: incident, RecoveryPointAt: incident.Add(-10 * time.Minute),
		DrillStartedAt: started, ServicesReadableAt: started.Add(90 * time.Minute),
		ReconciliationCompletedAt: started.Add(100 * time.Minute), Assets: assets, Differences: []string{},
	}
}
