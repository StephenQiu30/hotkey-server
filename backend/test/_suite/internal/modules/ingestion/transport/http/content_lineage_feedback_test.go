package http

import (
	"context"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type contentLineageFeedbackHTTPStub struct {
	command ingestionapplication.ReviewContentLineageCommand
	result  ingestionapplication.ReviewContentLineageResult
	calls   int
}

func (stub *contentLineageFeedbackHTTPStub) Review(_ context.Context, command ingestionapplication.ReviewContentLineageCommand) (ingestionapplication.ReviewContentLineageResult, error) {
	stub.calls++
	stub.command = command
	return stub.result, nil
}

func TestContentLineageFeedbackRouteRequiresVersionAndReturnsSafeResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &contentLineageFeedbackHTTPStub{result: ingestionapplication.ReviewContentLineageResult{Feedback: ingestionapplication.ContentLineageFeedbackDTO{
		FeedbackID: 31, LineageDecisionID: 11, ResultLineageDecisionID: 32, DocumentVersionID: 17,
		OriginalFamilyID: 19, ResultFamilyID: 23, ResultFamilyVersion: 2,
		OriginalRelation: "near_duplicate", ResultRelation: "unrelated", FeedbackType: "not_duplicate", ActorUserID: 7,
	}}}
	router := gin.New()
	RegisterContentLineageFeedbackRoutes(router, stub, quoteSelectorAuthenticator{role: httptransport.RoleEditor})
	response := performContentLineageFeedbackRequest(router, `{"expected_member_version":3,"feedback_type":"not_duplicate","reason_code":"different_work"}`, `"v3"`)
	if response.Code != stdhttp.StatusCreated || stub.calls != 1 || stub.command.ActorUserID != 7 ||
		stub.command.LineageDecisionID != 11 || stub.command.ExpectedMemberVersion != 3 || stub.command.IdempotencyKey != "lineage-review-11" {
		t.Fatalf("response/command = %d %s %#v", response.Code, response.Body.String(), stub.command)
	}
	for _, expected := range []string{`"feedback_id":31`, `"result_content_family_id":23`, `"result_relation":"unrelated"`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("response missing %s: %s", expected, response.Body.String())
		}
	}
	for _, forbidden := range []string{"command_fingerprint", "actor_user_id", "credibility", "confidence", "object_key"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, response.Body.String())
		}
	}
}

func TestContentLineageFeedbackRouteRejectsViewerUnknownFieldsAndStaleBodyVersion(t *testing.T) {
	stub := &contentLineageFeedbackHTTPStub{}
	viewer := gin.New()
	RegisterContentLineageFeedbackRoutes(viewer, stub, quoteSelectorAuthenticator{role: httptransport.RoleViewer})
	if response := performContentLineageFeedbackRequest(viewer, `{"expected_member_version":3,"feedback_type":"not_duplicate","reason_code":"different_work"}`, `"v3"`); response.Code != stdhttp.StatusForbidden {
		t.Fatalf("viewer status = %d body=%s", response.Code, response.Body.String())
	}

	editor := gin.New()
	RegisterContentLineageFeedbackRoutes(editor, stub, quoteSelectorAuthenticator{role: httptransport.RoleEditor})
	for _, body := range []string{
		`{"expected_member_version":2,"feedback_type":"not_duplicate","reason_code":"different_work"}`,
		`{"expected_member_version":3,"feedback_type":"not_duplicate","reason_code":"different_work","trusted":true}`,
	} {
		response := performContentLineageFeedbackRequest(editor, body, `"v3"`)
		if response.Code != stdhttp.StatusConflict && response.Code != stdhttp.StatusBadRequest {
			t.Fatalf("invalid input status = %d body=%s", response.Code, response.Body.String())
		}
	}
	if stub.calls != 0 {
		t.Fatalf("invalid requests reached service %d times", stub.calls)
	}
}

func performContentLineageFeedbackRequest(router *gin.Engine, body, etag string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/content-lineage-decisions/11/feedback", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer actor")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", etag)
	request.Header.Set("Idempotency-Key", "lineage-review-11")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
