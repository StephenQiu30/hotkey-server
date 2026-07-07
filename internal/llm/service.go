package llm

import (
	"context"
	"fmt"
	"strings"
)

// Service defines LLM-powered business operations.
type Service interface {
	// Summarize generates a concise summary of the given content.
	Summarize(ctx context.Context, content string) (string, error)
	// LabelTopics extracts relevant topic labels from the given content.
	LabelTopics(ctx context.Context, content string) ([]string, error)
	// GenerateDigest produces a structured daily digest combining summaries, labels, and original content.
	GenerateDigest(ctx context.Context, input DigestInput) (DigestOutput, error)
}

type serviceImpl struct {
	provider Provider
}

// NewService creates a new LLM Service backed by the given Provider.
func NewService(provider Provider) Service {
	return &serviceImpl{provider: provider}
}

func (s *serviceImpl) Summarize(ctx context.Context, content string) (string, error) {
	if content == "" {
		return "", ErrEmptyResponse
	}
	prompt := fmt.Sprintf("请用中文简洁地总结以下内容，控制在 200 字以内：\n\n%s", content)
	resp, err := s.provider.Chat(ctx, prompt, WithTemperature(0.3))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp), nil
}

func (s *serviceImpl) LabelTopics(ctx context.Context, content string) ([]string, error) {
	if content == "" {
		return nil, nil
	}
	prompt := fmt.Sprintf(`分析以下内容，提取 3-5 个关键词作为主题标签。
要求：
- 每个标签 2-6 个中文字符
- 用逗号分隔
- 只返回标签，不要序号和解释

内容：
%s`, content)

	resp, err := s.provider.Chat(ctx, prompt, WithTemperature(0.2))
	if err != nil {
		return nil, err
	}
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return nil, nil
	}
	labels := strings.Split(resp, "，")
	if len(labels) == 1 {
		labels = strings.Split(resp, ",")
	}
	result := make([]string, 0, len(labels))
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result, nil
}

// DigestInput is the input for GenerateDigest.
type DigestInput struct {
	Title string
	Date  string
	Posts []PostItem // each post with content + metadata
}

// PostItem represents a single post for digest generation.
type PostItem struct {
	ID          int64
	Title       string
	Content     string
	URL         string
	Platform    string
	Heat        float64
	PublishedAt string
	Labels      []string
	Summary     string
}

// DigestOutput is the structured daily digest.
type DigestOutput struct {
	Title       string          `json:"title"`
	Date        string          `json:"date"`
	Summary     string          `json:"summary"`
	Sections    []DigestSection `json:"sections"`
	GeneratedAt string          `json:"generated_at"`
}

// DigestSection is a topic section in the digest.
type DigestSection struct {
	Topic   string       `json:"topic"`
	Summary string       `json:"summary"`
	Posts   []PostDigest `json:"posts"`
}

// PostDigest is a single post entry in the digest.
type PostDigest struct {
	ID       int64    `json:"id"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	URL      string   `json:"url"`
	Platform string   `json:"platform"`
	Heat     float64  `json:"heat"`
	Labels   []string `json:"labels"`
}

func (s *serviceImpl) GenerateDigest(ctx context.Context, input DigestInput) (DigestOutput, error) {
	// Build prompt from all posts
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("请生成一份每日热点日报。\n\n日期：%s\n\n", input.Date))
	sb.WriteString(fmt.Sprintf("总览标题：%s\n\n", input.Title))

	for i, p := range input.Posts {
		sb.WriteString(fmt.Sprintf("--- 文章 %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("标题：%s\n", p.Title))
		sb.WriteString(fmt.Sprintf("平台：%s\n", p.Platform))
		sb.WriteString(fmt.Sprintf("热度：%.1f\n", p.Heat))
		if p.Summary != "" {
			sb.WriteString(fmt.Sprintf("摘要：%s\n", p.Summary))
		}
		if len(p.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("标签：%s\n", strings.Join(p.Labels, ", ")))
		}
		sb.WriteString(fmt.Sprintf("原文：%s\n\n", p.Content))
	}

	sb.WriteString(`请按以下格式输出：
## 今日总览
[整体趋势简要说明]

## 热点话题
### 话题1：[话题名]
- 热度趋势：[描述]
- 相关内容：[列举重要文章标题]

### 话题2：[话题名]
...（以此类推）

## 完整文章列表
### [文章标题]
- 平台：[平台名]
- 热度：[分数]
- 摘要：[一句话摘要]
- 标签：[标签列表]
- 原文链接：[URL]
`)

	resp, err := s.provider.Chat(ctx, sb.String(), WithTemperature(0.5), WithMaxTokens(4096))
	if err != nil {
		return DigestOutput{}, err
	}

	// Build structured output
	sections := make([]DigestSection, 0)
	posts := make([]PostDigest, len(input.Posts))
	for i, p := range input.Posts {
		posts[i] = PostDigest{
			ID:       p.ID,
			Title:    p.Title,
			Summary:  p.Summary,
			URL:      p.URL,
			Platform: p.Platform,
			Heat:     p.Heat,
			Labels:   p.Labels,
		}
	}
	sections = append(sections, DigestSection{
		Topic: "热点聚合",
		Posts: posts,
	})

	return DigestOutput{
		Title:       input.Title,
		Date:        input.Date,
		Summary:     resp,
		Sections:    sections,
		GeneratedAt: "now",
	}, nil
}
