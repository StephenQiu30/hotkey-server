package bootstrap

import (
	"context"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/jobs"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
)

func newMonitorIntentCompiler(repository *monitorpostgres.IntentRepository, embeddings *compiledIntentEmbeddingProducerAdapter) (*monitorapplication.IntentCompiler, error) {
	return monitorapplication.NewIntentCompiler(repository, embeddings, sharedclock.System{})
}

type intentPublicationBackfillAdapter struct {
	scheduler *ingestionjobs.PublishedMonitorMatchBackfillScheduler
}

var _ monitorapplication.PublishedIntentBackfillScheduler = (*intentPublicationBackfillAdapter)(nil)

func newIntentPublicationBackfillAdapter(scheduler *ingestionjobs.PublishedMonitorMatchBackfillScheduler) (*intentPublicationBackfillAdapter, error) {
	if scheduler == nil {
		return nil, monitorapplication.ErrInvalidIntentContract
	}
	return &intentPublicationBackfillAdapter{scheduler: scheduler}, nil
}

func (adapter *intentPublicationBackfillAdapter) SchedulePublishedIntentBackfill(ctx context.Context, command monitorapplication.SchedulePublishedIntentBackfillCommand) (monitorapplication.SchedulePublishedIntentBackfillResult, error) {
	result, err := adapter.scheduler.SchedulePublishedMonitorMatchBackfill(ctx, ingestionapplication.SchedulePublishedMonitorMatchBackfillCommand{
		MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID, CompiledProfileID: command.CompiledProfileID,
	})
	return monitorapplication.SchedulePublishedIntentBackfillResult{
		MonitorID: result.MonitorID, MonitorVersionID: result.MonitorVersionID, CompiledProfileID: result.CompiledProfileID,
		JobID: result.JobID, Created: result.Created,
	}, err
}

func newIntentPublicationService(repository *monitorpostgres.IntentRepository, backfills *intentPublicationBackfillAdapter) (*monitorapplication.IntentPublicationService, error) {
	return monitorapplication.NewIntentPublicationService(repository, backfills)
}

func newHybridRecallService(profiles *monitorpostgres.CompiledRecallProfileReader, documents *ingestionpostgres.HybridDocumentRecallReader) (*ingestionapplication.HybridRecallService, error) {
	return ingestionapplication.NewHybridRecallService(profiles, documents, documents, documents)
}

func newPublishedDocumentMatchService(recall *ingestionapplication.HybridRecallService, repository *ingestionpostgres.DocumentMatchRepository, reranker *ingestionapplication.RankSignalDocumentMatchReranker) (*ingestionapplication.PublishedDocumentMatchService, error) {
	return ingestionapplication.NewPublishedDocumentMatchService(recall, repository, repository, reranker, sharedclock.System{})
}

func newDocumentMatchRepository(runtime *database.Runtime, scheduler *ingestionjobs.AcceptedDocumentMatchProjectionScheduler) (*ingestionpostgres.DocumentMatchRepository, error) {
	return ingestionpostgres.NewDocumentMatchRepository(runtime, scheduler)
}

func newRelevanceCalibrationService(repository *ingestionpostgres.DocumentMatchRepository, reranker *ingestionapplication.RankSignalDocumentMatchReranker) (*ingestionapplication.RelevanceCalibrationService, error) {
	return ingestionapplication.NewRelevanceCalibrationService(reranker, repository, sharedclock.System{})
}

func newPublishedMatchEvaluationService(targets *ingestionpostgres.PublishedMatchTargetReader, evaluator *ingestionapplication.PublishedDocumentMatchService,
	accepted *acceptedDocumentMatchEventProjectionAdapter) (*ingestionapplication.PublishedMatchEvaluationService, error) {
	return ingestionapplication.NewPublishedMatchEvaluationService(targets, evaluator, accepted)
}

func newPublishedMonitorMatchBackfillService(targets *ingestionpostgres.PublishedMatchTargetReader, evaluator *ingestionapplication.PublishedMatchEvaluationService) (*ingestionapplication.PublishedMonitorMatchBackfillService, error) {
	return ingestionapplication.NewPublishedMonitorMatchBackfillService(targets, evaluator)
}

func newDocumentMatchReviewService(repository *ingestionpostgres.DocumentMatchRepository) (*ingestionapplication.DocumentMatchReviewService, error) {
	return ingestionapplication.NewDocumentMatchReviewService(repository, repository, sharedclock.System{})
}

func newDocumentMatchQueryService(repository *ingestionpostgres.DocumentMatchRepository) (*ingestionapplication.DocumentMatchQueryService, error) {
	return ingestionapplication.NewDocumentMatchQueryService(repository)
}

func newMonitorIntentPreviewEvaluator(intents *monitorapplication.IntentService, compiler *monitorapplication.IntentCompiler, recall *ingestionapplication.HybridRecallService, documents *ingestionpostgres.HybridDocumentRecallReader) (*monitorjobs.IntentPreviewEvaluator, error) {
	return monitorjobs.NewIntentPreviewEvaluator(intents, compiler, recall, documents)
}

func newMonitorIntentAnalysisProcessor(intents *monitorapplication.IntentService, runs *intelligenceapplication.RunService, preview *monitorjobs.IntentPreviewEvaluator) (*monitorjobs.IntentAnalysisCompositeProcessor, error) {
	return monitorjobs.NewIntentAnalysisCompositeProcessor(intents, runs, preview)
}

func exposeMonitorIntentAnalysisAvailability(processor *monitorjobs.IntentAnalysisCompositeProcessor) monitorapplication.IntentAnalysisAvailability {
	return processor
}

type monitorIntentAnalysisHandler struct {
	handler *monitorjobs.IntentAnalysisHandler
}

func newMonitorIntentAnalysisHandler(intents *monitorapplication.IntentService, processor *monitorjobs.IntentAnalysisCompositeProcessor) (*monitorIntentAnalysisHandler, error) {
	handler, err := monitorjobs.NewIntentAnalysisHandler(intents, processor)
	if err != nil {
		return nil, err
	}
	return &monitorIntentAnalysisHandler{handler: handler}, nil
}

func (handler *monitorIntentAnalysisHandler) Handle(ctx context.Context, job queue.Job) error {
	return handler.handler.Handle(ctx, job)
}
