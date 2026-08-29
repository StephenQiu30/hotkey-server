package bootstrap

import (
	"strings"

	eventpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/postgres"
	identitypostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/postgres"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/postgres"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	reportpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/infrastructure/postgres"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
)

func newPaginationCodec(cfg config.Config) (*pagination.Codec, error) {
	secret := strings.TrimSpace(cfg.Authentication.JWTSecret)
	if len([]byte(secret)) < 32 {
		// Worker-only roles do not require JWT runtime configuration. Their
		// database URL is still shared stable secret material, and Codec derives
		// a purpose-specific HMAC key rather than using it directly.
		secret = strings.TrimSpace(cfg.DatabaseURL)
	}
	return pagination.NewCodec(secret, pagination.DefaultTTL)
}

func newSourceRepository(runtime *database.Runtime, codec *pagination.Codec) *sourcepostgres.Repository {
	return sourcepostgres.NewRepositoryWithCursorCodec(runtime, codec)
}

func newUserRepository(runtime *database.Runtime, codec *pagination.Codec) *identitypostgres.UserRepository {
	return identitypostgres.NewUserRepositoryWithCursorCodec(runtime, codec)
}

func newKnowledgeRepository(runtime *database.Runtime, codec *pagination.Codec) *knowledgepostgres.Repository {
	return knowledgepostgres.NewRepositoryWithCursorCodec(runtime, codec)
}

func newCollectionRepository(runtime *database.Runtime, codec *pagination.Codec) *sourcepostgres.CollectionRepository {
	return sourcepostgres.NewCollectionRepositoryWithCursorCodec(runtime, codec)
}

func newRightsManagementRepository(runtime *database.Runtime, codec *pagination.Codec) *sourcepostgres.RightsManagementRepository {
	return sourcepostgres.NewRightsManagementRepositoryWithCursorCodec(runtime, codec)
}

func newContentRepository(runtime *database.Runtime, codec *pagination.Codec) *ingestionpostgres.ContentRepository {
	return ingestionpostgres.NewContentRepositoryWithCursorCodec(runtime, codec)
}

func newMonitorRepository(runtime *database.Runtime, codec *pagination.Codec) *monitorpostgres.Repository {
	return monitorpostgres.NewRepositoryWithCursorCodec(runtime, codec)
}

func newReportRepository(runtime *database.Runtime, codec *pagination.Codec) *reportpostgres.Repository {
	return reportpostgres.NewRepositoryWithCursorCodec(runtime, codec)
}

func newJobRepository(runtime *database.Runtime, codec *pagination.Codec) *operationspostgres.JobRepository {
	return operationspostgres.NewJobRepositoryWithCursorCodec(runtime, codec)
}

func newGovernanceRepository(runtime *database.Runtime, codec *pagination.Codec) *operationspostgres.GovernanceRepository {
	return operationspostgres.NewGovernanceRepositoryWithCursorCodec(runtime, codec)
}

func newMicroEventQueryRepository(runtime *database.Runtime, codec *pagination.Codec) (*eventpostgres.MicroEventQueryPostgresRepository, error) {
	return eventpostgres.NewMicroEventQueryPostgresRepositoryWithCursorCodec(runtime, codec)
}
