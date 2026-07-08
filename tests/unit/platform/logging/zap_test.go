package logging_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
)

func TestInitStandardLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, lvl := range levels {
		if err := logging.Init(lvl, "json", "stdout"); err != nil {
			t.Fatalf("Init(%q, json) returned error: %v", lvl, err)
		}
		if logging.L() == nil {
			t.Fatalf("L() returned nil after Init(%q)", lvl)
		}
		if logging.S() == nil {
			t.Fatalf("S() returned nil after Init(%q)", lvl)
		}
	}
}

func TestInitInvalidLevelDefaultsToInfo(t *testing.T) {
	if err := logging.Init("invalid", "json", "stdout"); err != nil {
		t.Fatalf("Init with invalid level returned error: %v", err)
	}
	if logging.L() == nil {
		t.Fatal("L() returned nil after Init with invalid level")
	}
}

func TestInitConsoleFormat(t *testing.T) {
	if err := logging.Init("info", "console", "stdout"); err != nil {
		t.Fatalf("Init(info, console) returned error: %v", err)
	}
	if logging.L() == nil {
		t.Fatal("L() returned nil after Init with console format")
	}
}
