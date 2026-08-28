package application

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type recoveryFactsFake struct{ fact VaultRebuildFact }

func (fake recoveryFactsFake) LoadVaultRebuildFact(context.Context, int64) (VaultRebuildFact, error) {
	return fake.fact, nil
}

type recoveryVaultFake struct {
	content string
	missing bool
	writes  int
}

func (fake *recoveryVaultFake) Read(string, string) ([]byte, string, error) {
	if fake.missing {
		return nil, "", os.ErrNotExist
	}
	return []byte(fake.content), "reports/17.md", nil
}

func (fake *recoveryVaultFake) CompareAndSwap(_, _ string, expectedHash, replacement string) (string, error) {
	currentHash := domain.HashContent("", fake.content)
	if fake.missing {
		currentHash = domain.HashContent("", "")
	}
	if currentHash != expectedHash {
		return "", domain.ErrVaultConflict
	}
	fake.content = replacement
	fake.missing = false
	fake.writes++
	return domain.HashContent("", replacement), nil
}

type recoveryProtectedSourceFake struct {
	content string
	err     error
	reads   int
}

func (fake *recoveryProtectedSourceFake) ReadVaultSnapshot(context.Context, string, int64) (string, error) {
	fake.reads++
	return fake.content, fake.err
}

func (fake *recoveryProtectedSourceFake) ReadVaultBackup(context.Context, int64, int64, int64) (string, error) {
	fake.reads++
	return fake.content, fake.err
}

func TestVaultRecoveryRestoresMissingFileFromRevisionSnapshot(t *testing.T) {
	fact, protected := recoveryFact(t)
	vault := &recoveryVaultFake{missing: true}
	revision := &recoveryProtectedSourceFake{content: protected}
	service := NewVaultRecoveryService(recoveryFactsFake{fact: fact}, vault, revision, nil)

	result, err := service.Recover(context.Background(), fact.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Restored || result.Source != domain.VaultRecoveryRevision || result.ContentHash != fact.Document.ContentHash || vault.content != protected || vault.writes != 1 || revision.reads != 1 {
		t.Fatalf("recovery result = %#v vault=%#v revision=%#v", result, vault, revision)
	}
}

func TestVaultRecoveryInspectionVerifiesProtectedHumanRegionWithoutWriting(t *testing.T) {
	fact, protected := recoveryFact(t)
	vault := &recoveryVaultFake{missing: true}
	revision := &recoveryProtectedSourceFake{content: protected}
	service := NewVaultRecoveryService(recoveryFactsFake{fact: fact}, vault, revision, nil)

	result, err := service.Inspect(context.Background(), fact.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedHumanSHA256, err := domain.VaultHumanRegionSHA256(protected)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Missing || result.DocumentID != fact.Document.ID || result.RevisionNo != fact.Document.RevisionNo ||
		result.ContentHash != fact.Document.ContentHash || result.HumanRegionSHA256 != expectedHumanSHA256 ||
		result.Source != domain.VaultRecoveryRevision || vault.writes != 0 || revision.reads != 1 {
		t.Fatalf("inspection=%+v vault=%+v revision=%+v", result, vault, revision)
	}
}

func TestVaultRecoveryFallsBackToBackupOnlyWhenRevisionIsMissing(t *testing.T) {
	fact, protected := recoveryFact(t)
	vault := &recoveryVaultFake{missing: true}
	revision := &recoveryProtectedSourceFake{err: os.ErrNotExist}
	backup := &recoveryProtectedSourceFake{content: protected}
	service := NewVaultRecoveryService(recoveryFactsFake{fact: fact}, vault, revision, backup)

	result, err := service.Recover(context.Background(), fact.Document.ID)
	if err != nil || result.Source != domain.VaultRecoveryBackup || !result.Restored || backup.reads != 1 {
		t.Fatalf("recovery result = %#v/%v revision=%#v backup=%#v", result, err, revision, backup)
	}
}

func TestVaultRecoveryStopsOnCurrentConflictWithoutReadingOlderCopies(t *testing.T) {
	fact, protected := recoveryFact(t)
	tampered := strings.Replace(protected, domain.HumanRegionBegin, domain.HumanRegionBegin+"\nuntracked edit", 1)
	vault := &recoveryVaultFake{content: tampered}
	revision := &recoveryProtectedSourceFake{content: protected}
	backup := &recoveryProtectedSourceFake{content: protected}
	service := NewVaultRecoveryService(recoveryFactsFake{fact: fact}, vault, revision, backup)

	if _, err := service.Recover(context.Background(), fact.Document.ID); !errors.Is(err, domain.ErrVaultConflict) || !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Recover() error = %v", err)
	}
	if vault.writes != 0 || vault.content != tampered || revision.reads != 0 || backup.reads != 0 {
		t.Fatalf("conflict side effects: vault=%#v revision=%#v backup=%#v", vault, revision, backup)
	}
}

func TestVaultRecoveryStopsWhenEveryProtectedHumanRegionIsMissing(t *testing.T) {
	fact, _ := recoveryFact(t)
	vault := &recoveryVaultFake{missing: true}
	revision := &recoveryProtectedSourceFake{err: os.ErrNotExist}
	backup := &recoveryProtectedSourceFake{err: os.ErrNotExist}
	service := NewVaultRecoveryService(recoveryFactsFake{fact: fact}, vault, revision, backup)

	if _, err := service.Recover(context.Background(), fact.Document.ID); !errors.Is(err, domain.ErrVaultHumanRegionUnavailable) || !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Recover() error = %v", err)
	}
	if vault.writes != 0 {
		t.Fatalf("Vault writes = %d", vault.writes)
	}
}

