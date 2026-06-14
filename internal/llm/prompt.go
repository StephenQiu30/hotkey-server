package llm

import (
	"fmt"
	"strings"
)

const maxPostChars = 500

var systemPrompt = `你是一位专业的中文新闻分析师。请根据提供的热点主题和相关帖子，生成一段客观、准确的中文摘要。
要求：
1. 摘要应客观描述主题内容，不编造事实
2. 仅基于提供的帖子内容进行总结
3. 摘要长度 2-4 段
4. 使用简洁专业的中文`

// BuildPrompt constructs the user-facing prompt message for topic summarization.
// Each post content is truncated to maxPostChars to control token usage.
func BuildPrompt(in TopicSummaryInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## 监控主题：%s\n", in.MonitorName)
	fmt.Fprintf(&b, "## 热点标题：%s\n", in.TopicTitle)
	if in.TopicKey != "" {
		fmt.Fprintf(&b, "## 主题标签：%s\n", in.TopicKey)
	}
	fmt.Fprintf(&b, "## 热度：%.1f\n", in.Heat)
	fmt.Fprintf(&b, "## 趋势：%s\n", in.Trend)
	fmt.Fprintf(&b, "## 帖子数量：%d\n", in.PostCount)
	b.WriteString("\n## 代表帖子：\n")

	for i, p := range in.Posts {
		content := truncateContent(p.Content, maxPostChars)
		fmt.Fprintf(&b, "\n### 帖子 %d\n", i+1)
		fmt.Fprintf(&b, "作者：%s\n", p.Author)
		fmt.Fprintf(&b, "内容：%s\n", content)
		fmt.Fprintf(&b, "链接：%s\n", p.URL)
	}

	b.WriteString("\n请根据以上信息生成中文摘要。")
	return b.String()
}

// truncateContent truncates s to max runes, appending "...(截断)" if truncated.
func truncateContent(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "...(截断)"
}
