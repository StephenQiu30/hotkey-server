//go:build !onnx || !cgo

package provider

import (
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
)

// NewONNXProvider is intentionally unavailable unless both the explicit onnx
// build tag and CGO are enabled. It must not inspect artifacts or load native
// code in this build, so normal application startup stays independent of ONNX.
func NewONNXProvider(config.AIConfig) (intelligencedomain.Provider, error) {
	return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
}
