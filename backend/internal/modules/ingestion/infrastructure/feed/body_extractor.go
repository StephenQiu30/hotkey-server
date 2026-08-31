// Package feed extracts bounded body facts from already-captured RSS, RDF,
// and Atom evidence. It never performs network I/O.
package feed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

const (
	maximumSelectedSourceEvidenceBytes = ingestionapplication.MaximumSelectedSourceEvidenceBytes

	rssItemSelectorVersion   = "rss2-go-xml-v1"
	rdfItemSelectorVersion   = "rss-rdf-go-xml-v1"
	atomEntrySelectorVersion = "atom-go-xml-v1"

	feedBodyExtractorVersion        = "feed-body-extractor-v1"
	feedBodyExtractorProfileVersion = "rss-atom-rdf-body-v1"
	feedBodyExtractorProfile        = "rss2:encoded>description|rdf:encoded>description|atom:content>summary|strict-xml|utf8|nfc|4mib-v1"
	plaintextTransformerProfile     = "captured-html-to-plaintext|html-parser|drop-script-style-noscript-template-svg|block-boundaries|collapse-space|lf|trim|nfc|4mib-v1"
	markdownTransformerProfile      = "html-to-markdown-v2.5.2|commonmark|gfm-table|http-https-mailto-links|relative-safe-base|no-remote-images|nfc|4mib-v1"
	maximumSelectedXMLDepth         = 64
	maximumSelectedXMLElements      = 50000
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
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("feed body extractor is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
	}
	evidence := command.Evidence
	if err := validateSelectedFeedEvidence(evidence); err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
	}

	rawBody, origin, completeness, err := selectedFeedBody(evidence.SelectedPayload, evidence.SelectorVersion)
	if err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
	}
	if origin != evidence.BodyOrigin || completeness != evidence.Completeness {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("selected feed body semantics do not match verified observation")
	}
	result := ingestionapplication.ExtractSelectedSourceBodyResult{
		BodyOrigin: origin, Completeness: completeness, Language: normalizedLanguage(evidence.Language),
		ExtractorVersion: feedBodyExtractorVersion, ExtractorProfileVersion: feedBodyExtractorProfileVersion,
		ExtractorProfileSHA256:            profileSHA256(feedBodyExtractorProfile),
		PlaintextTransformerProfileSHA256: profileSHA256(plaintextTransformerProfile),
		MarkdownTransformerProfileSHA256:  profileSHA256(markdownTransformerProfile),
	}
	if completeness == ingestionapplication.BodyCompletenessMetadataOnly {
		result.QualityWarnings = []string{"metadata_only"}
		return result, nil
	}

	baseURL, err := markdownBaseURL(evidence)
	if err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
	}
	projection, err := extractor.markdown.Convert(rawBody, baseURL)
	if err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, fmt.Errorf("convert captured feed field to Markdown: %w", err)
	}
	projection = canonicalText(projection)
	if projection == "" || !utf8.ValidString(projection) || len(projection) > ingestionapplication.MaximumMarkdownProjectionBytes {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("feed Markdown projection is invalid or exceeds the size limit")
	}
	anchored, err := extractor.anchors.MapDocumentText(ctx, ingestionapplication.MapDocumentTextCommand{Markdown: projection})
	if err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, fmt.Errorf("map canonical Markdown body: %w", err)
	}
	plaintext, err := canonicalPlaintextFromCapturedHTML(rawBody)
	if err != nil {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, err
	}
	if plaintext != anchored.Plaintext {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("plaintext and Markdown visible text do not align")
	}
	if plaintext == "" || len(plaintext) > ingestionapplication.MaximumCanonicalSourceBodyBytes {
		return ingestionapplication.ExtractSelectedSourceBodyResult{}, errors.New("feed plaintext projection is empty or exceeds the size limit")
	}
	result.Plaintext = plaintext
	result.Markdown = projection
	result.PlaintextSHA256 = profileSHA256(plaintext)
	result.MarkdownSHA256 = anchored.MarkdownSHA256
	result.TextNormalizationVersion = anchored.NormalizationVersion
	result.AnchorMapProfileVersion = anchored.AnchorMapProfileVersion
	result.AnchorMapSHA256 = anchored.AnchorMapSHA256
	result.AnchorBlocks = append([]ingestionapplication.DocumentAnchorBlockDTO(nil), anchored.Blocks...)
	if looksLikeMarkup(rawBody) {
		result.QualityWarnings = append(result.QualityWarnings, "captured_markup_sanitized")
	}
	if completeness == ingestionapplication.BodyCompletenessSummary {
		result.QualityWarnings = append(result.QualityWarnings, "feed_summary_only")
	}
	sort.Strings(result.QualityWarnings)
	return result, nil
}

