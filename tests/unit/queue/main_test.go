package queue_test

import (
	"os"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
)

func TestMain(m *testing.M) {
	_ = logging.Init("info", "json", "stdout")
	os.Exit(m.Run())
}
