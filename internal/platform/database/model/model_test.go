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

func TestPersistenceMetadataMakesEveryBusinessTableVersioned(t *testing.T) {
	for _, spec := range All() {
		metadata, found := PersistenceFor(spec.Table)
		if !found {
			t.Fatalf("PersistenceFor(%q) did not return metadata", spec.Table)
		}
		if spec.Lifecycle == LifecycleBusiness && metadata.VersionColumn != "version" {
			t.Errorf("business table %s VersionColumn = %q, want version", spec.Table, metadata.VersionColumn)
		}
		if spec.Lifecycle == LifecycleOperational && metadata.VersionColumn != "" {
			t.Errorf("operational table %s VersionColumn = %q, want empty", spec.Table, metadata.VersionColumn)
		}
	}
}
