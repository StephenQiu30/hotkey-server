package postgres

import (
	"context"
	"fmt"

	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// PublishedReferenceReader is a Monitor-owned, read-only adapter supplied to
// Source semantic-update commands. The caller must already hold the shared
// configuration advisory lock inside a transaction; this keeps the query in
// the same atomic boundary as the locked SourceConnection update.
type PublishedReferenceReader struct{ runtime *database.Runtime }

var _ sourcedomain.MonitorPublishedReferenceReader = (*PublishedReferenceReader)(nil)

func NewPublishedReferenceReader(runtime *database.Runtime) *PublishedReferenceReader {
	return &PublishedReferenceReader{runtime: runtime}
}

func (reader *PublishedReferenceReader) HasPublishedReference(ctx context.Context, sourceID int64) (bool, error) {
	if sourceID <= 0 {
		return false, fmt.Errorf("%w: source id is required", sharedrepository.ErrInvalidInput)
	}
	if reader == nil || reader.runtime == nil {
		return false, sharedrepository.ErrUnavailable
	}
	transaction, inTransaction := database.TransactionFromContext(ctx)
	if !inTransaction {
		return false, fmt.Errorf("%w: published reference read requires caller transaction", sharedrepository.ErrUnavailable)
	}

	var referenced bool
	if err := transaction.SQL.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM monitor_sources AS monitor_source
    JOIN monitor_config_versions AS config_version ON config_version.id = monitor_source.config_version_id
    WHERE monitor_source.source_connection_id = $1
      AND config_version.state IN ('published', 'superseded')
)`, sourceID).Scan(&referenced); err != nil {
		return false, sharedrepository.MapError(err)
	}
	return referenced, nil
}
