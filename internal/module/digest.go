package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var DigestModule = fx.Module("digest",
	fx.Provide(gormimpl.NewDigestRepo),
)
