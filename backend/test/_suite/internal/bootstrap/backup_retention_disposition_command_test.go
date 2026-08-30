package bootstrap

import "testing"

func TestParseBackupRetentionDispositionFlagsRequiresOneBoundedManifestPath(t *testing.T) {
	path, err := parseBackupRetentionDispositionFlags([]string{"--manifest", "evidence/backup-disposition.json"})
	if err != nil || path != "evidence/backup-disposition.json" {
		t.Fatalf("path=%q error=%v", path, err)
	}
	for _, args := range [][]string{
		nil,
		{"--manifest", "/"},
		{"--manifest", "evidence/run.json", "unexpected"},
	} {
		if _, err := parseBackupRetentionDispositionFlags(args); err == nil {
			t.Fatalf("invalid args were accepted: %#v", args)
		}
	}
}
