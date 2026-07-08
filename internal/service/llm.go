package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"go.uber.org/zap"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
)

// LLM sentinel errors.
var (
	ErrProviderError  = errors.New("llm provider error")
	ErrEmptyResponse  = errors.New("llm returned empty response")
)

// LLMProvider defines the interface for LLM model access.
type LLMProvider interface {
	Chat(ctx context.Context, prompt string, opts ...LLMOption) (string, error)
}

// LLMOption configures a Chat request.
type LLMOption func(*LLMOptions)

// LLMOptions holds optional parameters for Chat requests.
type LLMOptions struct {
	MaxTokens   int
	Temperature float64
	Model       string
}

// WithMaxTokens sets the maximum tokens for the response.
func WithMaxTokens(n int) LLMOption {
	return func(o *LLMOptions) { o.MaxTokens = n }
}

// WithTemperature sets the response temperature.
func WithTemperature(t float64) LLMOption {
	return func(o *LLMOptions) { o.Temperature = t }
}

// WithModel overrides the default model for this request.
func WithModel(m string) LLMOption {
	return func(o *LLMOptions) { o.Model = m }
}

// NewLLMProvider creates a Provider from the given config.
func NewLLMProvider(cfg *config.Config) (LLMProvider, error) {
	opts := LLMOptions{
		MaxTokens:   cfg.LLMMaxTokens,
		Temperature: cfg.LLMTemperature,
		Model:       cfg.LLMModel,
	}

	switch cfg.LLMProvider {
	case "openai":
		llm, err := openai.New(
			openai.WithModel(cfg.LLMModel),
			openai.WithBaseURL(cfg.LLMBaseURL),
			openai.WithToken(cfg.LLMAPIKey),
		)
		if err != nil {
			return nil, fmt.Errorf("create openai provider: %w", err)
		}
		return newLangchainAdapter(llm, opts), nil
	case "anthropic":
		return nil, fmt.Errorf("anthropic provider not yet implemented")
	case "ollama":
		return nil, fmt.Errorf("ollama provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", cfg.LLMProvider)
	}
}

// langchainAdapter wraps a langchaingo llms.Model as a Provider.
type langchainAdapter struct {
	model llms.Model
	opts  LLMOptions
}

func newLangchainAdapter(model llms.Model, opts LLMOptions) *langchainAdapter {
	return &langchainAdapter{model: model, opts: opts}
}

func (a *langchainAdapter) Chat(ctx context.Context, prompt string, opts ...LLMOption) (string, error) {
	o := a.opts
	for _, fn := range opts {
		fn(&o)
	}

	llmOpts := make([]llms.CallOption, 0)
	if o.MaxTokens > 0 {
		llmOpts = append(llmOpts, llms.WithMaxTokens(o.MaxTokens))
	}
	if o.Temperature > 0 {
		llmOpts = append(llmOpts, llms.WithTemperature(o.Temperature))
	}

	resp, err := llms.GenerateFromSinglePrompt(ctx, a.model, prompt, llmOpts...)
	if err != nil {
		logging.L().Error("llm provider error",
			zap.Error(err),
		)
		return "", ErrProviderError
	}
	if resp == "" {
		return "", ErrEmptyResponse
	}
	return resp, nil
}

// LLMService defines LLM-powered business operations.
type LLMService interface {
	Summarize(ctx context.Context, content string) (string, error)
	LabelTopics(ctx context.Context, content string) ([]string, error)
	GenerateDigest(ctx context.Context, input DigestInput) (DigestOutput, error)
}

type llmServiceImpl struct {
	provider LLMProvider
}

// NewLLMService creates a new LLM Service backed by the given Provider.
func NewLLMService(provider LLMProvider) LLMService {
	return &llmServiceImpl{provider: provider}
}

func (s *llmServiceImpl) Summarize(ctx context.Context, content string) (string, error) {
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

func (s *llmServiceImpl) LabelTopics(ctx context.Context, content string) ([]string, error) {
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
	Posts []PostItem
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

func (s *llmServiceImpl) GenerateDigest(ctx context.Context, input DigestInput) (DigestOutput, error) {
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
		GeneratedAt: time.Now().Format(time.RFC3339),
	}, nil
}

// LLMChain orchestrates multi-step LLM pipelines for content aggregation.
type LLMChain struct {
	svc LLMService
}

// NewLLMChain creates a Chain backed by the given Service.
func NewLLMChain(svc LLMService) *LLMChain {
	return &LLMChain{svc: svc}
}

// BuildDailyDigest runs the full digest pipeline.
func (c *LLMChain) BuildDailyDigest(ctx context.Context, input DigestInput, opts ...ChainOption) (DigestOutput, error) {
	cfg := defaultChainConfig()
	for _, fn := range opts {
		fn(&cfg)
	}

	posts := make([]PostItem, len(input.Posts))
	for i, p := range input.Posts {
		posts[i] = p

		if posts[i].Summary == "" && cfg.summarize {
			summary, err := c.svc.Summarize(ctx, truncateContent(p.Content, cfg.maxContentLen))
			if err != nil {
				posts[i].Summary = ""
			} else {
				posts[i].Summary = summary
			}
		}

		if len(posts[i].Labels) == 0 && cfg.label {
			labels, err := c.svc.LabelTopics(ctx, truncateContent(p.Content, cfg.maxContentLen))
			if err != nil {
				posts[i].Labels = nil
			} else {
				posts[i].Labels = labels
			}
		}
	}

	digestInput := DigestInput{
		Title: input.Title,
		Posts: posts,
	}

	return c.svc.GenerateDigest(ctx, digestInput)
}

// ChainOption configures the Chain pipeline.
type ChainOption func(*chainConfig)

type chainConfig struct {
	summarize     bool
	label         bool
	maxContentLen int
}

func defaultChainConfig() chainConfig {
	return chainConfig{
		summarize:     true,
		label:         true,
		maxContentLen: 4000,
	}
}

// WithSummarize enables or disables per-post summarization.
func WithSummarize(enabled bool) ChainOption {
	return func(c *chainConfig) { c.summarize = enabled }
}

// WithLabel enables or disables per-post topic labeling.
func WithLabel(enabled bool) ChainOption {
	return func(c *chainConfig) { c.label = enabled }
}

// WithMaxContentLen sets the maximum content length per post (in characters).
func WithMaxContentLen(n int) ChainOption {
	return func(c *chainConfig) { c.maxContentLen = n }
}

func truncateContent(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
