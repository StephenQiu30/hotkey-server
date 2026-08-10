package bootstrap

import (
	"context"
	"fmt"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/postgres"
	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
)

type automaticClaimEvidenceProjectionAdapter struct {
	projections *knowledgeapplication.ProjectionService
}

func (adapter automaticClaimEvidenceProjectionAdapter) ReadAutomaticClaimEvidenceProjection(ctx context.Context, query eventapplication.AutomaticClaimEvidenceProjectionQuery) (eventapplication.AutomaticClaimEvidenceProjectionDTO, error) {
	if adapter.projections == nil || query.Artifact.DocumentID <= 0 || query.Artifact.DocumentVersionID <= 0 || query.MaxBytes <= 0 {
		return eventapplication.AutomaticClaimEvidenceProjectionDTO{}, eventapplication.ErrInvalidAutomaticClaimEvidenceContract
	}
	result, err := adapter.projections.ReadDocumentProjection(ctx, knowledgeapplication.DocumentProjectionQueryDTO{
		DocumentID: query.Artifact.DocumentID, DocumentVersionID: query.Artifact.DocumentVersionID,
		ArtifactType: query.Artifact.ArtifactType, TransformerProfileSHA256: query.Artifact.TransformerProfileSHA256,
		SHA256: query.Artifact.PlaintextSHA256, SizeBytes: query.Artifact.SizeBytes, MaxBytes: query.MaxBytes,
	})
	if err != nil {
		return eventapplication.AutomaticClaimEvidenceProjectionDTO{}, err
	}
	return eventapplication.AutomaticClaimEvidenceProjectionDTO{Plaintext: result.Content, MIMEType: result.MIMEType,
		SHA256: result.SHA256, SizeBytes: result.SizeBytes}, nil
}

type automaticQuoteSelectorAdapter struct {
	selectors *ingestionapplication.TextQuoteSelectorService
}

func (adapter automaticQuoteSelectorAdapter) LocateAutomaticQuoteSelector(ctx context.Context, command eventapplication.LocateAutomaticQuoteSelectorCommand) (eventapplication.LocatedAutomaticQuoteSelectorDTO, error) {
	if adapter.selectors == nil {
		return eventapplication.LocatedAutomaticQuoteSelectorDTO{}, eventapplication.ErrInvalidAutomaticClaimEvidenceContract
	}
	result, err := adapter.selectors.LocateAndCreate(ctx, ingestionapplication.LocateTextQuoteSelectorCommand{
		DocumentVersionID: command.DocumentVersionID, ExactQuote: command.ExactQuote,
		PlaintextSHA256: command.PlaintextSHA256, NormalizationVersion: command.NormalizationVersion,
		DecisionAt: command.DecisionAt,
	})
	if err != nil {
		return eventapplication.LocatedAutomaticQuoteSelectorDTO{}, err
	}
	selector := result.Selector
	return eventapplication.LocatedAutomaticQuoteSelectorDTO{ID: selector.ID, Version: selector.Version,
		DocumentVersionID: selector.DocumentVersionID, ExactQuote: selector.ExactQuote,
		PlaintextSHA256: selector.PlaintextSHA256}, nil
}

func newAutomaticClaimEvidenceService(repository *eventpostgres.ClaimEvidencePostgresRepository,
	projections *knowledgeapplication.ProjectionService, models *intelligenceapplication.RunService,
	selectors *ingestionapplication.TextQuoteSelectorService, evidence *eventapplication.ClaimEvidenceService,
	summaries *eventapplication.EvidenceSummaryService, qualityProfiles *operationspostgres.DecisionQualityRepository) (*eventapplication.AutomaticClaimEvidenceService, error) {
	if repository == nil || projections == nil || models == nil || selectors == nil || evidence == nil || summaries == nil || qualityProfiles == nil {
		return nil, fmt.Errorf("automatic claim evidence composition dependencies are required")
	}
	return eventapplication.NewAutomaticClaimEvidenceService(eventapplication.AutomaticClaimEvidenceDependencies{
		Targets: repository, Projections: automaticClaimEvidenceProjectionAdapter{projections: projections},
		Models: models, Selectors: automaticQuoteSelectorAdapter{selectors: selectors}, Evidence: evidence, Summaries: summaries,
		QualityProfiles: qualityProfiles,
	})
}
