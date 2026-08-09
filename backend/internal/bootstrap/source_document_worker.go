package bootstrap

import (
	"context"
	"fmt"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionfeed "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/feed"
	ingestionmarkdown "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/markdown"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type sourceEvidenceSelectionReader interface {
	Read(context.Context, sourceapplication.EvidenceSelectionQuery) (sourceapplication.EvidenceSelectionResult, error)
}

// sourceEvidenceReaderAdapter is the only Source Application to Ingestion
// Application mapper used by the document-generation worker. It never exposes
// Source Domain entities, MinIO keys, response headers, or rights records.
type sourceEvidenceReaderAdapter struct {
	selections sourceEvidenceSelectionReader
}

var _ ingestionapplication.SourceEvidenceReader = (*sourceEvidenceReaderAdapter)(nil)

func newSourceEvidenceReaderAdapter(selections sourceEvidenceSelectionReader) (*sourceEvidenceReaderAdapter, error) {
	if selections == nil {
		return nil, fmt.Errorf("Source evidence selection reader is required")
	}
	return &sourceEvidenceReaderAdapter{selections: selections}, nil
}

func provideSourceEvidenceReaderAdapter(service *sourceapplication.EvidenceSelectionService) (*sourceEvidenceReaderAdapter, error) {
	if service == nil {
		return nil, fmt.Errorf("Source evidence selection service is required")
	}
	return newSourceEvidenceReaderAdapter(service)
}

func (adapter *sourceEvidenceReaderAdapter) ReadSelectedSourceEvidence(ctx context.Context, query ingestionapplication.SourceEvidenceQuery) (ingestionapplication.SelectedSourceEvidenceDTO, error) {
	if adapter == nil || adapter.selections == nil || query.EvidenceReferenceID <= 0 {
		return ingestionapplication.SelectedSourceEvidenceDTO{}, fmt.Errorf("%w: invalid Source evidence query", sharedrepository.ErrInvalidInput)
	}
	result, err := adapter.selections.Read(ctx, sourceapplication.EvidenceSelectionQuery{EvidenceReferenceID: query.EvidenceReferenceID})
	if err != nil {
		return ingestionapplication.SelectedSourceEvidenceDTO{}, err
	}
	mapped, err := selectedSourceEvidenceDTOFromSource(result.Evidence)
	if err != nil {
		return ingestionapplication.SelectedSourceEvidenceDTO{}, err
	}
	if mapped.EvidenceReferenceID != query.EvidenceReferenceID {
		return ingestionapplication.SelectedSourceEvidenceDTO{}, fmt.Errorf("%w: selected Source evidence identity changed", sharedrepository.ErrConstraint)
	}
	return mapped, nil
}

func selectedSourceEvidenceDTOFromSource(selected sourceapplication.SelectedEvidenceDTO) (ingestionapplication.SelectedSourceEvidenceDTO, error) {
	bodyOrigin := selected.BodyOrigin
	completeness := selected.Completeness
	if err := ingestionapplication.ValidateDocumentBodyClassification(bodyOrigin, completeness); err != nil {
		return ingestionapplication.SelectedSourceEvidenceDTO{}, fmt.Errorf("%w: selected Source body semantics are invalid", sharedrepository.ErrConstraint)
	}
	var publishedAt *time.Time
	if selected.PublishedAt != nil {
		value := *selected.PublishedAt
		publishedAt = &value
	}
	return ingestionapplication.SelectedSourceEvidenceDTO{
		EvidenceReferenceID:   selected.EvidenceReferenceID,
		SourceObservationID:   selected.SourceObservationID,
		EvidenceSnapshotID:    selected.EvidenceSnapshotID,
		SourceConnectionID:    selected.SourceConnectionID,
		ExternalWorkID:        selected.ExternalID,
		UpstreamIdentity:      selected.UpstreamIdentity,
		SourceCode:            selected.SourceCode,
		ContentType:           selected.ContentType,
		Title:                 selected.Title,
		Language:              selected.Language,
		Author:                selected.Author,
		SourceRecordURL:       selected.SourceRecordURL,
		CanonicalURL:          selected.CanonicalURL,
		DiscussionURL:         selected.DiscussionURL,
		BodyOrigin:            bodyOrigin,
		Completeness:          completeness,
		PublishedAt:           publishedAt,
		DiscoveredAt:          selected.DiscoveredAt,
		CapturedAt:            selected.CapturedAt,
		SelectedPayload:       append([]byte(nil), selected.SelectedPayload...),
		SelectedPayloadSHA256: selected.SelectedPayloadSHA256,
		PayloadMIMEType:       selected.PayloadMIMEType,
		SelectorVersion:       selected.SelectorVersion,
	}, nil
}

func newDocumentObservationPersistenceService(repository *ingestionpostgres.DocumentVersionRepository) (*ingestionapplication.DocumentVersionService, error) {
	return ingestionapplication.NewDocumentObservationPersistenceService(repository)
}

func newFeedBodyExtractor(markdown *ingestionmarkdown.Converter) *ingestionfeed.BodyExtractor {
	return ingestionfeed.NewBodyExtractor(markdown)
}

func newDerivedDocumentProjectionService(
	publisher *knowledgeapplication.ProjectionService,
	repository *ingestionpostgres.DerivedArtifactRepository,
	documentVersions *ingestionapplication.DocumentVersionService,
) (*ingestionapplication.DocumentProjectionService, error) {
	return ingestionapplication.NewDocumentProjectionService(ingestionapplication.DocumentProjectionDependencies{
		Publisher: publisher, Repository: repository, DocumentVersions: documentVersions,
	})
}

func newDocumentRecallProjectionService(
	writer *ingestionpostgres.DocumentRecallProjectionWriter,
) (*ingestionapplication.DocumentRecallProjectionService, error) {
	return ingestionapplication.NewDocumentRecallProjectionService(writer)
}

func newSourceDocumentGenerationService(
	evidence *sourceEvidenceReaderAdapter,
	extractor *ingestionfeed.BodyExtractor,
	documentVersions *ingestionapplication.DocumentVersionService,
	authorizations *ingestionpostgres.DocumentProjectionAuthorizationReader,
	projections *ingestionapplication.DocumentProjectionService,
	searchProjections *ingestionapplication.DocumentRecallProjectionService,
) (*ingestionapplication.SourceDocumentGenerationService, error) {
	return ingestionapplication.NewSourceDocumentGenerationService(ingestionapplication.SourceDocumentGenerationDependencies{
		Evidence: evidence, Extractor: extractor, DocumentVersions: documentVersions,
		Authorizations: authorizations, Projections: projections, SearchProjections: searchProjections,
		Now: func() time.Time { return time.Now().UTC() },
	})
}
