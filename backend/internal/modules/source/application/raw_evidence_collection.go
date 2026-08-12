package application

import (
	"context"
	"errors"
	"fmt"
)

// RawEvidenceArchiveUseCase is the narrow Application boundary used by the
// collection coordinator after it resolves current rights decisions.
type RawEvidenceArchiveUseCase interface {
	Archive(context.Context, ArchiveRawEvidenceCommand) (ArchiveRawEvidenceResult, error)
}

var _ RawEvidenceArchiveUseCase = (*RawEvidenceArchiveService)(nil)

type ArchiveCollectionEvidenceCommand struct {
	SourceConnectionID int64
	CollectionRunID    int64
	Fetch              RawEvidenceFetchDTO
}

type ArchiveCollectionEvidenceResult struct {
	Snapshots []PersistedEvidenceSnapshotDTO
}

// ArchiveContextEvidenceCommand archives a provider response that updates
// existing facts but does not discover a new SourceObservation. It therefore
// has no collection run identity; evidence_snapshots.collection_run_id is
// deliberately nullable for this case.
type ArchiveContextEvidenceCommand struct {
	SourceConnectionID int64
	Snapshots          []RawEvidenceSnapshotDTO
}

type ContextEvidenceArchiver interface {
	ArchiveContext(context.Context, ArchiveContextEvidenceCommand) error
}

type CollectionEvidenceArchiver interface {
	ArchiveFetch(context.Context, ArchiveCollectionEvidenceCommand) (ArchiveCollectionEvidenceResult, error)
}

var _ CollectionEvidenceArchiver = (*RawEvidenceCollectionService)(nil)

type RawEvidenceCollectionServiceDependencies struct {
	Rights  CurrentRawEvidenceRightsReader
	Archive RawEvidenceArchiveUseCase
	Clock   Clock
}

// RawEvidenceCollectionService resolves current single-action permissions and
// then delegates the raw-byte lifecycle to RawEvidenceArchiveService. It does
// not manufacture policies from legacy source configuration.
type RawEvidenceCollectionService struct {
	rights  CurrentRawEvidenceRightsReader
	archive RawEvidenceArchiveUseCase
	clock   Clock
}

func NewRawEvidenceCollectionService(dependencies RawEvidenceCollectionServiceDependencies) (*RawEvidenceCollectionService, error) {
	if dependencies.Rights == nil || dependencies.Archive == nil {
		return nil, errors.New("raw evidence rights reader and archive use case are required")
	}
	if dependencies.Clock == nil {
		dependencies.Clock = wallClock{}
	}
	return &RawEvidenceCollectionService{rights: dependencies.Rights, archive: dependencies.Archive, clock: dependencies.Clock}, nil
}

func (service *RawEvidenceCollectionService) ArchiveFetch(ctx context.Context, command ArchiveCollectionEvidenceCommand) (ArchiveCollectionEvidenceResult, error) {
	if service == nil || service.rights == nil || service.archive == nil || service.clock == nil {
		return ArchiveCollectionEvidenceResult{}, errors.New("raw evidence collection service is not initialized")
	}
	if command.SourceConnectionID <= 0 || command.CollectionRunID <= 0 {
		return ArchiveCollectionEvidenceResult{}, errors.New("raw evidence collection source and run are required")
	}
	return service.archiveFetch(ctx, command.SourceConnectionID, command.CollectionRunID, command.Fetch)
}

func (service *RawEvidenceCollectionService) archiveFetch(ctx context.Context, sourceConnectionID, collectionRunID int64, fetch RawEvidenceFetchDTO) (ArchiveCollectionEvidenceResult, error) {
	if len(fetch.Snapshots) == 0 {
		return ArchiveCollectionEvidenceResult{Snapshots: []PersistedEvidenceSnapshotDTO{}}, nil
	}
	fetchResult, err := rawEvidenceFetchEntityFromDTO(fetch)
	if err != nil {
		return ArchiveCollectionEvidenceResult{}, fmt.Errorf("validate collection raw evidence DTO: %w", err)
	}

	subjects := make([]RawEvidenceRightsSubjectDTO, 0, len(fetchResult.Snapshots))
	seen := make(map[string]struct{}, len(fetchResult.Snapshots))
	for _, snapshot := range fetchResult.Snapshots {
		identity := snapshot.Key + ":" + snapshot.PayloadSHA256
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		subjects = append(subjects, RawEvidenceRightsSubjectDTO{EvidenceKey: snapshot.Key, PayloadSHA256: snapshot.PayloadSHA256})
	}

	decisionAt := service.clock.Now().UTC()
	if decisionAt.IsZero() {
		return ArchiveCollectionEvidenceResult{}, errors.New("raw evidence collection clock returned zero time")
	}
	resolved, err := service.rights.ResolveCurrent(ctx, CurrentRawEvidenceRightsQuery{
		SourceConnectionID: sourceConnectionID,
		DecisionAt:         decisionAt,
		Subjects:           subjects,
	})
	if err != nil {
		return ArchiveCollectionEvidenceResult{}, fmt.Errorf("resolve raw evidence rights: %w", err)
	}
	archived, err := service.archive.Archive(ctx, ArchiveRawEvidenceCommand{
		SourceConnectionID: sourceConnectionID,
		CollectionRunID:    collectionRunID,
		Fetch:              fetch,
		StoreRawDecisions:  resolved.StoreRawDecisions,
		RetainDecisions:    resolved.RetainDecisions,
	})
	if err != nil {
		return ArchiveCollectionEvidenceResult{}, err
	}
	return ArchiveCollectionEvidenceResult{Snapshots: archived.Snapshots}, nil
}

func (service *RawEvidenceCollectionService) ArchiveContext(ctx context.Context, command ArchiveContextEvidenceCommand) error {
	if service == nil || service.rights == nil || service.archive == nil || service.clock == nil {
		return errors.New("raw evidence collection service is not initialized")
	}
	if command.SourceConnectionID <= 0 {
		return errors.New("raw context evidence source is required")
	}
	if len(command.Snapshots) == 0 {
		return nil
	}
	_, err := service.archiveFetch(ctx, command.SourceConnectionID, 0, RawEvidenceFetchDTO{Items: []RawEvidenceItemDTO{}, Snapshots: command.Snapshots})
	return err
}
