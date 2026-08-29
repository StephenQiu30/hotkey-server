package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceManagementAuditGateKeepsFiveCategoriesAndProductionWiring(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	files := map[string][]string{
		"backend/internal/modules/operations/domain/audit.go": {
			"source_credential.changed",
			"source_budget.updated",
			"collection.manual_requested",
			"collection.retry_requested",
			"source.health_checked",
			"retention.executed",
		},
		"backend/internal/modules/source/application/service.go": {
			"ActionSourceCredentialChanged",
			"ActionSourceBudgetUpdated",
		},
		"backend/internal/modules/source/application/collection_control.go": {
			"Audit      operationsapplication.AuditWriter",
			"dependencies.Audit == nil",
			"ActionCollectionManualRequested",
			"ActionCollectionRetryRequested",
			"ActionSourceHealthChecked",
		},
		"backend/internal/bootstrap/app.go": {
			"Audit: audit",
		},
		"backend/test/_suite/internal/modules/operations/infrastructure/postgres/governance_repository_test.go": {
			"TestGovernanceAuditQueryCoversFiveSourceManagementCategoriesWithoutSyntheticSecrets",
			"synthetic-source-token-never-persist",
		},
		"backend/Makefile": {
			"source-management-audit-acceptance:",
			"TestGovernanceAuditQueryCoversFiveSourceManagementCategoriesWithoutSyntheticSecrets",
		},
	}
	for path, fragments := range files {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("%s is missing source management audit contract %q", path, fragment)
			}
		}
	}
}
