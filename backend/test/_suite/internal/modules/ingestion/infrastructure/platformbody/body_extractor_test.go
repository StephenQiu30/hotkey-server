package platformbody

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionmarkdown "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/markdown"
)

func TestBodyExtractorProjectsExactXPostWithoutTreatingMarkupAsHTML(t *testing.T) {
	payload := []byte(`{"id":"200","text":"Public *launch* <not-html>\n第二行","author_id":"u1"}`)
	digest := sha256.Sum256(payload)
	extractor := NewBodyExtractor(ingestionmarkdown.NewConverter(), ingestionmarkdown.NewAnchorMapper())
	result, err := extractor.Extract(context.Background(), ingestionapplication.ExtractSelectedSourceBodyCommand{Evidence: ingestionapplication.SelectedSourceEvidenceDTO{
		SourceCode: "x", ContentType: "post", BodyOrigin: ingestionapplication.BodyOriginPlatformPost,
		Completeness: ingestionapplication.BodyCompletenessFull, SelectedPayload: payload,
		SelectedPayloadSHA256: fmt.Sprintf("%x", digest), PayloadMIMEType: "application/json",
		SelectorVersion: "rfc6901-json-number-preserving-v1", CanonicalURL: "https://x.com/official/status/200",
	}})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if result.BodyOrigin != ingestionapplication.BodyOriginPlatformPost || result.Completeness != ingestionapplication.BodyCompletenessFull ||
		result.Plaintext != "Public *launch* <not-html>\n\n第二行" || result.Markdown == "" || len(result.AnchorBlocks) != 2 {
		t.Fatalf("Extract() = %#v", result)
	}
}

func TestBodyExtractorKeepsMetadataOnlyPlatformRecordsBodyless(t *testing.T) {
	payload := []byte(`{"id":"201","text":""}`)
	digest := sha256.Sum256(payload)
	result, err := NewBodyExtractor(ingestionmarkdown.NewConverter(), ingestionmarkdown.NewAnchorMapper()).Extract(context.Background(), ingestionapplication.ExtractSelectedSourceBodyCommand{Evidence: ingestionapplication.SelectedSourceEvidenceDTO{
		SourceCode: "x", ContentType: "post", BodyOrigin: ingestionapplication.BodyOriginPlatformPost,
		Completeness: ingestionapplication.BodyCompletenessMetadataOnly, SelectedPayload: payload,
		SelectedPayloadSHA256: fmt.Sprintf("%x", digest), PayloadMIMEType: "application/json",
		SelectorVersion: "rfc6901-json-number-preserving-v1",
	}})
	if err != nil || result.Plaintext != "" || result.Markdown != "" || len(result.AnchorBlocks) != 0 {
		t.Fatalf("Extract(metadata) = %#v, %v", result, err)
	}
}
