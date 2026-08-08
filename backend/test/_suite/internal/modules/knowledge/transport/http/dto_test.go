package http

import (
	"encoding/json"
	"strings"
	"testing"

	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
)

func TestKnowledgeResponsesMatchGeneratedLowerCamelContract(t *testing.T) {
	reportID := int64(9)
	values := []any{
		documentResponse(knowledgedomain.Document{ID: 3, Version: 1, RevisionNo: 0, Type: knowledgedomain.DocumentReport, VaultPath: "reports/9.md", Status: knowledgedomain.DocumentPlanned, ReportID: &reportID}),
		proposalResponse(knowledgedomain.Proposal{ID: 5, Version: 1, DocumentID: 3, BaseRevisionNo: 0, BaseHash: strings.Repeat("a", 64), DiffSummary: "归档报告", Status: knowledgedomain.ProposalPending}),
		reconciliationResponse(knowledgedomain.ReconciliationReport{Scanned: 1, Changed: 1}),
	}
	for _, value := range values {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"ID"`) || strings.Contains(string(body), `"RevisionNo"`) || strings.Contains(string(body), `"Scanned"`) {
			t.Fatalf("response escaped generated JSON contract: %s", body)
		}
	}
}
