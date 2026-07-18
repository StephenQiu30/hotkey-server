package markdown

import (
	"strings"
	"testing"
)

func TestConverterProducesSafeCommonMarkAndGFMTable(t *testing.T) {
	t.Parallel()

	converter := NewConverter()
	got, err := converter.Convert(`<article>
		<h1>标题</h1><p>正文 <a href="/read">相对链接</a> <a href="mailto:news@example.test">邮件</a></p>
		<table><thead><tr><th>名称</th><th>值</th></tr></thead><tbody><tr><td>热度</td><td>90</td></tr></tbody></table>
		<ul><li>列表</li></ul><pre><code class="language-go">fmt.Println("ok")</code></pre>
		<script>alert(1)</script><style>body{display:none}</style><iframe src="https://evil.test"></iframe>
		<form action="https://evil.test"><input value="secret"></form><img src="https://remote.test/tracker.png" alt="tracker">
		<a href="javascript:alert(1)">危险链接</a><a href="data:text/html,bad">数据链接</a>
	</article>`, "https://example.test/base/")
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	for _, want := range []string{"# 标题", "[相对链接](https://example.test/read)", "[邮件](mailto:news@example.test)", "| 名称 | 值 |", "- 列表", "```go"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Convert() = %q, want %q", got, want)
		}
	}
	for _, forbidden := range []string{"alert(1)", "display:none", "evil.test", "tracker.png", "javascript:", "data:text"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Convert() leaked %q: %s", forbidden, got)
		}
	}
}

func TestConverterRejectsInvalidBaseURLAndEmptyProjection(t *testing.T) {
	t.Parallel()

	converter := NewConverter()
	if _, err := converter.Convert(`<p>正文</p>`, "javascript:alert(1)"); err == nil {
		t.Fatal("Convert() error = nil, want invalid base URL rejection")
	}
	if _, err := converter.Convert(`<script>alert(1)</script>`, "https://example.test/article"); err == nil {
		t.Fatal("Convert() error = nil, want empty safe projection rejection")
	}
}
