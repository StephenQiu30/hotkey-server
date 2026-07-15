package model

import "testing"

func TestSpecsHaveUniqueTablesAndColumns(t *testing.T) {
	seen := map[string]bool{}
	for _, spec := range All() {
		if spec.Table == "" || seen[spec.Table] {
			t.Fatalf("invalid or duplicate table spec %q", spec.Table)
		}
		seen[spec.Table] = true
		if len(spec.Columns) == 0 {
			t.Fatalf("%s has no mapped columns", spec.Table)
		}
	}
	if got, want := len(seen), 48; got != want {
		t.Fatalf("mapped table count = %d, want %d", got, want)
	}
}
