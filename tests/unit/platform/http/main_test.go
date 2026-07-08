package platformhttp_test

import (
	"os"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
)

func TestMain(m *testing.M) {
	_ = logging.Init("info", "json")
	os.Exit(m.Run())
}
