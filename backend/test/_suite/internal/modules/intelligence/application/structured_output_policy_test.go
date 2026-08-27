package application

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

func TestStructuredOutputPolicyRejectsEvidenceOutsideExactInputWhitelist(t *testing.T) {
	input := json.RawMessage(`{"event_id":7,"event_key":"evt-7","evidence":[{"content_id":2,"locator":"title","excerpt":"trusted exact excerpt"}]}`)
	for _, testCase := range []struct {
		name     string
		taskType domain.TaskType
		output   json.RawMessage
	}{
		{
			name: "summary forged id", taskType: domain.TaskTypeEventSummary,
			output: json.RawMessage(`{"title_zh":"事件","sentences":[{"text":"事实","evidence":[{"content_id":999,"locator":"title"}]}]}`),
		},
		{
			name: "summary forged locator", taskType: domain.TaskTypeEventSummary,
			output: json.RawMessage(`{"title_zh":"事件","sentences":[{"text":"事实","evidence":[{"content_id":2,"locator":"body:99"}]}]}`),
		},
		{
			name: "summary altered excerpt", taskType: domain.TaskTypeEventSummary,
			output: json.RawMessage(`{"title_zh":"事件","sentences":[{"text":"事实","evidence":[{"content_id":2,"locator":"title","excerpt":"fabricated excerpt"}]}]}`),
		},
		{
			name: "claim forged id", taskType: domain.TaskTypeEntityClaimExtraction,
			output: json.RawMessage(`{"entities":[],"claims":[{"claim":"事实","evidence":[{"content_id":999,"locator":"title"}]}]}`),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := validateStructuredOutputPolicy(testCase.taskType, "v1", input, testCase.output); err == nil {
				t.Fatal("forged evidence was accepted")
			} else if code, known := domain.CodeOf(err); !known || code != domain.CodeAIOutputInvalid {
				t.Fatalf("error=%v code=%d known=%v", err, code, known)
			}
		})
	}
	valid := json.RawMessage(`{"title_zh":"事件","sentences":[{"text":"事实","evidence":[{"content_id":2,"locator":"title","excerpt":"trusted exact excerpt"}]}]}`)
	if err := validateStructuredOutputPolicy(domain.TaskTypeEventSummary, "v1", input, valid); err != nil {
		t.Fatalf("exact whitelisted evidence was rejected: %v", err)
	}
}

func TestStructuredOutputPolicyRequiresV2ExactQuoteFromAuthorizedBody(t *testing.T) {
	input := json.RawMessage(`{"body":"Acme 发布了 Nova。","document_version_id":11}`)
	valid := json.RawMessage(`{"claims":[{"exact_quote":"Acme 发布了 Nova。"}]}`)
	if err := validateStructuredOutputPolicy(domain.TaskTypeEntityClaimExtraction, "v2", input, valid); err != nil {
		t.Fatalf("exact quote was rejected: %v", err)
	}
	forged := json.RawMessage(`{"claims":[{"exact_quote":"Acme 发布了伪造产品。"}]}`)
	if err := validateStructuredOutputPolicy(domain.TaskTypeEntityClaimExtraction, "v2", input, forged); err == nil {
		t.Fatal("quote outside the authorized body was accepted")
	}
}
