package markdown

import (
	"context"
	"errors"
	"html"
	"strings"
	"unicode/utf8"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	goldmarkutil "github.com/yuin/goldmark/util"
	"golang.org/x/text/unicode/norm"
)

type AnchorMapper struct {
	parser goldmark.Markdown
}

var _ ingestionapplication.DocumentTextAnchorMapper = (*AnchorMapper)(nil)

func NewAnchorMapper() *AnchorMapper {
	return &AnchorMapper{parser: goldmark.New(goldmark.WithExtensions(extension.GFM))}
}

func (mapper *AnchorMapper) MapDocumentText(ctx context.Context, command ingestionapplication.MapDocumentTextCommand) (ingestionapplication.MapDocumentTextResult, error) {
	if mapper == nil || mapper.parser == nil {
		return ingestionapplication.MapDocumentTextResult{}, errors.New("document anchor mapper is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return ingestionapplication.MapDocumentTextResult{}, err
	}
	if err := ingestionapplication.ValidateMapDocumentTextCommand(command); err != nil {
		return ingestionapplication.MapDocumentTextResult{}, err
	}
	source := []byte(command.Markdown)
	document := mapper.parser.Parser().Parse(text.NewReader(source))
	if containsUnsafeMarkdownNode(document) {
		return ingestionapplication.MapDocumentTextResult{}, errors.New("document anchor mapping rejects raw HTML")
	}
	blocks := make([]markdownVisibleBlock, 0, 32)
	if err := appendMarkdownVisibleBlocks(document, source, &blocks); err != nil {
		return ingestionapplication.MapDocumentTextResult{}, err
	}
	if len(blocks) == 0 {
		return ingestionapplication.MapDocumentTextResult{}, errors.New("document anchor mapping found no visible body")
	}

	result := ingestionapplication.MapDocumentTextResult{
		NormalizationVersion:    ingestionapplication.CanonicalDocumentTextNormalizationVersion,
		AnchorMapProfileVersion: ingestionapplication.CanonicalDocumentAnchorMapProfileVersion,
		MarkdownSHA256:          documentAnchorDigest(command.Markdown),
		Blocks:                  make([]ingestionapplication.DocumentAnchorBlockDTO, 0, len(blocks)),
	}
	var plaintext strings.Builder
	for ordinal, block := range blocks {
		if ordinal > 0 {
			plaintext.WriteString("\n\n")
		}
		start := int64(plaintext.Len())
		plaintext.WriteString(block.text)
		end := int64(plaintext.Len())
		result.Blocks = append(result.Blocks, ingestionapplication.DocumentAnchorBlockDTO{
			Ordinal: ordinal, PlaintextUTF8ByteStart: start, PlaintextUTF8ByteEnd: end,
			MarkdownUTF8ByteStart: int64(block.sourceStart), MarkdownUTF8ByteEnd: int64(block.sourceEnd),
			MarkdownAnchor: ingestionapplication.DocumentMarkdownAnchor(ordinal, block.text),
		})
	}
	result.Plaintext = plaintext.String()
	result.PlaintextSHA256 = documentAnchorDigest(result.Plaintext)
	result.AnchorMapSHA256 = ingestionapplication.DocumentAnchorMapSHA256(result)
	if err := ingestionapplication.ValidateMapDocumentTextResult(command, result); err != nil {
		return ingestionapplication.MapDocumentTextResult{}, err
	}
	return result, nil
}

type markdownVisibleBlock struct {
	text                   string
	sourceStart, sourceEnd int
}

func appendMarkdownVisibleBlocks(node ast.Node, source []byte, blocks *[]markdownVisibleBlock) error {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindHeading, ast.KindParagraph, ast.KindCodeBlock, ast.KindFencedCodeBlock:
			if err := appendMarkdownVisibleBlock(child, source, blocks, false); err != nil {
				return err
			}
		case ast.KindList:
			if err := appendMarkdownListBlocks(child, source, blocks); err != nil {
				return err
			}
		case ast.KindBlockquote, extensionast.KindTable:
			if err := appendMarkdownVisibleBlocks(child, source, blocks); err != nil {
				return err
			}
		case extensionast.KindTableHeader, extensionast.KindTableRow:
			if err := appendMarkdownVisibleBlock(child, source, blocks, true); err != nil {
				return err
			}
		case ast.KindThematicBreak:
			continue
		default:
			if child.Type() == ast.TypeBlock {
				if err := appendMarkdownVisibleBlocks(child, source, blocks); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func appendMarkdownListBlocks(list ast.Node, source []byte, blocks *[]markdownVisibleBlock) error {
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if item.Kind() != ast.KindListItem {
			continue
		}
		var parts []string
		for child := item.FirstChild(); child != nil; child = child.NextSibling() {
			if child.Kind() == ast.KindList {
				continue
			}
			part := canonicalMarkdownVisibleText(visibleMarkdownText(child, source))
			if part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) > 0 {
			start, end, ok := markdownSourceRange(item, source)
			if !ok {
				return errors.New("document list item has no stable source range")
			}
			*blocks = append(*blocks, markdownVisibleBlock{text: strings.Join(parts, " "), sourceStart: start, sourceEnd: end})
		}
		for child := item.FirstChild(); child != nil; child = child.NextSibling() {
			if child.Kind() == ast.KindList {
				if err := appendMarkdownListBlocks(child, source, blocks); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func appendMarkdownVisibleBlock(node ast.Node, source []byte, blocks *[]markdownVisibleBlock, tableRow bool) error {
	var visible string
	if tableRow {
		cells := make([]string, 0, node.ChildCount())
		for cell := node.FirstChild(); cell != nil; cell = cell.NextSibling() {
			text := canonicalMarkdownVisibleText(visibleMarkdownText(cell, source))
			if text != "" {
				cells = append(cells, text)
			}
		}
		visible = strings.Join(cells, " ")
	} else if node.Kind() == ast.KindCodeBlock || node.Kind() == ast.KindFencedCodeBlock {
		visible = canonicalMarkdownVisibleText(string(node.Lines().Value(source)))
	} else {
		visible = canonicalMarkdownVisibleText(visibleMarkdownText(node, source))
	}
	if visible == "" {
		return nil
	}
	start, end, ok := markdownSourceRange(node, source)
	if !ok {
		return errors.New("document visible block has no stable source range")
	}
	*blocks = append(*blocks, markdownVisibleBlock{text: visible, sourceStart: start, sourceEnd: end})
	return nil
}

func visibleMarkdownText(node ast.Node, source []byte) string {
	var builder strings.Builder
	_ = ast.Walk(node, func(current ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if current.Kind() == ast.KindImage || current.Kind() == ast.KindRawHTML || current.Kind() == ast.KindHTMLBlock {
			return ast.WalkSkipChildren, nil
		}
		switch value := current.(type) {
		case *ast.Text:
			visible := value.Segment.Value(source)
			if !value.IsRaw() {
				visible = goldmarkutil.UnescapePunctuations(visible)
			}
			builder.Write(visible)
			if value.SoftLineBreak() || value.HardLineBreak() {
				builder.WriteByte(' ')
			}
		case *ast.String:
			visible := value.Value
			if !value.IsRaw() && !value.IsCode() {
				visible = goldmarkutil.UnescapePunctuations(visible)
			}
			builder.Write(visible)
		case *ast.AutoLink:
			builder.Write(value.Label(source))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return builder.String()
}

func canonicalMarkdownVisibleText(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.Join(strings.Fields(value), " ")
	return norm.NFC.String(value)
}

func markdownSourceRange(node ast.Node, source []byte) (int, int, bool) {
	start, end := len(source), -1
	_ = ast.Walk(node, func(current ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if current.Kind() == ast.KindImage {
			return ast.WalkSkipChildren, nil
		}
		if current.Type() == ast.TypeBlock {
			lines := current.Lines()
			for index := 0; index < lines.Len(); index++ {
				segment := lines.At(index)
				if segment.Start < start {
					start = segment.Start
				}
				if segment.Stop > end {
					end = segment.Stop
				}
			}
		}
		if textNode, ok := current.(*ast.Text); ok {
			if textNode.Segment.Start < start {
				start = textNode.Segment.Start
			}
			if textNode.Segment.Stop > end {
				end = textNode.Segment.Stop
			}
		}
		return ast.WalkContinue, nil
	})
	if end <= start || start < 0 || end > len(source) {
		return 0, 0, false
	}
	for start > 0 && source[start-1] != '\n' {
		start--
	}
	for end < len(source) && source[end] != '\n' {
		end++
	}
	if end <= start || !utf8.Valid(source[start:end]) {
		return 0, 0, false
	}
	return start, end, true
}

func containsUnsafeMarkdownNode(document ast.Node) bool {
	unsafe := false
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && (node.Kind() == ast.KindRawHTML || node.Kind() == ast.KindHTMLBlock) {
			unsafe = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return unsafe
}

func documentAnchorDigest(value string) string {
	return ingestionapplication.DocumentAnchorMapTextSHA256(value)
}
