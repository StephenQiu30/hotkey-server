package evidence

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/evidencecapture"
)

func TestSelectorReplaysJSONPointerAndDigest(t *testing.T) {
	payload := []byte(`{"items":[{"text":"热点正文"}]}`)
	snapshot, err := evidencecapture.NewJSONSnapshot(payload, "api-test-v1", "https://api.example.test/items", "https://api.example.test/items", nil, 200, http.Header{}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := evidencecapture.SelectJSONPointer(payload, "/items/0")
	digest := sha256.Sum256(selected)
	input := sourceapplication.EvidenceSelectorInputDTO{
		Snapshot: sourceapplication.RawEvidenceSnapshotDTO{
			EvidenceKey: snapshot.Key, Payload: snapshot.Payload, PayloadSHA256: snapshot.PayloadSHA256,
			CollectorProfileVersion: snapshot.CollectorProfileVersion.String(), MIMEType: snapshot.MIMEType,
			ResponseStatus: snapshot.StatusCode, RequestedURL: snapshot.RequestedURL, FinalURL: snapshot.FinalURL,
			RedirectChain: snapshot.RedirectChain, CapturedAt: snapshot.CapturedAt,
		},
		Reference: sourceapplication.RawEvidenceReferenceDTO{
			EvidenceKey: snapshot.Key, Usage: "document_source", LocatorType: "json_pointer", LocatorValue: "/items/0",
			SelectedPayloadSHA256: fmt.Sprintf("%x", digest), SelectorVersion: evidencecapture.JSONPointerSelectorVersion,
		},
	}
	result, err := NewSelector().Select(input)
	if err != nil || string(result) != `{"text":"热点正文"}` {
		t.Fatalf("Select() = %s, %v", result, err)
	}
}
