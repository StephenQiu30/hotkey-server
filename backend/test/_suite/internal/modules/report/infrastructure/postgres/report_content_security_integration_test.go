//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	reportapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

type maliciousReportContentFixture struct {
	Cases []maliciousReportContentCase `json:"cases"`
}

type maliciousReportContentCase struct {
	Name    string `json:"name"`
	Payload string `json:"payload"`
}

type reportSecurityFacts struct {
	Reports, Items, Sentences, Citations, Transitions string
	Deliveries, DeliveryAttempts, KnowledgeDocuments  string
	KnowledgeProposals, KnowledgeRevisions, Jobs      string
}

func TestMaliciousReportFixtureCannotCreateApprovedOrDownstreamFacts(t *testing.T) {
	ctx := requestcontext.WithTraceID(requestcontext.WithRequestID(context.Background(), "report-xss-acceptance"), strings.Repeat("a", 32))
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	fixture := seedReportEvidenceFixture(t, runtime)
	repository := NewRepository(runtime)
	if err := repository.Save(ctx, fixture.report); err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Transition(ctx, domain.RevisionTransition{
		ReportID: fixture.report.ID, ExpectedVersion: fixture.report.Version, ActorID: fixture.actorID,
		From: domain.ReportDraft, To: domain.ReportPendingApproval,
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := reportapplication.NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	service.WithContentSecurityAudit(operationspostgres.NewAuditWriter(runtime))
	editor := identitydomain.Subject{UserID: fixture.actorID, SessionID: 1, Role: identitydomain.RoleEditor}
	attacks := readMaliciousReportContentFixture(t)

	for _, attack := range attacks.Cases {
		t.Run(attack.Name, func(t *testing.T) {
			injectUnsafeReportBody(t, runtime, pending.ID, attack.Payload)
			before := snapshotReportSecurityFacts(t, runtime)
			for attempt := 0; attempt < 2; attempt++ {
				_, err := service.ApproveRevision(ctx, reportapplication.RevisionLifecycleInput{
					Subject: editor, ReportID: pending.ID, ExpectedVersion: pending.Version,
				})
				if !errors.Is(err, domain.ErrUnsafeContent) {
					t.Errorf("attempt %d error = %v, want ErrUnsafeContent", attempt+1, err)
				}
			}
			if after := snapshotReportSecurityFacts(t, runtime); after != before {
				t.Errorf("business facts changed after repeated content attack: before=%#v after=%#v", before, after)
			}
		})
	}

	wantAudits := len(attacks.Cases) * 2
	var auditCount int
	var allSanitized bool
	var auditRows string
	if err := runtime.SQL.QueryRowContext(ctx, `
SELECT count(*),
       bool_and(actor_type='user' AND actor_id=$1 AND resource_type='report' AND resource_id=$2
                AND request_id='report-xss-acceptance' AND trace_id=$3 AND result='denied'
                AND before_data IS NULL AND after_data='{"reason_code":"report_content_unsafe"}'::jsonb),
       string_agg(row_to_json(audit_logs)::text, '')
FROM audit_logs WHERE action='report.content_rejected'`, fixture.actorID, pending.ID, strings.Repeat("a", 32)).
		Scan(&auditCount, &allSanitized, &auditRows); err != nil {
		t.Fatal(err)
	}
	if auditCount != wantAudits || !allSanitized {
		t.Errorf("content rejection audits = count:%d sanitized:%v, want %d sanitized rows", auditCount, allSanitized, wantAudits)
	}
	for _, attack := range attacks.Cases {
		if strings.Contains(auditRows, attack.Payload) || strings.Contains(auditRows, attack.Name) {
			t.Errorf("content rejection audit leaked fixture %q", attack.Name)
		}
	}
}

// injectUnsafeReportBody simulates a legacy/imported row that predates the
// current write boundary. Normal pending-report triggers deliberately prohibit
// this mutation; the publication service must still fail closed when storage
// is already compromised.
func injectUnsafeReportBody(t *testing.T, runtime *database.Runtime, reportID int64, payload string) {
	t.Helper()
	tx, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE reports SET body=$2 WHERE id=$1`, reportID, payload); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func readMaliciousReportContentFixture(t *testing.T) maliciousReportContentFixture {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "security", "malicious_report_content.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture maliciousReportContentFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("malicious report fixture must not be empty")
	}
	for _, test := range fixture.Cases {
		if strings.TrimSpace(test.Name) == "" || strings.TrimSpace(test.Payload) == "" {
			t.Fatalf("invalid malicious report fixture case: %#v", test)
		}
	}
	return fixture
}

func snapshotReportSecurityFacts(t *testing.T, runtime *database.Runtime) reportSecurityFacts {
	t.Helper()
	var facts reportSecurityFacts
	if err := runtime.SQL.QueryRow(`
SELECT
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(r)::text, '' ORDER BY id),'')) FROM reports r),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(i)::text, '' ORDER BY id),'')) FROM report_items i),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(s)::text, '' ORDER BY id),'')) FROM report_item_sentences s),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(c)::text, '' ORDER BY id),'')) FROM report_item_sentence_evidences c),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(t)::text, '' ORDER BY id),'')) FROM report_revision_transitions t),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(d)::text, '' ORDER BY id),'')) FROM report_deliveries d),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(a)::text, '' ORDER BY id),'')) FROM delivery_attempts a),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(d)::text, '' ORDER BY id),'')) FROM knowledge_documents d),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(p)::text, '' ORDER BY id),'')) FROM knowledge_change_proposals p),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(r)::text, '' ORDER BY id),'')) FROM knowledge_revisions r),
  (SELECT count(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(j)::text, '' ORDER BY id),'')) FROM river_job j)`).
		Scan(&facts.Reports, &facts.Items, &facts.Sentences, &facts.Citations, &facts.Transitions,
			&facts.Deliveries, &facts.DeliveryAttempts, &facts.KnowledgeDocuments,
			&facts.KnowledgeProposals, &facts.KnowledgeRevisions, &facts.Jobs); err != nil {
		t.Fatal(err)
	}
	return facts
}
