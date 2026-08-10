package bootstrap

import (
	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/postgres"
)

// Fx intentionally does not infer concrete-to-interface bindings. These
// composition-root factories keep PostgreSQL types out of Application ports.
func newMicroEventGovernanceService(repository *eventpostgres.MicroEventGovernancePostgresRepository) (*eventapplication.MicroEventGovernanceService, error) {
	return eventapplication.NewMicroEventGovernanceService(repository)
}

func newClaimEvidenceService(repository *eventpostgres.ClaimEvidencePostgresRepository) (*eventapplication.ClaimEvidenceService, error) {
	return eventapplication.NewClaimEvidenceService(repository)
}

func newEvidenceSummaryService(repository *eventpostgres.EvidenceSummaryPostgresRepository) (*eventapplication.EvidenceSummaryService, error) {
	return eventapplication.NewEvidenceSummaryService(repository)
}

func newMicroEventQueryService(repository *eventpostgres.MicroEventQueryPostgresRepository) (*eventapplication.MicroEventQueryService, error) {
	return eventapplication.NewMicroEventQueryService(repository)
}
