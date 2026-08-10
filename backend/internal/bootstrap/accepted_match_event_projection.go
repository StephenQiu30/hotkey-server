package bootstrap

import (
	"context"
	"errors"
	"fmt"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/postgres"
	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type acceptedDocumentMatchEventProjectionAdapter struct {
	service  *eventapplication.AcceptedMatchEventProjectionService
	evidence *eventapplication.AutomaticClaimEvidenceService
}

var _ ingestionapplication.AcceptedDocumentMatchConsumer = (*acceptedDocumentMatchEventProjectionAdapter)(nil)

func exposeAcceptedMatchFamilyReader(repository *eventpostgres.MicroEventRepository) eventapplication.AcceptedMatchFamilyReader {
	return repository
}

func newMicroEventService(repository *eventpostgres.MicroEventRepository, qualityProfiles *operationspostgres.DecisionQualityRepository) (*eventapplication.MicroEventService, error) {
	return eventapplication.NewMicroEventServiceWithQualityProfiles(repository, qualityProfiles)
}

func newStorylineService(repository *eventpostgres.StorylinePostgresRepository) (*eventapplication.StorylineService, error) {
	return eventapplication.NewStorylineService(repository)
}

func newEventHeatV2Service(repository *eventpostgres.EventHeatRepository) (*eventapplication.EventHeatService, error) {
	return eventapplication.NewEventHeatService(repository)
}

func newAcceptedDocumentMatchEventProjectionAdapter(service *eventapplication.AcceptedMatchEventProjectionService,
	evidence *eventapplication.AutomaticClaimEvidenceService) (*acceptedDocumentMatchEventProjectionAdapter, error) {
	if service == nil || evidence == nil {
		return nil, fmt.Errorf("accepted document match event projection service is required")
	}
	return &acceptedDocumentMatchEventProjectionAdapter{service: service, evidence: evidence}, nil
}

func newAcceptedDocumentMatchProjectionHandler(adapter *acceptedDocumentMatchEventProjectionAdapter) (*ingestionjobs.AcceptedDocumentMatchProjectionHandler, error) {
	return ingestionjobs.NewAcceptedDocumentMatchProjectionHandler(adapter)
}

func (adapter *acceptedDocumentMatchEventProjectionAdapter) ConsumeAcceptedDocumentMatch(ctx context.Context,
	command ingestionapplication.ConsumeAcceptedDocumentMatchCommand) (ingestionapplication.ConsumeAcceptedDocumentMatchResult, error) {
	if adapter == nil || adapter.service == nil || command.DocumentMatchDecisionID <= 0 || command.DocumentVersionID <= 0 {
		return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	projected, err := adapter.service.Project(ctx, eventapplication.ProjectAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: command.DocumentMatchDecisionID, DocumentVersionID: command.DocumentVersionID})
	if err != nil {
		return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, fmt.Errorf("project accepted match into event v2: %w", err)
	}
	if projected.Membership.Action != "review" {
		extracted, extractionErr := adapter.evidence.Extract(ctx, eventapplication.AutomaticClaimEvidenceCommand{
			MicroEventID: projected.MicroEvent.ID, ExpectedEventVersion: projected.MicroEvent.Version,
			DocumentVersionID: command.DocumentVersionID,
		})
		if extractionErr != nil && !errors.Is(extractionErr, sharedrepository.ErrNotFound) {
			return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, fmt.Errorf("extract accepted match claim evidence: %w", extractionErr)
		}
		if extractionErr == nil && extracted.Status != "succeeded" && extracted.Status != "degraded" {
			return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, eventapplication.ErrInvalidAutomaticClaimEvidenceContract
		}
	}
	return ingestionapplication.ConsumeAcceptedDocumentMatchResult{DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		DocumentVersionID: command.DocumentVersionID}, nil
}
