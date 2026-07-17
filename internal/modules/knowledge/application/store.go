package application

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
)

type Store interface {
	SaveDocument(context.Context, domain.Document) error
}
