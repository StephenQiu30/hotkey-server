package bootstrap

import (
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourceevidence "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/evidence"
	sourcejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/jobs"
	sourceminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/minio"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func newSourceEvidenceSelectorVerifier() sourceevidence.Selector {
	return sourceevidence.NewSelector()
}

func newSourceRawEvidenceStore(cfg config.Config) (*sourceminio.RawEvidenceStore, error) {
	return sourceminio.NewRawEvidenceStore(cfg.MinIO)
}

func newSourceRawEvidenceObjectReader(cfg config.Config) (*sourceminio.RawEvidenceObjectReader, error) {
	return sourceminio.NewRawEvidenceObjectReader(cfg.MinIO)
}

func newEvidenceSnapshotRepository(runtime *database.Runtime, scheduler *sourcejobs.SourceDocumentGenerationScheduler) (*sourcepostgres.EvidenceSnapshotRepository, error) {
	return sourcepostgres.NewEvidenceSnapshotRepository(runtime, scheduler)
}

func newRawEvidenceArchiveService(
	store *sourceminio.RawEvidenceStore,
	repository *sourcepostgres.EvidenceSnapshotRepository,
	selector sourceevidence.Selector,
) (*sourceapplication.RawEvidenceArchiveService, error) {
	return sourceapplication.NewRawEvidenceArchiveService(sourceapplication.RawEvidenceArchiveServiceDependencies{
		Store: store, Repository: repository, SelectorVerifier: selector,
	})
}

func newRawEvidenceCollectionService(
	rights *sourcepostgres.RightsDecisionReader,
	archive *sourceapplication.RawEvidenceArchiveService,
) (*sourceapplication.RawEvidenceCollectionService, error) {
	return sourceapplication.NewRawEvidenceCollectionService(sourceapplication.RawEvidenceCollectionServiceDependencies{
		Rights: rights, Archive: archive,
	})
}

func newEvidenceSelectionService(
	manifests *sourcepostgres.EvidenceSelectionManifestReader,
	objects *sourceminio.RawEvidenceObjectReader,
	selector sourceevidence.Selector,
) (*sourceapplication.EvidenceSelectionService, error) {
	return sourceapplication.NewEvidenceSelectionService(sourceapplication.EvidenceSelectionDependencies{
		Manifests: manifests, Objects: objects, Selector: selector,
	})
}
