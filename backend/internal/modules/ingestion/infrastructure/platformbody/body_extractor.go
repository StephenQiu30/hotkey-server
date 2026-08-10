// Package platformbody extracts canonical body projections from exact JSON
// evidence selected by official API connectors. It performs no network I/O
// and never treats generated search synthesis as citable source text.
package platformbody

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"unicode/utf8"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	"golang.org/x/text/unicode/norm"
)

const (
	extractorVersion        = "official-platform-json-body-extractor-v1"
	extractorProfileVersion = "official-api-json-body-v1"
	extractorProfile        = "x:text|weibo:text_raw>text|hn:text-html|bilibili:desc-summary|google:successful-snippet|json-exact|utf8|nfc|4mib-v1"
	plaintextProfile        = "safe-markdown-visible-text|lf|trim|nfc|4mib-v1"
	markdownProfile         = "provider-plain-or-html-to-markdown|sanitize|links-http-https|no-remote-images|nfc|4mib-v1"
)

type BodyExtractor struct {
	markdown ingestiondomain.MarkdownProjector
	anchors  ingestionapplication.DocumentTextAnchorMapper
}

var _ ingestionapplication.SelectedSourceBodyExtractor = (*BodyExtractor)(nil)

func NewBodyExtractor(markdown ingestiondomain.MarkdownProjector, anchors ingestionapplication.DocumentTextAnchorMapper) *BodyExtractor {
	return &BodyExtractor{markdown: markdown, anchors: anchors}
}

func (extractor *BodyExtractor) Extract(ctx context.Context, command ingestionapplication.ExtractSelectedSourceBodyCommand) (ingestionapplication.ExtractSelectedSourceBodyResult, error) {
	if extractor == nil || extractor.markdown == nil || extractor.anchors == nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("platform body extractor is not initialized")
	}
	evidence := command.Evidence
	if err := validateEvidence(evidence); err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
	}
	body, markup, expectedOrigin, expectedCompleteness, err := selectedBody(evidence)
	if err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
	}
	if evidence.BodyOrigin != expectedOrigin || evidence.Completeness != expectedCompleteness {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("selected platform body semantics do not match observation")
	}
	result := ingestionapplication.ExtractSelectedSourceBodyResult{
		BodyOrigin: expectedOrigin, Completeness: expectedCompleteness, Language: strings.TrimSpace(evidence.Language),
		ExtractorVersion: extractorVersion, ExtractorProfileVersion: extractorProfileVersion,
		ExtractorProfileSHA256:            digest(extractorProfile),
		PlaintextTransformerProfileSHA256: digest(plaintextProfile),
		MarkdownTransformerProfileSHA256:  digest(markdownProfile),
	}
	if expectedCompleteness == ingestionapplication.BodyCompletenessMetadataOnly {
		result.QualityWarnings = []string{"metadata_only"}
		return result, nil
	}
	body = norm.NFC.String(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")))
	if body == "" || !utf8.ValidString(body) || len(body) > ingestionapplication.MaximumSelectedSourceEvidenceBytes {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("selected platform body is empty or invalid")
	}
	markdown := plainTextMarkdown(body)
	if markup {
		baseURL, err := sourceBaseURL(evidence)
		if err != nil {
			return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
		}
		markdown, err = extractor.markdown.Convert(body, baseURL)
		if err != nil {
			return ingestionapplication.ExtractSelectedSourceBodyResult{}, fmt.Errorf("convert selected platform body to Markdown: %w", err)
		}
	}
	markdown = norm.NFC.String(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(markdown, "\r\n", "\n"), "\r", "\n")))
	anchored, err := extractor.anchors.MapDocumentText(ctx, ingestionapplication.MapDocumentTextCommand{Markdown: markdown})
	if err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, fmt.Errorf("map selected platform body anchors: %w", err)
	}
	if anchored.Plaintext == "" || len(anchored.Plaintext) > ingestionapplication.MaximumCanonicalSourceBodyBytes ||
		len(markdown) > ingestionapplication.MaximumMarkdownProjectionBytes {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("selected platform projection exceeds the body limit")
	}
	result.Plaintext = anchored.Plaintext
	result.Markdown = markdown
	result.PlaintextSHA256 = digest(anchored.Plaintext)
	result.MarkdownSHA256 = anchored.MarkdownSHA256
	result.TextNormalizationVersion = anchored.NormalizationVersion
	result.AnchorMapProfileVersion = anchored.AnchorMapProfileVersion
	result.AnchorMapSHA256 = anchored.AnchorMapSHA256
	result.AnchorBlocks = append([]ingestionapplication.DocumentAnchorBlockDTO(nil), anchored.Blocks...)
	if markup {
		result.QualityWarnings = []string{"captured_markup_sanitized"}
	}
	if expectedCompleteness == ingestionapplication.BodyCompletenessSummary || expectedCompleteness == ingestionapplication.BodyCompletenessSnippet {
		result.QualityWarnings = append(result.QualityWarnings, "source_excerpt_only")
	}
	return result, nil
}

