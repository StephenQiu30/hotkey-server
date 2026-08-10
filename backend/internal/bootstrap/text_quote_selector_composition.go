package bootstrap

import (
	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
)

func newTextQuoteSelectorService(repository *ingestionpostgres.TextQuoteSelectorRepository, projections *knowledgeapplication.ProjectionService) (*ingestionapplication.TextQuoteSelectorService, error) {
	return ingestionapplication.NewTextQuoteSelectorService(ingestionapplication.TextQuoteSelectorDependencies{
		Repository: repository, Projections: projections,
	})
}
