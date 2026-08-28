package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestStablePathRejectsTraversal(t *testing.T) {
	if _, err := StablePath("/tmp/vault", "events", "../escape"); err == nil {
		t.Fatal("path traversal accepted")
	}
	path, err := StablePath("/tmp/vault", "events", "evt-1")
	if err != nil || path != "/tmp/vault/events/evt-1.md" {
		t.Fatalf("path = %q/%v", path, err)
	}
	for _, kind := range []string{"documents", "documents/1/2/markdown", "unknown"} {
		if _, err := StablePath("/tmp/vault", kind, "projection-hash"); err == nil {
			t.Errorf("reserved or unknown legacy kind %q was accepted", kind)
		}
	}
}

func TestMergeAutomaticRegionPreservesHumanNotes(t *testing.T) {
	existing := "# Note\n\nHuman note.\n\n" + AutomaticRegionBegin + "\nold\n" + AutomaticRegionEnd + "\n"
	merged, err := MergeAutomaticRegion(existing, "new generated facts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(merged, "Human note.") || strings.Contains(merged, "old") || !strings.Contains(merged, "new generated facts") {
		t.Fatalf("merged document lost manual/automatic content: %q", merged)
	}
}

func TestRenderVaultDocumentIsDeterministicAndSeparatesOwnedRegions(t *testing.T) {
	input := VaultDocumentRenderInput{
		DocumentID: 17,
		RevisionNo: 4,
		Type:       DocumentReport,
		SourceID:   91,
		Title:      `每日 "热点"`,
		Generated:  "## 摘要\n\n- 事实 A\n",
	}

	first, err := RenderVaultDocument(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderVaultDocument(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("identical Vault render input produced different bytes")
	}
	wantLines := []string{
		"---",
		"hotkey_schema: 1",
		"hotkey_document_id: 17",
		"hotkey_document_type: report",
		"hotkey_source_id: 91",
		"hotkey_revision: 4",
		"hotkey_generated_sha256: " + HashContent("", input.Generated),
		`title: "每日 \"热点\""`,
		"---",
		"",
		AutomaticRegionBegin,
		"## 摘要",
		"",
		"- 事实 A",
		AutomaticRegionEnd,
		"",
		HumanRegionBegin,
		HumanRegionEnd,
		"",
	}
	if lines := strings.Split(first, "\n"); !reflect.DeepEqual(lines, wantLines) {
		t.Fatalf("Vault Markdown lines = %#v, want %#v", lines, wantLines)
	}
	if strings.Count(first, AutomaticRegionBegin) != 1 || strings.Count(first, AutomaticRegionEnd) != 1 ||
		strings.Count(first, HumanRegionBegin) != 1 || strings.Count(first, HumanRegionEnd) != 1 {
		t.Fatalf("Vault Markdown markers are not unique: %q", first)
	}
}

func TestRenderVaultDocumentRejectsUnstableIdentityAndMarkerInjection(t *testing.T) {
	valid := VaultDocumentRenderInput{
		DocumentID: 17,
		RevisionNo: 1,
		Type:       DocumentEvent,
		SourceID:   91,
		Title:      "事件",
		Generated:  "事实",
	}

	cases := map[string]func(*VaultDocumentRenderInput){
		"missing document identity": func(input *VaultDocumentRenderInput) { input.DocumentID = 0 },
		"missing source identity":   func(input *VaultDocumentRenderInput) { input.SourceID = 0 },
		"invalid document type":     func(input *VaultDocumentRenderInput) { input.Type = "unknown" },
		"multiline title":           func(input *VaultDocumentRenderInput) { input.Title = "title\ninjected: true" },
		"automatic marker":          func(input *VaultDocumentRenderInput) { input.Generated = AutomaticRegionBegin },
		"human marker":              func(input *VaultDocumentRenderInput) { input.Generated = HumanRegionBegin },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := RenderVaultDocument(input); err == nil {
				t.Fatal("RenderVaultDocument() error = nil")
			}
		})
	}
}

func TestUpdateVaultDocumentPreservesHumanRegionByteForByte(t *testing.T) {
	initial, err := RenderVaultDocument(VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 1, Type: DocumentReport, SourceID: 91,
		Title: "日报 v1", Generated: "generated v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	human := HumanRegionBegin + "\n人工笔记  \n\n- [ ] 后续核查\n" + HumanRegionEnd
	initial = strings.Replace(initial, HumanRegionBegin+"\n"+HumanRegionEnd, human, 1)

	updated, err := UpdateVaultDocument(initial, VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 2, Type: DocumentReport, SourceID: 91,
		Title: "日报 v2", Generated: "generated v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated, human) || strings.Contains(updated, "generated v1") || !strings.Contains(updated, "generated v2") {
		t.Fatalf("updated Vault document = %q", updated)
	}
	if strings.Count(updated, human) != 1 {
		t.Fatalf("human region changed or duplicated: %q", updated)
	}

	repeated, err := UpdateVaultDocument(updated, VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 2, Type: DocumentReport, SourceID: 91,
		Title: "日报 v2", Generated: "generated v2",
	})
	if err != nil || repeated != updated {
		t.Fatalf("idempotent update = %q/%v", repeated, err)
	}
}

