package domain

import "testing"

func TestEvidenceLineageMaintenanceVocabularyIsFinite(t *testing.T) {
	for _, phase := range []EvidenceLineageMigrationPhase{
		MigrationPhaseSource,
		MigrationPhaseEvidenceMetadata,
		MigrationPhaseDocument,
		MigrationPhaseMatch,
		MigrationPhaseFamilyEvent,
		MigrationPhaseEvidenceState,
	} {
		if !phase.Valid() {
			t.Fatalf("phase %q should be valid", phase)
		}
	}
	if EvidenceLineageMigrationPhase("events").Valid() {
		t.Fatal("unregistered phase should be rejected")
	}

	for _, scope := range []EvidenceLineageReconciliationScope{
		ReconciliationScopePostgresMinIO,
		ReconciliationScopePostgresVault,
		ReconciliationScopeRightsRetention,
		ReconciliationScopeAll,
	} {
		if !scope.Valid() {
			t.Fatalf("scope %q should be valid", scope)
		}
	}
	if EvidenceLineageReconciliationScope("storage").Valid() {
		t.Fatal("unregistered scope should be rejected")
	}
}

func TestEvidenceLineageMaintenanceResultVocabularyDoesNotInventFacts(t *testing.T) {
	for _, disposition := range []EvidenceLineageItemDisposition{
		EvidenceLineageItemReused,
		EvidenceLineageItemCreated,
		EvidenceLineageItemSkipped,
		EvidenceLineageItemBlocked,
		EvidenceLineageItemFailed,
	} {
		if !disposition.Valid() {
			t.Fatalf("disposition %q should be valid", disposition)
		}
	}
	for _, finding := range []EvidenceLineageReconciliationFinding{
		ReconciliationFindingMissing,
		ReconciliationFindingOrphanWithinGrace,
		ReconciliationFindingOrphanExpired,
		ReconciliationFindingDigestMismatch,
		ReconciliationFindingPolicyBlocked,
		ReconciliationFindingRetentionBlocked,
		ReconciliationFindingActivePointerInvalid,
		ReconciliationFindingHealthy,
	} {
		if !finding.Valid() {
			t.Fatalf("finding %q should be valid", finding)
		}
	}
}
