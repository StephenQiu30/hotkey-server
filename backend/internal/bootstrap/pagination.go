package bootstrap

import (
	"strings"

	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
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
