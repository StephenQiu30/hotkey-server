package database

import "testing"

func TestCanonicalCatalogIncludesSetNullForeignKeyFromAlterTable(t *testing.T) {
	contract, err := canonicalCatalogContract()
	if err != nil {
		t.Fatalf("canonicalCatalogContract() error = %v", err)
	}
	checkpoint, found := contract.Tables["source_checkpoints"]
	if !found {
		t.Fatal("canonical catalog is missing source_checkpoints")
	}
	if got, want := checkpoint.Constraints.Foreign, 2; got != want {
		t.Fatalf("source_checkpoints foreign key count = %d, want %d", got, want)
	}
}
