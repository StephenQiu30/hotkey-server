package bootstrap

import (
	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
)

func newContentLineageFeedbackService(repository *ingestionpostgres.ContentFamilyRepository) (*ingestionapplication.ContentLineageFeedbackService, error) {
	return ingestionapplication.NewContentLineageFeedbackService(repository)
}
