package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) ListAfter(ctx context.Context, query domain.NotificationQuery) (domain.NotificationPage, error) {
	if repository == nil || repository.runtime == nil {
		return domain.NotificationPage{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.NotificationPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT id, event_type, resource_type, resource_id, audience_role, occurred_at, payload
FROM notification_events
WHERE id > $1
  AND (
    $2 = 'admin'
    OR ($2 = 'editor' AND audience_role IN ('viewer', 'editor'))
    OR ($2 = 'viewer' AND audience_role = 'viewer')
  )
ORDER BY id ASC
LIMIT $3`, query.AfterID, query.Role, query.Limit)
	if err != nil {
		return domain.NotificationPage{}, databaserepository.MapError(err)
	}
	defer rows.Close()

	page := domain.NotificationPage{Items: make([]domain.NotificationEvent, 0, query.Limit), NextAfterID: query.AfterID}
	for rows.Next() {
		var event domain.NotificationEvent
		var payload []byte
		if err := rows.Scan(&event.ID, &event.EventType, &event.ResourceType, &event.ResourceID, &event.Audience, &event.OccurredAt, &payload); err != nil {
			return domain.NotificationPage{}, databaserepository.MapError(err)
		}
		if err := json.Unmarshal(payload, &event.Payload); err != nil {
			return domain.NotificationPage{}, fmt.Errorf("decode notification payload: %w", err)
		}
		if err := event.Validate(); err != nil {
			return domain.NotificationPage{}, fmt.Errorf("validate persisted notification: %w", err)
		}
		page.Items = append(page.Items, event)
		page.NextAfterID = event.ID
	}
	if err := rows.Err(); err != nil {
		return domain.NotificationPage{}, databaserepository.MapError(err)
	}
	return page, nil
}
