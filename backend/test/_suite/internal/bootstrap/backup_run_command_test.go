package bootstrap

import "testing"

func TestParseBackupRunFlagsRequiresExactlyOneManifest(t *testing.T) {
	path, err := parseBackupRunFlags([]string{"--manifest", "/isolated/backup/run.json"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/isolated/backup/run.json" {
		t.Fatalf("manifest path=%q", path)
	}
	for _, arguments := range [][]string{{}, {"--manifest", ""}, {"--manifest", "a", "extra"}} {
		if _, err := parseBackupRunFlags(arguments); err == nil {
			t.Fatalf("accepted invalid arguments=%v", arguments)
		}
	}
}
