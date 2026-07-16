package provider

import (
	"testing"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
)

// This name is intentionally stable: ordinary CI invokes it with -tags=onnx
// and CGO_ENABLED=0 to prove no native headers or artifacts are required.
func TestONNXProviderUnavailableWithoutCGO(t *testing.T) {
	provider, err := NewONNXProvider(config.AIConfig{})
	if provider != nil {
		t.Fatalf("NewONNXProvider() provider = %#v, want no native provider in default build", provider)
	}
	assertCode(t, err, intelligencedomain.CodeAIModelUnavailable)
}
