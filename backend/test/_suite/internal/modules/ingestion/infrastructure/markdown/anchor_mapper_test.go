package markdown

import (
	"context"
	"strings"
	"testing"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

func TestAnchorMapperBuildsCanonicalUTF8BlockMapForCommonMarkAndGFM(t *testing.T) {
	t.Parallel()
	markdown := strings.TrimSpace(`# 中文标题

Second **safe** [link](https://example.test/story).

- first item
- second item

| Signal | Score |
| --- | ---: |
| AI | 90 |

~~~text
code line
第二行
~~~`)
	mapper := NewAnchorMapper()
	result, err := mapper.MapDocumentText(context.Background(), ingestionapplication.MapDocumentTextCommand{Markdown: markdown})
	if err != nil {
		t.Fatalf("MapDocumentText() error = %v", err)
	}
	wantPlaintext := "中文标题\n\nSecond safe link.\n\nfirst item\n\nsecond item\n\nSignal Score\n\nAI 90\n\ncode line 第二行"
	if result.Plaintext != wantPlaintext {
		t.Fatalf("plaintext = %q, want %q", result.Plaintext, wantPlaintext)
	}
	if len(result.Blocks) != 7 {
		t.Fatalf("blocks = %#v", result.Blocks)
	}
	if err := ingestionapplication.ValidateMapDocumentTextResult(ingestionapplication.MapDocumentTextCommand{Markdown: markdown}, result); err != nil {
		t.Fatalf("mapped result is invalid: %v", err)
	}
	for index, block := range result.Blocks {
		if block.Ordinal != index || block.PlaintextUTF8ByteStart < 0 || block.PlaintextUTF8ByteEnd <= block.PlaintextUTF8ByteStart ||
			block.MarkdownUTF8ByteStart < 0 || block.MarkdownUTF8ByteEnd <= block.MarkdownUTF8ByteStart ||
			!strings.HasPrefix(block.MarkdownAnchor, "body-") {
			t.Fatalf("block %d = %#v", index, block)
		}
	}
	if got := result.Blocks[0].PlaintextUTF8ByteEnd; got != int64(len("中文标题")) {
		t.Fatalf("Chinese block end = %d, want UTF-8 bytes %d", got, len("中文标题"))
	}
}

func TestAnchorMapperUsesRenderedCommonMarkTextForBackslashEscapes(t *testing.T) {
	t.Parallel()
	markdown := "Article URL: [https://example.test/pg\\_stat\\_ch](https://example.test/pg_stat_ch)\n\n\\# Comments: 0\n\n`keep\\_code`"

	result, err := NewAnchorMapper().MapDocumentText(
		context.Background(),
		ingestionapplication.MapDocumentTextCommand{Markdown: markdown},
	)
	if err != nil {
		t.Fatalf("MapDocumentText() error = %v", err)
	}
	want := "Article URL: https://example.test/pg_stat_ch\n\n# Comments: 0\n\nkeep\\_code"
	if result.Plaintext != want {
		t.Fatalf("plaintext = %q, want %q", result.Plaintext, want)
	}
}

func TestAnchorMapperFailsClosedOnUnsupportedBodylessOrUnsafeMarkdown(t *testing.T) {
	t.Parallel()
	mapper := NewAnchorMapper()
	for name, markdown := range map[string]string{
		"empty":         "",
		"raw HTML only": "<script>alert(1)</script>",
		"image only":    "![tracking](https://tracker.example.test/pixel.png)",
		"non NFC":       "Cafe\u0301",
		"CR newline":    "one\r\ntwo",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mapper.MapDocumentText(context.Background(), ingestionapplication.MapDocumentTextCommand{Markdown: markdown}); err == nil {
				t.Fatalf("MapDocumentText() accepted %s", name)
			}
		})
	}
}
