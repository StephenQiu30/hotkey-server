// Package application exposes the narrow audit port used by monitor and source
// application services. It avoids coupling either module to an adapter.
package application

import (
	"context"

	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
)

type AuditWriter interface {
	Write(context.Context, operationsdomain.AuditEntry) error
}
