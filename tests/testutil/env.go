package testutil

import (
	"os"
	"testing"
)

// SkipIfNoDB skips the current test when no database URL is available.
// It checks TEST_DATABASE_URL first, then falls back to DATABASE_URL.
func SkipIfNoDB(t *testing.T) {
	t.Helper()

	if os.Getenv("TEST_DATABASE_URL") != "" || os.Getenv("DATABASE_URL") != "" {
		return
	}

	t.Skip("skipping: neither TEST_DATABASE_URL nor DATABASE_URL is set")
}
