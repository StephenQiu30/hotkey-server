package domain

// EvidenceLineageMigrationPhase is the ordered, conservative backfill
// vocabulary. Phases are intentionally semantic and never infer a later fact
// from a legacy truth/credibility projection.
type EvidenceLineageMigrationPhase string

const (
	MigrationPhaseSource           EvidenceLineageMigrationPhase = "source"
	MigrationPhaseEvidenceMetadata EvidenceLineageMigrationPhase = "evidence_metadata"
	MigrationPhaseDocument         EvidenceLineageMigrationPhase = "document"
	MigrationPhaseMatch            EvidenceLineageMigrationPhase = "match"
	MigrationPhaseFamilyEvent      EvidenceLineageMigrationPhase = "family_event"
	MigrationPhaseEvidenceState    EvidenceLineageMigrationPhase = "evidence_state"
)

func (phase EvidenceLineageMigrationPhase) Valid() bool {
	switch phase {
	case MigrationPhaseSource, MigrationPhaseEvidenceMetadata, MigrationPhaseDocument,
		MigrationPhaseMatch, MigrationPhaseFamilyEvent, MigrationPhaseEvidenceState:
		return true
	default:
		return false
	}
}

type EvidenceLineageReconciliationScope string

const (
	ReconciliationScopePostgresMinIO   EvidenceLineageReconciliationScope = "pg-minio"
	ReconciliationScopePostgresVault   EvidenceLineageReconciliationScope = "pg-vault"
	ReconciliationScopeRightsRetention EvidenceLineageReconciliationScope = "rights-retention"
	ReconciliationScopeAll             EvidenceLineageReconciliationScope = "all"
)

func (scope EvidenceLineageReconciliationScope) Valid() bool {
	return scope == ReconciliationScopePostgresMinIO || scope == ReconciliationScopePostgresVault ||
		scope == ReconciliationScopeRightsRetention || scope == ReconciliationScopeAll
}

type EvidenceLineageItemDisposition string

const (
	EvidenceLineageItemReused  EvidenceLineageItemDisposition = "reused"
	EvidenceLineageItemCreated EvidenceLineageItemDisposition = "created"
	EvidenceLineageItemSkipped EvidenceLineageItemDisposition = "skipped"
	EvidenceLineageItemBlocked EvidenceLineageItemDisposition = "blocked"
	EvidenceLineageItemFailed  EvidenceLineageItemDisposition = "failed"
)

func (disposition EvidenceLineageItemDisposition) Valid() bool {
	switch disposition {
	case EvidenceLineageItemReused, EvidenceLineageItemCreated, EvidenceLineageItemSkipped,
		EvidenceLineageItemBlocked, EvidenceLineageItemFailed:
		return true
	default:
		return false
	}
}

type EvidenceLineageReconciliationFinding string

const (
	ReconciliationFindingMissing              EvidenceLineageReconciliationFinding = "missing"
	ReconciliationFindingOrphanWithinGrace    EvidenceLineageReconciliationFinding = "orphan_within_grace"
	ReconciliationFindingOrphanExpired        EvidenceLineageReconciliationFinding = "orphan_expired"
	ReconciliationFindingDigestMismatch       EvidenceLineageReconciliationFinding = "digest_mismatch"
	ReconciliationFindingPolicyBlocked        EvidenceLineageReconciliationFinding = "policy_blocked"
	ReconciliationFindingRetentionBlocked     EvidenceLineageReconciliationFinding = "retention_blocked"
	ReconciliationFindingActivePointerInvalid EvidenceLineageReconciliationFinding = "active_pointer_invalid"
	ReconciliationFindingHealthy              EvidenceLineageReconciliationFinding = "healthy"
)

func (finding EvidenceLineageReconciliationFinding) Valid() bool {
	switch finding {
	case ReconciliationFindingMissing, ReconciliationFindingOrphanWithinGrace,
		ReconciliationFindingOrphanExpired, ReconciliationFindingDigestMismatch,
		ReconciliationFindingPolicyBlocked, ReconciliationFindingRetentionBlocked,
		ReconciliationFindingActivePointerInvalid, ReconciliationFindingHealthy:
		return true
	default:
		return false
	}
}