func validateEvidence(evidence ingestionapplication.SelectedSourceEvidenceDTO) error {
	if len(evidence.SelectedPayload) == 0 || len(evidence.SelectedPayload) > ingestionapplication.MaximumSelectedSourceEvidenceBytes ||
		!utf8.Valid(evidence.SelectedPayload) || evidence.SelectedPayloadSHA256 == "" {
		return errors.New("selected platform evidence is invalid")
	}
	mediaType, _, err := mime.ParseMediaType(evidence.PayloadMIMEType)
	allowedMIME := strings.ToLower(mediaType) == "application/json" || evidence.SourceCode == "bing_grounding" && strings.ToLower(mediaType) == "text/event-stream"
	if err != nil || !allowedMIME {
		return errors.New("selected platform evidence MIME type is unsupported")
	}
	declared, err := hex.DecodeString(evidence.SelectedPayloadSHA256)
	digestValue := sha256.Sum256(evidence.SelectedPayload)
	if err != nil || len(declared) != sha256.Size || subtle.ConstantTimeCompare(declared, digestValue[:]) != 1 ||
		strings.ToLower(evidence.SelectedPayloadSHA256) != evidence.SelectedPayloadSHA256 {
		return errors.New("selected platform evidence digest is invalid")
	}
	return nil
}

func selectedBody(evidence ingestionapplication.SelectedSourceEvidenceDTO) (string, bool, string, string, error) {
	var record map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(evidence.SelectedPayload)))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return "", false, "", "", errors.New("selected platform JSON is invalid")
	}
	switch evidence.SourceCode {
	case "x":
		return bodySemantics(stringField(record, "text"), false, ingestionapplication.BodyOriginPlatformPost, ingestionapplication.BodyCompletenessFull)
	case "weibo":
		if value := stringField(record, "text_raw"); strings.TrimSpace(value) != "" {
			return bodySemantics(value, false, ingestionapplication.BodyOriginPlatformPost, ingestionapplication.BodyCompletenessFull)
		}
		return bodySemantics(stringField(record, "text"), true, ingestionapplication.BodyOriginPlatformPost, ingestionapplication.BodyCompletenessFull)
	case "hacker_news":
		return bodySemantics(stringField(record, "text"), true, ingestionapplication.BodyOriginPlatformPost, ingestionapplication.BodyCompletenessFull)
	case "bilibili":
		field := "desc"
		if evidence.ContentType == "article" {
			field = "summary"
		}
		return bodySemantics(stringField(record, field), false, ingestionapplication.BodyOriginPlatformPost, ingestionapplication.BodyCompletenessSummary)
	case "google_agent_search":
		return googleSnippetSemantics(record)
	case "bing_grounding":
		// Foundry Web Search returns a model-synthesized answer. The raw RPC
		// response is retained for audit, but generated text and citations are
		// not promoted into citable body evidence.
		return "", false, ingestionapplication.BodyOriginSearchSnippet, ingestionapplication.BodyCompletenessMetadataOnly, nil
	default:
		return "", false, "", "", errors.New("selected platform source is unsupported")
	}
}

func bodySemantics(body string, markup bool, origin, presentCompleteness string) (string, bool, string, string, error) {
	if strings.TrimSpace(body) == "" {
		return "", markup, origin, ingestionapplication.BodyCompletenessMetadataOnly, nil
	}
	return body, markup, origin, presentCompleteness, nil
}

func googleSnippetSemantics(record map[string]any) (string, bool, string, string, error) {
	document, _ := record["document"].(map[string]any)
	derived, _ := document["derivedStructData"].(map[string]any)
	snippets, _ := derived["snippets"].([]any)
	for _, value := range snippets {
		snippet, _ := value.(map[string]any)
		status := strings.ToUpper(strings.TrimSpace(firstStringField(snippet, "snippetStatus", "snippet_status")))
		if status != "" && status != "SUCCESS" {
			continue
		}
		if body := stringField(snippet, "snippet"); strings.TrimSpace(body) != "" {
			return body, true, ingestionapplication.BodyOriginSearchSnippet, ingestionapplication.BodyCompletenessSnippet, nil
		}
	}
	return "", false, ingestionapplication.BodyOriginSearchSnippet, ingestionapplication.BodyCompletenessMetadataOnly, nil
}

func stringField(record map[string]any, field string) string {
	value, _ := record[field].(string)
	return value
}

func firstStringField(record map[string]any, fields ...string) string {
	for _, field := range fields {
		if value := stringField(record, field); value != "" {
			return value
		}
	}
	return ""
}

func plainTextMarkdown(value string) string {
	lines := strings.Split(value, "\n")
	paragraphs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			paragraphs = append(paragraphs, markdownLiteral(line))
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

func markdownLiteral(value string) string {
	var result strings.Builder
	for _, character := range value {
		switch character {
		case '&':
			result.WriteString("&amp;")
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		case '\\', '`', '*', '_', '[', ']', '#', '!':
			_, _ = fmt.Fprintf(&result, "&#%d;", character)
		default:
			result.WriteRune(character)
		}
	}
	return result.String()
}

func sourceBaseURL(evidence ingestionapplication.SelectedSourceEvidenceDTO) (string, error) {
	for _, raw := range []string{evidence.CanonicalURL, evidence.SourceRecordURL} {
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err == nil && parsed.IsAbs() && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" {
			return parsed.String(), nil
		}
	}
	return "https://invalid.local/", nil
}

func digest(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