type rssSelectedItemRecord struct {
	XMLName     xml.Name
	Description selectedFeedFieldRecord `xml:"description"`
	Encoded     selectedFeedFieldRecord `xml:"encoded"`
}

type rdfSelectedItemRecord struct {
	XMLName     xml.Name
	Description selectedFeedFieldRecord `xml:"description"`
	Encoded     selectedFeedFieldRecord `xml:"encoded"`
}

type atomSelectedEntryRecord struct {
	XMLName xml.Name
	Content selectedFeedFieldRecord `xml:"content"`
	Summary selectedFeedFieldRecord `xml:"summary"`
}

// selectedFeedFieldRecord is a parser-only record. Escaped HTML fields become
// decoded text, while Atom XHTML child elements remain a captured fragment for
// the sanitizer and Markdown converter; neither form leaves Infrastructure as
// raw HTML.
type selectedFeedFieldRecord struct {
	InnerXML string `xml:",innerxml"`
}

func selectedFeedBody(payload []byte, selectorVersion string) (string, string, string, error) {
	root, err := validateSingleXMLDocument(payload)
	if err != nil {
		return "", "", "", err
	}
	switch selectorVersion {
	case rssItemSelectorVersion:
		if root.Local != "rssItem" && root.Local != "item" {
			return "", "", "", errors.New("RSS selected evidence root is invalid")
		}
		var record rssSelectedItemRecord
		if err := xml.Unmarshal(payload, &record); err != nil {
			return "", "", "", errors.New("RSS selected evidence could not be decoded")
		}
		return preferredSelectedFeedFields(record.Encoded, record.Description)
	case rdfItemSelectorVersion:
		if root.Local != "rdfItem" && root.Local != "item" {
			return "", "", "", errors.New("RDF selected evidence root is invalid")
		}
		var record rdfSelectedItemRecord
		if err := xml.Unmarshal(payload, &record); err != nil {
			return "", "", "", errors.New("RDF selected evidence could not be decoded")
		}
		return preferredSelectedFeedFields(record.Encoded, record.Description)
	case atomEntrySelectorVersion:
		if root.Local != "atomEntry" && root.Local != "entry" {
			return "", "", "", errors.New("atom selected evidence root is invalid")
		}
		var record atomSelectedEntryRecord
		if err := xml.Unmarshal(payload, &record); err != nil {
			return "", "", "", errors.New("atom selected evidence could not be decoded")
		}
		return preferredSelectedFeedFields(record.Content, record.Summary)
	default:
		return "", "", "", errors.New("selected feed selector version is unsupported")
	}
}

func preferredSelectedFeedFields(content, summary selectedFeedFieldRecord) (string, string, string, error) {
	contentValue, err := content.capturedValue()
	if err != nil {
		return "", "", "", err
	}
	summaryValue, err := summary.capturedValue()
	if err != nil {
		return "", "", "", err
	}
	return preferredFeedField(contentValue, summaryValue)
}

func (field selectedFeedFieldRecord) capturedValue() (string, error) {
	if strings.TrimSpace(field.InnerXML) == "" {
		return "", nil
	}
	hasElement, err := capturedFieldHasElement(field.InnerXML)
	if err != nil {
		return "", errors.New("selected feed body field is malformed")
	}
	if hasElement {
		return field.InnerXML, nil
	}
	var decoded struct {
		Value string `xml:",chardata"`
	}
	if err := xml.Unmarshal([]byte("<captured>"+field.InnerXML+"</captured>"), &decoded); err != nil {
		return "", errors.New("selected feed body text could not be decoded")
	}
	return decoded.Value, nil
}

func capturedFieldHasElement(innerXML string) (bool, error) {
	decoder := xml.NewDecoder(strings.NewReader("<captured>" + innerXML + "</captured>"))
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		switch token.(type) {
		case xml.StartElement:
			depth++
			if depth > 1 {
				return true, nil
			}
		case xml.EndElement:
			depth--
		}
	}
}

func preferredFeedField(content, summary string) (string, string, string, error) {
	if strings.TrimSpace(content) != "" {
		return content, ingestionapplication.BodyOriginFeedContent, ingestionapplication.BodyCompletenessFull, nil
	}
	if strings.TrimSpace(summary) != "" {
		return summary, ingestionapplication.BodyOriginFeedSummary, ingestionapplication.BodyCompletenessSummary, nil
	}
	return "", ingestionapplication.BodyOriginFeedSummary, ingestionapplication.BodyCompletenessMetadataOnly, nil
}

