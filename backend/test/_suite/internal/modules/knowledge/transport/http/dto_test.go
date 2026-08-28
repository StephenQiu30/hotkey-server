package http

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
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

func TestKnowledgeErrorMapsVaultSecurityReasonsToStableCodes(t *testing.T) {
	tests := []struct {
		err  error
		code int
	}{
		{err: knowledgedomain.ErrVaultPathInvalid, code: sharederrors.CodeKnowledgePathInvalid},
		{err: knowledgedomain.ErrVaultContentUnsafe, code: sharederrors.CodeKnowledgeContentUnsafe},
		{err: knowledgedomain.ErrVaultPathSymlink, code: sharederrors.CodeKnowledgePathSymlink},
	}
	for _, test := range tests {
		mapped := knowledgeError(test.err)
		var appError *sharederrors.AppError
		if !errors.As(mapped, &appError) || appError.Code != test.code || strings.Contains(appError.Error(), "/Users/") {
			t.Errorf("knowledgeError(%v) = %#v", test.err, mapped)
		}
	}
}
