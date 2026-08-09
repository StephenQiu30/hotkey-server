package feed

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionmarkdown "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/markdown"
)

func TestBodyExtractorSafelyExtractsRSSContentAsCanonicalPlaintextAndMarkdown(t *testing.T) {
	t.Parallel()

	payload := []byte(`<item xmlns:content="http://purl.org/rss/1.0/modules/content/">
		<guid>one</guid><description>fallback summary</description>
		<content:encoded><![CDATA[
			<h1>Café launch</h1><p>Second &amp; safe <a href="/story">link</a>.</p>
			<table><tr><th>Signal</th><th>Score</th></tr><tr><td>AI</td><td>90</td></tr></table>
			<script>alert(1)</script><img src="https://tracker.example.test/pixel.png">
			<a href="javascript:alert(2)">unsafe</a>
		]]></content:encoded>
	</item>`)
	extractor := NewBodyExtractor(ingestionmarkdown.NewConverter())
	result, err := extractor.Extract(context.Background(), extractionCommand(payload, "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if result.BodyOrigin != ingestionapplication.BodyOriginFeedContent || result.Completeness != ingestionapplication.BodyCompletenessFull ||
		result.Plaintext != "Café launch\n\nSecond & safe link.\n\nSignal Score\n\nAI 90\n\nunsafe" || result.Language != "en" || result.Truncated || result.QualityScore != nil {
		t.Fatalf("Extract() body facts = %#v", result)
	}
	for _, want := range []string{"# Café launch", "[link](https://news.example.test/story)", "| Signal | Score |"} {
		if !strings.Contains(result.Markdown, want) {
			t.Fatalf("Markdown = %q, want %q", result.Markdown, want)
		}
	}
	for _, forbidden := range []string{"<h1", "<script", "alert(", "javascript:", "tracker.example.test", "<img"} {
		if strings.Contains(result.Markdown, forbidden) || strings.Contains(result.Plaintext, forbidden) {
			t.Fatalf("extraction leaked unsafe/raw content %q: %#v", forbidden, result)
		}
	}
	if result.PlaintextSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(result.Plaintext))) ||
		len(result.ExtractorProfileSHA256) != 64 || len(result.PlaintextTransformerProfileSHA256) != 64 ||
		len(result.MarkdownTransformerProfileSHA256) != 64 ||
		strings.ToLower(result.ExtractorProfileSHA256) != result.ExtractorProfileSHA256 ||
		strings.ToLower(result.PlaintextTransformerProfileSHA256) != result.PlaintextTransformerProfileSHA256 ||
		strings.ToLower(result.MarkdownTransformerProfileSHA256) != result.MarkdownTransformerProfileSHA256 ||
		result.PlaintextTransformerProfileSHA256 == result.MarkdownTransformerProfileSHA256 {
		t.Fatalf("extraction profiles/digest = %#v", result)
	}
	if !reflect.DeepEqual(result.QualityWarnings, []string{"captured_markup_sanitized"}) {
		t.Fatalf("quality warnings = %#v", result.QualityWarnings)
	}
}

