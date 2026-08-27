package postgres

import (
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
)

func ingestionTestCursorCodec(runtime *database.Runtime, scope string) *pagination.Codec {
	seed := "unavailable"
	if runtime != nil && runtime.Pool != nil {
		seed = runtime.Pool.Config().ConnString()
	}
	return pagination.NewTestCodec(fmt.Sprintf("ingestion:%s:%s", scope, seed))
}
