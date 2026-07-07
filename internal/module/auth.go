package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var AuthModule = fx.Module("auth",
	fx.Provide(gormimpl.NewUserRepo),
	fx.Provide(auth.NewService),
)