func TestUpdateVaultDocumentStopsOnIdentityRevisionAndMarkerConflicts(t *testing.T) {
	initial, err := RenderVaultDocument(VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 3, Type: DocumentEvent, SourceID: 91,
		Title: "事件", Generated: "generated",
	})
	if err != nil {
		t.Fatal(err)
	}
	valid := VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 4, Type: DocumentEvent, SourceID: 91,
		Title: "事件", Generated: "updated",
	}
	cases := map[string]struct {
		existing string
		mutate   func(*VaultDocumentRenderInput)
	}{
		"missing markers":     {existing: "# human-only\n"},
		"duplicate auto":      {existing: initial + AutomaticRegionBegin},
		"overlapping regions": {existing: strings.Replace(initial, HumanRegionBegin, AutomaticRegionEnd+"\n"+HumanRegionBegin, 1)},
		"document identity":   {existing: initial, mutate: func(input *VaultDocumentRenderInput) { input.DocumentID++ }},
		"source identity":     {existing: initial, mutate: func(input *VaultDocumentRenderInput) { input.SourceID++ }},
		"revision skipped":    {existing: initial, mutate: func(input *VaultDocumentRenderInput) { input.RevisionNo += 2 }},
		"revision regressed":  {existing: initial, mutate: func(input *VaultDocumentRenderInput) { input.RevisionNo = 2 }},
	}
	for name, fixture := range cases {
		t.Run(name, func(t *testing.T) {
			input := valid
			if fixture.mutate != nil {
				fixture.mutate(&input)
			}
			if _, err := UpdateVaultDocument(fixture.existing, input); err == nil {
				t.Fatal("UpdateVaultDocument() error = nil")
			}
		})
	}
}

func TestRecoverVaultDocumentUsesOnlyProtectedHumanRegionSources(t *testing.T) {
	initial, err := RenderVaultDocument(VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 1, Type: DocumentReport, SourceID: 91,
		Title: "日报 v1", Generated: "generated v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	human := HumanRegionBegin + "\n人工恢复笔记  \n\n- [ ] 保留原始空白\n" + HumanRegionEnd
	initial = strings.Replace(initial, HumanRegionBegin+"\n"+HumanRegionEnd, human, 1)
	input := VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 2, Type: DocumentReport, SourceID: 91,
		Title: "日报 v2", Generated: "generated v2",
	}
	tests := []struct {
		name    string
		sources VaultRecoverySources
		want    VaultRecoverySource
	}{
		{name: "current Vault", sources: VaultRecoverySources{Current: initial}, want: VaultRecoveryCurrent},
		{name: "Knowledge Revision", sources: VaultRecoverySources{Revision: initial}, want: VaultRecoveryRevision},
		{name: "backup", sources: VaultRecoverySources{Backup: initial}, want: VaultRecoveryBackup},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.sources.ExpectedHash = HashContent("", initial)
			result, err := RecoverVaultDocument(test.sources, input)
			if err != nil {
				t.Fatal(err)
			}
			if result.Source != test.want || !strings.Contains(result.Content, human) || strings.Contains(result.Content, "generated v1") || !strings.Contains(result.Content, "generated v2") {
				t.Fatalf("recovery result = %#v", result)
			}
		})
	}
}

func TestRecoverVaultDocumentStopsWithoutProtectedHumanRegionOrOnConflict(t *testing.T) {
	initial, err := RenderVaultDocument(VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 1, Type: DocumentReport, SourceID: 91,
		Title: "日报", Generated: "generated",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 2, Type: DocumentReport, SourceID: 91,
		Title: "日报", Generated: "updated",
	}
	if _, err := RecoverVaultDocument(VaultRecoverySources{ExpectedHash: HashContent("", initial)}, input); !errors.Is(err, ErrVaultHumanRegionUnavailable) {
		t.Fatalf("missing protected source error = %v", err)
	}
	tampered := strings.Replace(initial, HumanRegionBegin, HumanRegionBegin+"\nuntracked edit", 1)
	if _, err := RecoverVaultDocument(VaultRecoverySources{
		ExpectedHash: HashContent("", initial), Current: tampered, Revision: initial,
	}, input); !errors.Is(err, ErrVaultConflict) {
		t.Fatalf("current Vault conflict error = %v", err)
	}
	wrongIdentity, err := RenderVaultDocument(VaultDocumentRenderInput{
		DocumentID: 18, RevisionNo: 1, Type: DocumentReport, SourceID: 91,
		Title: "其他日报", Generated: "generated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverVaultDocument(VaultRecoverySources{
		ExpectedHash: HashContent("", wrongIdentity), Revision: wrongIdentity, Backup: initial,
	}, input); !errors.Is(err, ErrVaultConflict) {
		t.Fatalf("revision identity conflict error = %v", err)
	}
}