func TestBodyExtractorSupportsFrozenRSSRDFAndAtomFallbackSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		selector     string
		mimeType     string
		payload      string
		origin       string
		completeness string
		plaintext    string
		warning      string
	}{
		{
			name: "source-marshaled RSS content", selector: "rss2-go-xml-v1", mimeType: "text/xml; charset=utf-8",
			payload: `<rssItem><guid>one</guid><encoded>&lt;p&gt;full RSS body&lt;/p&gt;</encoded><description>summary</description></rssItem>`,
			origin:  ingestionapplication.BodyOriginFeedContent, completeness: ingestionapplication.BodyCompletenessFull, plaintext: "full RSS body",
		},
		{
			name: "RSS description summary", selector: "rss2-go-xml-v1", mimeType: "application/xml",
			payload: `<rssItem><guid>one</guid><encoded> </encoded><description>RSS summary</description></rssItem>`,
			origin:  ingestionapplication.BodyOriginFeedSummary, completeness: ingestionapplication.BodyCompletenessSummary, plaintext: "RSS summary", warning: "feed_summary_only",
		},
		{
			name: "RDF namespaced content", selector: "rss-rdf-go-xml-v1", mimeType: "application/rdf+xml",
			payload: `<rdfItem xmlns:content="http://purl.org/rss/1.0/modules/content/"><content:encoded>&lt;p&gt;RDF full&lt;/p&gt;</content:encoded><description>summary</description></rdfItem>`,
			origin:  ingestionapplication.BodyOriginFeedContent, completeness: ingestionapplication.BodyCompletenessFull, plaintext: "RDF full",
		},
		{
			name: "Atom content", selector: "atom-go-xml-v1", mimeType: "application/atom+xml",
			payload: `<atomEntry><id>one</id><content>&lt;p&gt;Atom full&lt;/p&gt;</content><summary>summary</summary></atomEntry>`,
			origin:  ingestionapplication.BodyOriginFeedContent, completeness: ingestionapplication.BodyCompletenessFull, plaintext: "Atom full",
		},
		{
			name: "Atom XHTML content", selector: "atom-go-xml-v1", mimeType: "application/atom+xml",
			payload: `<entry xmlns="http://www.w3.org/2005/Atom"><id>one</id><content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>Nested <strong>Atom</strong></p></div></content></entry>`,
			origin:  ingestionapplication.BodyOriginFeedContent, completeness: ingestionapplication.BodyCompletenessFull, plaintext: "Nested Atom",
		},
		{
			name: "Atom summary", selector: "atom-go-xml-v1", mimeType: "application/xml",
			payload: `<entry xmlns="http://www.w3.org/2005/Atom"><id>one</id><content> </content><summary>Atom summary</summary></entry>`,
			origin:  ingestionapplication.BodyOriginFeedSummary, completeness: ingestionapplication.BodyCompletenessSummary, plaintext: "Atom summary", warning: "feed_summary_only",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extractor := NewBodyExtractor(ingestionmarkdown.NewConverter())
			result, err := extractor.Extract(context.Background(), extractionCommand([]byte(test.payload), test.selector, test.mimeType, test.origin, test.completeness))
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			if result.BodyOrigin != test.origin || result.Completeness != test.completeness || result.Plaintext != test.plaintext || result.Markdown == "" {
				t.Fatalf("Extract() = %#v", result)
			}
			if test.warning != "" && !containsWarning(result.QualityWarnings, test.warning) {
				t.Fatalf("quality warnings = %#v, want %q", result.QualityWarnings, test.warning)
			}
		})
	}
}

func TestBodyExtractorReturnsExplicitMetadataOnlyWithoutMarkdownConversion(t *testing.T) {
	t.Parallel()

	payload := []byte(`<atomEntry><id>one</id><content> </content><summary> </summary></atomEntry>`)
	projector := &markdownProjectorSpy{}
	extractor := NewBodyExtractor(projector)
	command := extractionCommand(payload, "atom-go-xml-v1", "application/atom+xml", ingestionapplication.BodyOriginFeedSummary, ingestionapplication.BodyCompletenessMetadataOnly)
	command.Evidence.SourceRecordURL = ""
	command.Evidence.CanonicalURL = ""
	result, err := extractor.Extract(context.Background(), command)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if result.Completeness != ingestionapplication.BodyCompletenessMetadataOnly || result.BodyOrigin != ingestionapplication.BodyOriginFeedSummary ||
		result.Plaintext != "" || result.Markdown != "" || projector.calls != 0 || !reflect.DeepEqual(result.QualityWarnings, []string{"metadata_only"}) {
		t.Fatalf("metadata-only extraction = %#v, projector calls = %d", result, projector.calls)
	}
	if result.PlaintextSHA256 != "" {
		t.Fatalf("metadata-only extraction fabricated plaintext SHA = %s", result.PlaintextSHA256)
	}
	if len(result.PlaintextTransformerProfileSHA256) != 64 || len(result.MarkdownTransformerProfileSHA256) != 64 {
		t.Fatalf("metadata-only extractor capability profiles = %#v", result)
	}
}

