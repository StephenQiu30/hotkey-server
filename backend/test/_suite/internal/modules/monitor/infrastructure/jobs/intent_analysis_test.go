package jobs

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestIntentAnalysisJobArgsRoundTripOnlyBoundedRunIdentity(t *testing.T) {
	t.Parallel()
	args := IntentAnalysisJobArgs{
		RunID: 41, DraftID: 11, DraftResourceVersion: 3,
	}
	encoded, err := EncodeIntentAnalysisJobArgs(args)
	if err != nil {
		t.Fatalf("EncodeIntentAnalysisJobArgs(): %v", err)
	}
	decoded, err := DecodeIntentAnalysisJobArgs(encoded)
	if err != nil {
		t.Fatalf("DecodeIntentAnalysisJobArgs(): %v", err)
	}
	if !reflect.DeepEqual(decoded, args) {
		t.Fatalf("decoded = %#v, want %#v", decoded, args)
	}
	for _, forbidden := range []string{"body", "raw", "markdown", "document", "objective", "clause", "candidate"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("durable args leaked %q: %s", forbidden, encoded)
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil || len(object) != 3 {
		t.Fatalf("durable args shape = %v / %v", object, err)
	}
}

func TestIntentAnalysisJobArgsRejectUnknownAndInvalidFacts(t *testing.T) {
	t.Parallel()
	tests := []string{
		`{}`,
		`{"run_id":0,"draft_id":1,"draft_resource_version":1}`,
		`{"run_id":1,"draft_id":0,"draft_resource_version":1}`,
		`{"run_id":1,"draft_id":1,"draft_resource_version":0}`,
		`{"run_id":1,"draft_id":1,"draft_resource_version":1,"body":"secret"}`,
		`[]`,
	}
	for _, encoded := range tests {
		if _, err := DecodeIntentAnalysisJobArgs([]byte(encoded)); err == nil {
			t.Errorf("DecodeIntentAnalysisJobArgs accepted %s", encoded)
		}
	}
}
