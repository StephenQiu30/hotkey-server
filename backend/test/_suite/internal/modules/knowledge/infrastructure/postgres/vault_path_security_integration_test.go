//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

type vaultPathSecurityFixture struct {
	InvalidPaths []vaultPathSecurityCase `json:"invalid_paths"`
	SymlinkPaths []vaultPathSecurityCase `json:"symlink_paths"`
}

type vaultPathSecurityCase struct {
	Name, Path, Kind string
}

func TestVaultPathSecurityFixtureRejectsBeforeIOAndPreservesFilesAndBusinessFacts(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	fixture := readVaultPathSecurityFixture(t)
	vaultRoot := filepath.Join(t.TempDir(), "vault")
	outsideRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultRoot, "events"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(vaultRoot, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "reports", "inside-guard.md"), []byte("inside guard"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideRoot, "leaf-secret.md"), []byte("outside leaf secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideRoot, "directory-escape.md"), []byte("outside directory secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range fixture.SymlinkPaths {
		parts := strings.Split(filepath.ToSlash(test.Path), "/")
		if len(parts) != 2 {
			t.Fatalf("symlink fixture path %q must use one controlled namespace", test.Path)
		}
		var target, link string
		switch test.Kind {
		case "leaf":
			target = filepath.Join(outsideRoot, "leaf-secret.md")
			link = filepath.Join(vaultRoot, filepath.FromSlash(test.Path))
		case "directory":
			target = outsideRoot
			link = filepath.Join(vaultRoot, parts[0])
		default:
			t.Fatalf("unknown symlink fixture kind %q", test.Kind)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	repository := NewRepository(runtime)
	service := knowledgeapplication.NewProposalService(repository, repository).
		WithVaultSecurityAudit(operationspostgres.NewAuditWriter(runtime))
	writer := knowledgevault.NewWriter(vaultRoot)
	type attack struct {
		fixture  vaultPathSecurityCase
		proposal domain.Proposal
		want     error
	}
	attacks := make([]attack, 0, len(fixture.InvalidPaths)+len(fixture.SymlinkPaths))
	for _, test := range fixture.InvalidPaths {
		attacks = append(attacks, attack{fixture: test, proposal: createApprovedVaultPathProposal(t, runtime, test), want: domain.ErrVaultPathInvalid})
	}
	for _, test := range fixture.SymlinkPaths {
		attacks = append(attacks, attack{fixture: test, proposal: createApprovedVaultPathProposal(t, runtime, test), want: domain.ErrVaultPathSymlink})
	}

	beforeVault := snapshotVaultPathTree(t, vaultRoot)
	beforeOutside := snapshotVaultPathTree(t, outsideRoot)
	beforeFacts := snapshotVaultPathBusinessFacts(t, runtime)
	for _, attack := range attacks {
		for attempt := 0; attempt < 2; attempt++ {
			_, err := service.Apply(ctx, attack.proposal, writer)
			if !errors.Is(err, attack.want) || !errors.Is(err, sharedrepository.ErrInvalidInput) {
				t.Errorf("%s attempt %d error = %v, want sanitized path rejection", attack.fixture.Name, attempt+1, err)
			}
		}
	}
	if after := snapshotVaultPathTree(t, vaultRoot); after != beforeVault {
		t.Errorf("Vault root changed after repeated path attacks: before=%s after=%s", beforeVault, after)
	}
	if after := snapshotVaultPathTree(t, outsideRoot); after != beforeOutside {
		t.Errorf("outside files changed after repeated path attacks: before=%s after=%s", beforeOutside, after)
	}
	if after := snapshotVaultPathBusinessFacts(t, runtime); after != beforeFacts {
		t.Errorf("knowledge business facts changed after repeated path attacks: before=%#v after=%#v", beforeFacts, after)
	}

	wantAudits := 2 * len(attacks)
	var auditCount, invalidCount, symlinkCount int
	var allSanitized bool
	var auditRows string
	if err := runtime.SQL.QueryRow(`
SELECT count(*),
       count(*) FILTER (WHERE after_data = '{"reason_code":"vault_path_invalid"}'::jsonb),
       count(*) FILTER (WHERE after_data = '{"reason_code":"vault_path_symlink"}'::jsonb),
       bool_and(actor_type='system' AND resource_type='knowledge_document' AND result='denied'
                AND before_data IS NULL AND after_data IN (
                  '{"reason_code":"vault_path_invalid"}'::jsonb,
                  '{"reason_code":"vault_path_symlink"}'::jsonb
                )),
       string_agg(row_to_json(audit_logs)::text, '')
FROM audit_logs WHERE action='knowledge.projection_rejected'`).
		Scan(&auditCount, &invalidCount, &symlinkCount, &allSanitized, &auditRows); err != nil {
		t.Fatal(err)
	}
	if auditCount != wantAudits || invalidCount != 2*len(fixture.InvalidPaths) || symlinkCount != 2*len(fixture.SymlinkPaths) || !allSanitized {
		t.Errorf("Vault rejection audits = total:%d invalid:%d symlink:%d sanitized:%v", auditCount, invalidCount, symlinkCount, allSanitized)
	}
	for _, forbidden := range append([]string{vaultRoot, outsideRoot, "outside leaf secret", "outside directory secret"}, fixtureVaultPaths(fixture)...) {
		if strings.Contains(auditRows, forbidden) {
			t.Errorf("Vault rejection audit leaked path or file content %q", forbidden)
		}
	}
}

func readVaultPathSecurityFixture(t *testing.T) vaultPathSecurityFixture {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "security", "vault_paths.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture vaultPathSecurityFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.InvalidPaths) == 0 || len(fixture.SymlinkPaths) == 0 {
		t.Fatal("Vault path security fixture must cover invalid and symlink paths")
	}
	return fixture
}

func createApprovedVaultPathProposal(t *testing.T, runtime *database.Runtime, fixture vaultPathSecurityCase) domain.Proposal {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, time.August, 28, 18, 0, 0, 0, time.UTC)
	var eventID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO events (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at)
VALUES ($1,'Vault security fixture','','active',$2,$2) RETURNING id`, "vault-path-"+strings.ReplaceAll(fixture.Name, " ", "-"), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	emptyHash := domain.HashContent("", "")
	var documentID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO knowledge_documents (version,document_type,event_id,vault_path,revision_no,content_hash,generated_hash,status)
VALUES (1,'event',$1,$2,0,$3,$3,'planned') RETURNING id`, eventID, fixture.Path, emptyHash).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	proposal := domain.Proposal{
		Version: 2, DocumentID: documentID, BaseRevisionNo: 0, BaseHash: emptyHash,
		ProposedFrontmatter: `{"title":"Safe fixture"}`, ProposedBody: "safe generated body", Status: domain.ProposalApproved,
	}
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO knowledge_change_proposals
  (version,document_id,change_type,base_revision_no,base_hash,proposed_frontmatter,proposed_body,reason,status)
VALUES ($1,$2,'update',$3,$4,$5::jsonb,$6,'vault path acceptance','approved') RETURNING id`,
		proposal.Version, proposal.DocumentID, proposal.BaseRevisionNo, proposal.BaseHash,
		proposal.ProposedFrontmatter, proposal.ProposedBody).Scan(&proposal.ID); err != nil {
		t.Fatal(err)
	}
	return proposal
}

type vaultPathBusinessFacts struct {
	Documents, Proposals, Revisions, Annotations string
}

func snapshotVaultPathBusinessFacts(t *testing.T, runtime *database.Runtime) vaultPathBusinessFacts {
	t.Helper()
	var facts vaultPathBusinessFacts
	if err := runtime.SQL.QueryRow(`
SELECT
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(d)::text, '' ORDER BY id),'')) FROM knowledge_documents d),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(p)::text, '' ORDER BY id),'')) FROM knowledge_change_proposals p),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(r)::text, '' ORDER BY id),'')) FROM knowledge_revisions r),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(a)::text, '' ORDER BY id),'')) FROM knowledge_annotations a)`).
		Scan(&facts.Documents, &facts.Proposals, &facts.Revisions, &facts.Annotations); err != nil {
		t.Fatal(err)
	}
	return facts
}

func snapshotVaultPathTree(t *testing.T, root string) string {
	t.Helper()
	records := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			records = append(records, "link:"+relative+"->"+target)
			return nil
		}
		if entry.IsDir() {
			records = append(records, "dir:"+relative)
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		records = append(records, "file:"+relative+":"+hex.EncodeToString(digest[:]))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(records)
	digest := sha256.Sum256([]byte(strings.Join(records, "\n")))
	return hex.EncodeToString(digest[:])
}

func fixtureVaultPaths(fixture vaultPathSecurityFixture) []string {
	paths := make([]string, 0, len(fixture.InvalidPaths)+len(fixture.SymlinkPaths))
	for _, test := range append(fixture.InvalidPaths, fixture.SymlinkPaths...) {
		paths = append(paths, test.Path)
	}
	return paths
}
