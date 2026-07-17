package application

import (
	"context"
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
)

type Store interface {
	Save(context.Context, domain.Report) error
}

func Save(ctx context.Context, store Store, report domain.Report) error {
	if store == nil {
		return fmt.Errorf("report store is required")
	}
	if err := report.Validate(); err != nil {
		return err
	}
	return store.Save(ctx, report)
}
