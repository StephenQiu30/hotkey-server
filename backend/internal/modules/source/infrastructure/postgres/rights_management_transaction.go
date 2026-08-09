package postgres

import (
	"context"
	"fmt"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// RightsManagementTransactionAdapter lets the Source repository and a future
// Operations audit adapter share the Runtime transaction carried by context.
type RightsManagementTransactionAdapter struct {
	runtime *database.Runtime
}

var _ sourceapplication.RightsManagementTransactionRunner = (*RightsManagementTransactionAdapter)(nil)

func NewRightsManagementTransactionAdapter(runtime *database.Runtime) *RightsManagementTransactionAdapter {
	return &RightsManagementTransactionAdapter{runtime: runtime}
}

func (adapter *RightsManagementTransactionAdapter) WithinRightsManagementTransaction(ctx context.Context, operation func(context.Context) error) error {
	if adapter == nil || adapter.runtime == nil || adapter.runtime.SQL == nil || adapter.runtime.GORM == nil {
		return sharedrepository.ErrUnavailable
	}
	if operation == nil {
		return fmt.Errorf("%w: rights management transaction operation is required", sharedrepository.ErrInvalidInput)
	}
	if _, found := database.TransactionFromContext(ctx); found {
		return operation(ctx)
	}
	err := adapter.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		return operation(transactionCtx)
	})
	return databaserepository.MapError(err)
}
