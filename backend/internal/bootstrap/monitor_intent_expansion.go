package bootstrap

import (
	"context"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/jobs"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func newMonitorIntentAnalysisProcessor(intents *monitorapplication.IntentService, runs *intelligenceapplication.RunService) (*monitorjobs.IntentAnalysisCompositeProcessor, error) {
	return monitorjobs.NewIntentAnalysisCompositeProcessor(intents, runs)
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
