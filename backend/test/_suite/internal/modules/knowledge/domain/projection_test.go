package domain

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestImmutableProjectionValidationAndPath(t *testing.T) {
	content := []byte("# archived document\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	profile := strings.Repeat("a", 64)
	projection := ImmutableProjection{
		DocumentID: 7, DocumentVersionID: 19, Format: ProjectionMarkdown,
		TransformerProfileSHA256: profile, Content: content, SHA256: digest,
	}
	if err := projection.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if got, want := projection.RelativePath(), "documents/7/19/markdown/"+profile+".md"; got != want {
		t.Fatalf("RelativePath() = %q, want %q", got, want)
	}
	if got, want := projection.MIMEType(), "text/markdown; charset=utf-8"; got != want {
		t.Fatalf("MIMEType() = %q, want %q", got, want)
	}
}

func TestImmutableProjectionRejectsInvalidOrMismatchedInput(t *testing.T) {
	content := []byte("document")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	profile := strings.Repeat("a", 64)
	tests := []struct {
		name       string
		projection ImmutableProjection
	}{
		{"missing_document", ImmutableProjection{DocumentVersionID: 1, Format: ProjectionMarkdown, TransformerProfileSHA256: profile, Content: content, SHA256: digest}},
		{"missing_version", ImmutableProjection{DocumentID: 1, Format: ProjectionMarkdown, TransformerProfileSHA256: profile, Content: content, SHA256: digest}},
		{"unsupported_format", ImmutableProjection{DocumentID: 1, DocumentVersionID: 1, Format: "html", TransformerProfileSHA256: profile, Content: content, SHA256: digest}},
		{"missing_transformer_profile", ImmutableProjection{DocumentID: 1, DocumentVersionID: 1, Format: ProjectionPlaintext, Content: content, SHA256: digest}},
		{"uppercase_transformer_profile", ImmutableProjection{DocumentID: 1, DocumentVersionID: 1, Format: ProjectionPlaintext, TransformerProfileSHA256: strings.ToUpper(profile), Content: content, SHA256: digest}},
		{"empty_content", ImmutableProjection{DocumentID: 1, DocumentVersionID: 1, Format: ProjectionPlaintext, TransformerProfileSHA256: profile, SHA256: fmt.Sprintf("%x", sha256.Sum256(nil))}},
		{"uppercase_digest", ImmutableProjection{DocumentID: 1, DocumentVersionID: 1, Format: ProjectionPlaintext, TransformerProfileSHA256: profile, Content: content, SHA256: strings.ToUpper(digest)}},
		{"mismatched_digest", ImmutableProjection{DocumentID: 1, DocumentVersionID: 1, Format: ProjectionPlaintext, TransformerProfileSHA256: profile, Content: content, SHA256: fmt.Sprintf("%064d", 0)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.projection.Validate(); err == nil {
				t.Fatalf("Validate() accepted %#v", test.projection)
			}
		})
	}
}
