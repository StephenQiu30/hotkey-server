package application

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"unicode/utf8"
)

func TestDocumentAnchorMapResultValidatesNFCUTF8BoundariesAndImmutableHash(t *testing.T) {
	t.Parallel()
	plaintext := "中文标题\n\nSecond link."
	markdown := "# 中文标题\n\nSecond [link](https://example.test)."
	result := MapDocumentTextResult{
		Plaintext:               plaintext,
		NormalizationVersion:    CanonicalDocumentTextNormalizationVersion,
		AnchorMapProfileVersion: CanonicalDocumentAnchorMapProfileVersion,
		PlaintextSHA256:         documentAnchorTestSHA(plaintext),
		MarkdownSHA256:          documentAnchorTestSHA(markdown),
		Blocks: []DocumentAnchorBlockDTO{
			{Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: 12, MarkdownUTF8ByteStart: 0, MarkdownUTF8ByteEnd: 14, MarkdownAnchor: "body-0000-45937b7f2844"},
			{Ordinal: 1, PlaintextUTF8ByteStart: 14, PlaintextUTF8ByteEnd: int64(len(plaintext)), MarkdownUTF8ByteStart: 16, MarkdownUTF8ByteEnd: int64(len(markdown)), MarkdownAnchor: "body-0001-abb2e259fe58"},
		},
	}
	result.AnchorMapSHA256 = DocumentAnchorMapSHA256(result)
	if err := ValidateMapDocumentTextResult(MapDocumentTextCommand{Markdown: markdown}, result); err != nil {
		t.Fatalf("ValidateMapDocumentTextResult() error = %v", err)
	}

	brokenBoundary := result
	brokenBoundary.Blocks = append([]DocumentAnchorBlockDTO(nil), result.Blocks...)
	brokenBoundary.Blocks[0].PlaintextUTF8ByteEnd = 1
	brokenBoundary.AnchorMapSHA256 = DocumentAnchorMapSHA256(brokenBoundary)
	if err := ValidateMapDocumentTextResult(MapDocumentTextCommand{Markdown: markdown}, brokenBoundary); err == nil {
		t.Fatal("ValidateMapDocumentTextResult() accepted an offset inside a multibyte rune")
	}

	changedHash := result
	changedHash.AnchorMapSHA256 = documentAnchorTestSHA("different")
	if err := ValidateMapDocumentTextResult(MapDocumentTextCommand{Markdown: markdown}, changedHash); err == nil {
		t.Fatal("ValidateMapDocumentTextResult() accepted a changed map hash")
	}
}

func TestDocumentAnchorMapResultRejectsGapsOverlapAndBodyBearingBlocks(t *testing.T) {
	t.Parallel()
	if _, exposed := any(DocumentAnchorBlockDTO{}).(interface{ Body() string }); exposed {
		t.Fatal("DocumentAnchorBlockDTO exposes body content")
	}
	plaintext := "one\n\ntwo"
	markdown := "one\n\ntwo"
	base := MapDocumentTextResult{
		Plaintext:               plaintext,
		NormalizationVersion:    CanonicalDocumentTextNormalizationVersion,
		AnchorMapProfileVersion: CanonicalDocumentAnchorMapProfileVersion,
		PlaintextSHA256:         documentAnchorTestSHA(plaintext), MarkdownSHA256: documentAnchorTestSHA(markdown),
		Blocks: []DocumentAnchorBlockDTO{
			{Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: 3, MarkdownUTF8ByteStart: 0, MarkdownUTF8ByteEnd: 3, MarkdownAnchor: "body-0000-7692c3ad3540"},
			{Ordinal: 1, PlaintextUTF8ByteStart: 5, PlaintextUTF8ByteEnd: 8, MarkdownUTF8ByteStart: 5, MarkdownUTF8ByteEnd: 8, MarkdownAnchor: "body-0001-3fc4ccfe7458"},
		},
	}
	base.AnchorMapSHA256 = DocumentAnchorMapSHA256(base)
	if err := ValidateMapDocumentTextResult(MapDocumentTextCommand{Markdown: markdown}, base); err != nil {
		t.Fatalf("valid block map rejected: %v", err)
	}
	for name, mutate := range map[string]func(*MapDocumentTextResult){
		"overlap":          func(value *MapDocumentTextResult) { value.Blocks[1].PlaintextUTF8ByteStart = 2 },
		"bad ordinal":      func(value *MapDocumentTextResult) { value.Blocks[1].Ordinal = 3 },
		"duplicate anchor": func(value *MapDocumentTextResult) { value.Blocks[1].MarkdownAnchor = value.Blocks[0].MarkdownAnchor },
		"out of range":     func(value *MapDocumentTextResult) { value.Blocks[1].MarkdownUTF8ByteEnd = int64(len(markdown) + 1) },
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			value.Blocks = append([]DocumentAnchorBlockDTO(nil), base.Blocks...)
			mutate(&value)
			value.AnchorMapSHA256 = DocumentAnchorMapSHA256(value)
			if err := ValidateMapDocumentTextResult(MapDocumentTextCommand{Markdown: markdown}, value); err == nil {
				t.Fatalf("invalid map %s was accepted", name)
			}
		})
	}
}

func TestUTF8DocumentTextRangeRequiresLeftClosedRightOpenRuneBoundaries(t *testing.T) {
	t.Parallel()
	value := "A中B"
	for _, test := range []struct {
		start, end int64
		valid      bool
	}{
		{0, 1, true}, {1, 4, true}, {4, 5, true}, {0, 5, true},
		{1, 2, false}, {2, 4, false}, {-1, 1, false}, {4, 4, false}, {0, 6, false},
	} {
		if got := ValidUTF8DocumentTextRange(value, test.start, test.end); got != test.valid {
			t.Fatalf("ValidUTF8DocumentTextRange(%q,%d,%d) = %t, want %t; utf8=%t", value, test.start, test.end, got, test.valid, utf8.ValidString(value))
		}
	}
}

func documentAnchorTestSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)
}