func validateSelectedFeedEvidence(evidence ingestionapplication.SelectedSourceEvidenceDTO) error {
	if evidence.SourceCode != "rss" || len(evidence.SelectedPayload) == 0 || len(evidence.SelectedPayload) > maximumSelectedSourceEvidenceBytes ||
		!utf8.Valid(evidence.SelectedPayload) || evidence.SelectedPayloadSHA256 == "" || evidence.SelectorVersion == "" {
		return errors.New("selected feed evidence is invalid or exceeds the size limit")
	}
	mediaType, _, err := mime.ParseMediaType(evidence.PayloadMIMEType)
	if err != nil || !allowedFeedXMLMediaType(strings.ToLower(mediaType)) {
		return errors.New("selected feed evidence MIME type is unsupported")
	}
	declared, err := hex.DecodeString(evidence.SelectedPayloadSHA256)
	digest := sha256.Sum256(evidence.SelectedPayload)
	if err != nil || len(declared) != sha256.Size || subtle.ConstantTimeCompare(declared, digest[:]) != 1 ||
		strings.ToLower(evidence.SelectedPayloadSHA256) != evidence.SelectedPayloadSHA256 {
		return errors.New("selected feed evidence digest does not match captured bytes")
	}
	return nil
}

func allowedFeedXMLMediaType(mediaType string) bool {
	switch mediaType {
	case "application/xml", "text/xml", "application/rss+xml", "application/atom+xml", "application/rdf+xml":
		return true
	default:
		return false
	}
}

func validateSingleXMLDocument(payload []byte) (xml.Name, error) {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	decoder.Strict = true
	depth := 0
	rootCount := 0
	elementCount := 0
	var root xml.Name
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return xml.Name{}, errors.New("selected feed XML is malformed")
		}
		switch value := token.(type) {
		case xml.Directive:
			return xml.Name{}, errors.New("selected feed XML directives are forbidden")
		case xml.StartElement:
			elementCount++
			if depth >= maximumSelectedXMLDepth || elementCount > maximumSelectedXMLElements {
				return xml.Name{}, errors.New("selected feed XML exceeds structural limits")
			}
			if depth == 0 {
				rootCount++
				root = value.Name
			}
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				return xml.Name{}, errors.New("selected feed XML is malformed")
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(value)) != "" {
				return xml.Name{}, errors.New("selected feed XML contains trailing data")
			}
		}
	}
	if rootCount != 1 || depth != 0 || root.Local == "" {
		return xml.Name{}, errors.New("selected feed XML must contain exactly one item or entry")
	}
	return root, nil
}

func markdownBaseURL(evidence ingestionapplication.SelectedSourceEvidenceDTO) (string, error) {
	for _, candidate := range []string{evidence.CanonicalURL, evidence.SourceRecordURL, evidence.DiscussionURL} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err == nil && parsed.User == nil && parsed.Host != "" &&
			(strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
			return parsed.String(), nil
		}
	}
	return "", errors.New("selected feed body has no safe HTTP(S) base URL")
}

func canonicalPlaintextFromCapturedHTML(value string) (string, error) {
	document, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return "", errors.New("captured feed field could not be parsed for plaintext")
	}
	var builder strings.Builder
	appendSafeText(&builder, document)
	lines := strings.Split(strings.ReplaceAll(builder.String(), "\u00a0", " "), "\n")
	normalizedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		normalizedLines = append(normalizedLines, line)
	}
	return norm.NFC.String(strings.Join(normalizedLines, "\n\n")), nil
}

func appendSafeText(builder *strings.Builder, node *html.Node) {
	if node.Type == html.ElementNode {
		name := strings.ToLower(node.Data)
		if forbiddenBodyElement(name) {
			return
		}
		if name == "br" {
			builder.WriteByte('\n')
			return
		}
		if blockBodyElement(name) {
			builder.WriteByte('\n')
		}
	}
	if node.Type == html.TextNode {
		builder.WriteString(node.Data)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendSafeText(builder, child)
	}
	if node.Type == html.ElementNode {
		name := strings.ToLower(node.Data)
		if name == "td" || name == "th" {
			builder.WriteByte(' ')
		}
		if blockBodyElement(name) {
			builder.WriteByte('\n')
		}
	}
}

func forbiddenBodyElement(name string) bool {
	switch name {
	case "script", "style", "iframe", "form", "img", "picture", "svg", "object", "embed", "noscript", "template":
		return true
	default:
		return false
	}
}

func blockBodyElement(name string) bool {
	switch name {
	case "address", "article", "aside", "blockquote", "div", "dl", "fieldset", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "tr", "ul":
		return true
	default:
		return false
	}
}

func looksLikeMarkup(value string) bool {
	tokenizer := html.NewTokenizer(strings.NewReader(value))
	for {
		switch tokenizer.Next() {
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken, html.DoctypeToken:
			return true
		case html.ErrorToken:
			return false
		}
	}
}

func canonicalText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return norm.NFC.String(strings.TrimSpace(value))
}

func normalizedLanguage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "und"
	}
	return value
}

func profileSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)
}
