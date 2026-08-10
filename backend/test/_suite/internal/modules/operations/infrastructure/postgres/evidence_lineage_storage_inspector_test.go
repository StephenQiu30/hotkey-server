package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVaultProjectionInspectorListsAndVerifiesOnlyRootedRegularFiles(t *testing.T) {
	root := t.TempDir()
	relativePath := "documents/17/29/markdown/" + fmt.Sprintf("%x", sha256.Sum256([]byte("profile"))) + ".md"
	absolutePath := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("# archived body\n")
	if err := os.WriteFile(absolutePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(filepath.Dir(absolutePath), "escaped.md")); err != nil {
		t.Fatal(err)
	}
	inspector := &vaultProjectionFileInspector{root: root}

	listed, err := inspector.ListVaultProjections(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Locator != relativePath || listed[0].ModifiedAt.IsZero() {
		t.Fatalf("listed = %+v", listed)
	}
	inspection, err := inspector.InspectVaultProjection(context.Background(), relativePath, maximumReconciliationAssetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Exists || inspection.SizeBytes != int64(len(payload)) || inspection.SHA256 != fmt.Sprintf("%x", sha256.Sum256(payload)) {
		t.Fatalf("inspection = %+v", inspection)
	}
	for _, unsafePath := range []string{"../outside.md", "/tmp/outside.md", "documents/17/../outside.md", "documents\\outside.md", "documents/17/29/markdown/escaped.md"} {
		if _, err := inspector.InspectVaultProjection(context.Background(), unsafePath, maximumReconciliationAssetBytes); err == nil {
			t.Fatalf("InspectVaultProjection(%q) error=nil", unsafePath)
		}
	}
}

func TestVaultProjectionInspectorTreatsMissingDocumentsRootAsEmpty(t *testing.T) {
	inspector := &vaultProjectionFileInspector{root: t.TempDir()}
	listed, err := inspector.ListVaultProjections(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed = %+v", listed)
	}
}

func TestOrphanManifestOrderingAndCursorAreDeterministic(t *testing.T) {
	now := time.Now().UTC()
	objects := []evidenceLineageStoredAssetRecord{
		{Locator: "source-raw/v1/2/bb/b.raw", ModifiedAt: now},
		{Locator: "source-raw/v1/1/aa/a.raw", ModifiedAt: now.Add(-time.Hour)},
	}
	records, err := orphanEvidenceLineageManifests(objects, map[string]struct{}{}, "raw_object_orphan", rawOrphanReconciliationCursorBase, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Locator >= records[1].Locator || records[0].Cursor != rawOrphanReconciliationCursorBase+1 || records[1].Cursor != rawOrphanReconciliationCursorBase+2 {
		t.Fatalf("records = %+v", records)
	}
	resumed, err := orphanEvidenceLineageManifests(objects, map[string]struct{}{}, "raw_object_orphan", rawOrphanReconciliationCursorBase, records[0].Cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed) != 1 || resumed[0].Locator != records[1].Locator {
		t.Fatalf("resumed = %+v", resumed)
	}
}

func TestRawEvidenceObjectKeyValidationIsSourceScopedAndCanonical(t *testing.T) {
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("raw")))
	valid := "source-raw/v1/42/" + digest[:2] + "/" + digest + ".raw"
	if !validRawEvidenceObjectKey(valid) {
		t.Fatalf("validRawEvidenceObjectKey(%q)=false", valid)
	}
	for _, value := range []string{"../" + valid, "/" + valid, "source-raw/v1/42/ff/" + digest + ".raw", "source-raw/v1/42/" + digest[:2] + "/not-a-hash.raw"} {
		if validRawEvidenceObjectKey(value) {
			t.Fatalf("validRawEvidenceObjectKey(%q)=true", value)
		}
	}
}
