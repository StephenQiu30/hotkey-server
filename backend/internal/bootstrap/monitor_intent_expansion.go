package bootstrap

import (
	"context"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/jobs"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
)

func newMonitorIntentCompiler(repository *monitorpostgres.IntentRepository) (*monitorapplication.IntentCompiler, error) {
	return monitorapplication.NewIntentCompiler(repository, sharedclock.System{})
}

func newIntentPublicationService(repository *monitorpostgres.IntentRepository) (*monitorapplication.IntentPublicationService, error) {
	return monitorapplication.NewIntentPublicationService(repository)
}

func newHybridRecallService(profiles *monitorpostgres.CompiledRecallProfileReader, documents *ingestionpostgres.HybridDocumentRecallReader) (*ingestionapplication.HybridRecallService, error) {
	return ingestionapplication.NewHybridRecallService(profiles, documents, documents, documents)
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