func TestBodyExtractorFailsClosedOnUntrustedOrUnboundedSelectedEvidence(t *testing.T) {
	t.Parallel()

	valid := []byte(`<rssItem><encoded>body</encoded></rssItem>`)
	deeplyNested := []byte(`<rssItem><encoded>` + strings.Repeat(`<x>`, maximumSelectedXMLDepth) + `body` + strings.Repeat(`</x>`, maximumSelectedXMLDepth) + `</encoded></rssItem>`)
	tests := []struct {
		name    string
		command ingestionapplication.ExtractSelectedSourceBodyCommand
	}{
		{name: "incorrect selected hash", command: func() ingestionapplication.ExtractSelectedSourceBodyCommand {
			command := extractionCommand(valid, "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)
			command.Evidence.SelectedPayloadSHA256 = strings.Repeat("0", 64)
			return command
		}()},
		{name: "unsupported selector", command: extractionCommand(valid, "rss-unknown-v2", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
		{name: "selector root mismatch", command: extractionCommand([]byte(`<atomEntry><content>body</content></atomEntry>`), "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
		{name: "unsupported MIME", command: extractionCommand(valid, "rss2-go-xml-v1", "text/html", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
		{name: "multiple roots", command: extractionCommand([]byte(`<rssItem><encoded>one</encoded></rssItem><rssItem><encoded>two</encoded></rssItem>`), "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
		{name: "XML directive", command: extractionCommand([]byte(`<!DOCTYPE rssItem [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><rssItem><encoded>&xxe;</encoded></rssItem>`), "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
		{name: "excessive XML depth", command: extractionCommand(deeplyNested, "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
		{name: "invalid UTF-8", command: extractionCommand([]byte{'<', 'r', 's', 's', 'I', 't', 'e', 'm', '>', 0xff, '<', '/', 'r', 's', 's', 'I', 't', 'e', 'm', '>'}, "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
		{name: "oversized payload", command: extractionCommand([]byte(strings.Repeat("x", maximumSelectedSourceEvidenceBytes+1)), "rss2-go-xml-v1", "application/rss+xml", ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extractor := NewBodyExtractor(ingestionmarkdown.NewConverter())
			if _, err := extractor.Extract(context.Background(), test.command); err == nil {
				t.Fatal("Extract() accepted untrusted selected evidence")
			}
		})
	}
}

func TestBodyExtractorDoesNotExposeSelectedBytesOrRawHTMLRecords(t *testing.T) {
	t.Parallel()

	resultType := reflect.TypeOf(ingestionapplication.ExtractSelectedSourceBodyResult{})
	for _, forbidden := range []string{"SelectedPayload", "RawHTML", "RawPayload"} {
		if _, exposed := resultType.FieldByName(forbidden); exposed {
			t.Fatalf("ExtractSelectedSourceBodyResult exposes %s", forbidden)
		}
	}
}

func extractionCommand(payload []byte, selector, mimeType, origin, completeness string) ingestionapplication.ExtractSelectedSourceBodyCommand {
	return ingestionapplication.ExtractSelectedSourceBodyCommand{Evidence: ingestionapplication.SelectedSourceEvidenceDTO{
		EvidenceReferenceID: 71, SourceObservationID: 41, EvidenceSnapshotID: 51, SourceConnectionID: 7,
		ExternalWorkID: "article-41", SourceCode: "rss", ContentType: "article", Title: "Launch", Language: "en",
		SourceRecordURL: "https://feed.example.test/rss.xml", CanonicalURL: "https://news.example.test/base/article",
		BodyOrigin: origin, Completeness: completeness, CapturedAt: time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC),
		SelectedPayload: append([]byte(nil), payload...), SelectedPayloadSHA256: fmt.Sprintf("%x", sha256.Sum256(payload)),
		PayloadMIMEType: mimeType, SelectorVersion: selector,
	}}
}

func containsWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

type markdownProjectorSpy struct {
	calls int
}

func (projector *markdownProjectorSpy) Convert(input, _ string) (string, error) {
	projector.calls++
	if !utf8.ValidString(input) {
		return "", fmt.Errorf("invalid input")
	}
	return strings.TrimSpace(input), nil
}
