package bootstrap

import (
	"context"
	"fmt"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/jobs"
	eventpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/postgres"
	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
)

type acceptedDocumentMatchEventProjector interface {
	Project(context.Context, eventapplication.ProjectAcceptedDocumentMatchCommand) (eventapplication.ProjectAcceptedDocumentMatchResult, error)
}

type acceptedDocumentMatchEventProjectionAdapter struct {
	service  acceptedDocumentMatchEventProjector
	evidence eventapplication.AutomaticClaimEvidenceScheduler
	refresh  eventapplication.ProductEventRefreshScheduler
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

func exposeAutomaticClaimEvidenceScheduler(scheduler *eventjobs.AutomaticClaimEvidenceScheduler) eventapplication.AutomaticClaimEvidenceScheduler {
	return scheduler
}

func exposeProductEventRefreshScheduleTargetReader(repository *eventpostgres.ProductEventRefreshPostgresRepository) eventapplication.ProductEventRefreshScheduleTargetReader {
	return repository
}

func exposeProductEventRefreshRepository(repository *eventpostgres.ProductEventRefreshPostgresRepository) eventapplication.ProductEventRefreshRepository {
	return repository
}

func exposeProductEventAlertEvaluator(repository *eventpostgres.ProductEventRefreshPostgresRepository) eventapplication.ProductEventAlertEvaluator {
	return repository
}

func newProductEventRefreshService(repository eventapplication.ProductEventRefreshRepository,
	heat *eventapplication.EventHeatService, evidence *eventapplication.ClaimEvidenceService,
	alerts eventapplication.ProductEventAlertEvaluator) (*eventapplication.ProductEventRefreshService, error) {
	return eventapplication.NewProductEventRefreshService(repository, heat, evidence, alerts)
}

func exposeProductEventRefreshScheduler(scheduler *eventjobs.ProductEventRefreshScheduler) eventapplication.ProductEventRefreshScheduler {
	return scheduler
}

func exposeAcceptedDocumentMatchEventProjector(service *eventapplication.AcceptedMatchEventProjectionService) acceptedDocumentMatchEventProjector {
	return service
}

func newAcceptedDocumentMatchEventProjectionAdapter(service acceptedDocumentMatchEventProjector,
	evidence eventapplication.AutomaticClaimEvidenceScheduler,
	refresh eventapplication.ProductEventRefreshScheduler) (*acceptedDocumentMatchEventProjectionAdapter, error) {
	if service == nil || evidence == nil || refresh == nil {
		return nil, fmt.Errorf("accepted document match event projection dependencies are required")
	}
	return &acceptedDocumentMatchEventProjectionAdapter{service: service, evidence: evidence, refresh: refresh}, nil
}

func newAcceptedDocumentMatchProjectionHandler(adapter *acceptedDocumentMatchEventProjectionAdapter) (*ingestionjobs.AcceptedDocumentMatchProjectionHandler, error) {
	return ingestionjobs.NewAcceptedDocumentMatchProjectionHandler(adapter)
}

func (adapter *acceptedDocumentMatchEventProjectionAdapter) ConsumeAcceptedDocumentMatch(ctx context.Context,
	command ingestionapplication.ConsumeAcceptedDocumentMatchCommand) (ingestionapplication.ConsumeAcceptedDocumentMatchResult, error) {
	if adapter == nil || adapter.service == nil || adapter.evidence == nil || adapter.refresh == nil || command.DocumentMatchDecisionID <= 0 || command.DocumentVersionID <= 0 {
		return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	projected, err := adapter.service.Project(ctx, eventapplication.ProjectAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: command.DocumentMatchDecisionID, DocumentVersionID: command.DocumentVersionID})
	if err != nil {
		return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, fmt.Errorf("project accepted match into event v2: %w", err)
	}
	if projected.Membership.Action != "review" {
		scheduled, scheduleErr := adapter.evidence.ScheduleAutomaticClaimEvidence(ctx, eventapplication.ScheduleAutomaticClaimEvidenceCommand{
			MicroEventID: projected.MicroEvent.ID, DocumentVersionID: command.DocumentVersionID,
		})
		if scheduleErr != nil {
			return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, fmt.Errorf("schedule automatic claim evidence: %w", scheduleErr)
		}
		if scheduled.MicroEventID != projected.MicroEvent.ID || scheduled.DocumentVersionID != command.DocumentVersionID || scheduled.JobID <= 0 {
			return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
		}
		refresh, refreshErr := adapter.refresh.ScheduleProductEventRefresh(ctx, eventapplication.ScheduleProductEventRefreshCommand{
			MicroEventID: projected.MicroEvent.ID, ExpectedEventVersion: projected.MicroEvent.Version,
		})
		if refreshErr != nil {
			return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, fmt.Errorf("schedule product event refresh: %w", refreshErr)
		}
		if refresh.Available && (refresh.MicroEventID != projected.MicroEvent.ID ||
			refresh.MicroEventVersion != projected.MicroEvent.Version || refresh.JobID <= 0) {
			return ingestionapplication.ConsumeAcceptedDocumentMatchResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
		}
	}
	return ingestionapplication.ConsumeAcceptedDocumentMatchResult{DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		DocumentVersionID: command.DocumentVersionID}, nil
}