func TestVaultRecoveryAuditsUnsafeRevisionWithoutPublishing(t *testing.T) {
	fact, protected := recoveryFact(t)
	unsafe := strings.Replace(protected, domain.HumanRegionBegin, domain.HumanRegionBegin+"\n<img src=x onerror=sentinel>", 1)
	fact.Document.ContentHash = domain.HashContent("", unsafe)
	vault := &recoveryVaultFake{missing: true}
	revision := &recoveryProtectedSourceFake{content: unsafe}
	audit := &vaultSecurityAuditFake{}
	service := NewVaultRecoveryService(recoveryFactsFake{fact: fact}, vault, revision, nil, audit)

	_, err := service.Recover(context.Background(), fact.Document.ID)
	if !errors.Is(err, domain.ErrVaultContentUnsafe) || !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("Recover() error = %v", err)
	}
	if vault.writes != 0 || len(audit.entries) != 1 || audit.entries[0].After["reason_code"] != domain.VaultReasonContentUnsafe {
		t.Fatalf("unsafe recovery side effects/audit = vault:%#v audit:%#v", vault, audit.entries)
	}
}

func recoveryFact(t *testing.T) (VaultRebuildFact, string) {
	t.Helper()
	input := domain.VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 3, Type: domain.DocumentReport, SourceID: 91,
		Title: "日报", Generated: "approved generated body",
	}
	content, err := domain.RenderVaultDocument(input)
	if err != nil {
		t.Fatal(err)
	}
	human := domain.HumanRegionBegin + "\n人工笔记  \n" + domain.HumanRegionEnd
	content = strings.Replace(content, domain.HumanRegionBegin+"\n"+domain.HumanRegionEnd, human, 1)
	reportID := int64(91)
	return VaultRebuildFact{
		Document: domain.Document{
			ID: 17, Version: 4, RevisionNo: 3, Type: domain.DocumentReport, ReportID: &reportID,
			VaultPath: "reports/17.md", ContentHash: domain.HashContent("", content), Status: domain.DocumentActive,
		},
		RenderInput: input, SnapshotObjectKey: "knowledge/v1/17/3.md",
	}, content
}
