# LLM 集成实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 langchaingo + 设计模式在 hotkey-server 中集成 LLM 能力，实现内容聚合的首个落地场景。

**Architecture:** Strategy + Factory + Adapter + Pipeline 四模式组合：Provider 接口统一不同模型，Factory 根据配置创建 provider，Adapter 封装 langchaingo 差异，Chain 编排内容聚合流水线。

**Tech Stack:** Go 1.26, langchaingo, DeepSeek v4-flash (OpenAI-compatible), go.uber.org/fx

---

### Task 1: 配置层 — LLM Config 类型 + .env.example 更新

**Files:**
- Modify: `internal/config/config.go:11-30` — 追加 LLMConfig 字段并移除旧死字段
- Modify: `internal/config/config.go:34-108` — LLM 配置加载逻辑

**Depends on:** None (ground layer)

- [ ] **Step 1: 在 Config 中追加 LLMConfig 结构**

```go
// 在 Config struct 中追加（替换旧的 LLMProvider/LLMAPIKey/LLMBaseURL/LLMModel 单字段）
type LLMConfig struct {
    Provider    string
    APIKey      string
    BaseURL     string
    Model       string
    MaxTokens   int
    Temperature float64
}

// Config 新增字段
LLM LLMConfig `mapstructure:",squash"` // squash 展开为 LLM_PROVIDER, LLM_API_KEY 等环境变量
```

实际因为 viper `mapstructure:",squash"` 的工作方式，还是保留现有单字段，或者嵌套结构体。更简单的方式：保留现有 4 个单字段并新增 `LLM_MAX_TOKENS` 和 `LLM_TEMPERATURE`。

- [ ] **Step 2: 在 Load() 中加载 LLM 配置并设置默认值**

```go
// SetDefaults
v.SetDefault("LLM_MAX_TOKENS", 4096)
v.SetDefault("LLM_TEMPERATURE", 0.7)

// BindEnv
_ = v.BindEnv("LLM_MAX_TOKENS")
_ = v.BindEnv("LLM_TEMPERATURE")

// 默认值兜底
if cfg.LLMMaxTokens <= 0 {
    cfg.LLMMaxTokens = 4096
}
if cfg.LLMTemperature <= 0 {
    cfg.LLMTemperature = 0.7
}
```

- [ ] **Step 3: 更新 .env.example**

```env
# --- LLM 内容聚合 ---
LLM_PROVIDER=openai
LLM_API_KEY=
LLM_BASE_URL=https://api.deepseek.com
LLM_MODEL=deepseek-v4-flash
LLM_MAX_TOKENS=4096
LLM_TEMPERATURE=0.7
```

- [ ] **Step 4: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 5: 提交**

```bash
git add -A && git commit -m "chore: add LLM config fields and update .env.example

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Provider 接口 + 自定义错误类型

**Files:**
- Create: `internal/llm/provider.go` — Provider 接口 + Option 类型
- Create: `internal/llm/errors.go` — 自定义错误

**Depends on:** Task 1 (config 定义)

- [ ] **Step 1: 创建 internal/llm 包和 provider.go**

```go
package llm

import "context"

// Provider defines the interface for LLM model access.
// Implementations wrap langchaingo or other backends.
type Provider interface {
    // Chat sends a chat completion request and returns the response text.
    Chat(ctx context.Context, prompt string, opts ...Option) (string, error)
}

// Option configures a Chat request.
type Option func(*Options)

// Options holds optional parameters for Chat requests.
type Options struct {
    MaxTokens   int
    Temperature float64
    Model       string // per-request model override
}

// WithMaxTokens sets the maximum tokens for the response.
func WithMaxTokens(n int) Option {
    return func(o *Options) { o.MaxTokens = n }
}

// WithTemperature sets the response temperature.
func WithTemperature(t float64) Option {
    return func(o *Options) { o.Temperature = t }
}

// WithModel overrides the default model for this request.
func WithModel(m string) Option {
    return func(o *Options) { o.Model = m }
}
```

- [ ] **Step 2: 创建 errors.go**

```go
package llm

import "errors"

var (
    // ErrProviderError is returned when the underlying LLM API call fails.
    ErrProviderError = errors.New("llm provider error")
    // ErrContentTooLong is returned when input exceeds the model context window.
    ErrContentTooLong = errors.New("content exceeds model context length")
    // ErrEmptyResponse is returned when the LLM returns empty content.
    ErrEmptyResponse = errors.New("llm returned empty response")
)
```

- [ ] **Step 3: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 4: 提交**

```bash
git add -A && git commit -m "feat: add Provider interface and custom LLM errors

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: langchaingo Adapter + Factory

**Files:**
- Create: `internal/llm/adapter.go` — langchaingo 适配 Provider 接口
- Create: `internal/llm/factory.go` — Factory 创建 provider
- Modify: `go.mod` / `go.sum` — 添加 langchaingo 依赖

**Depends on:** Task 2 (Provider 接口)

- [ ] **Step 1: 添加 langchaingo 依赖**

```bash
go get github.com/tmc/langchaingo
go get github.com/tmc/langchaingo/llms/openai
```

- [ ] **Step 2: 创建 adapter.go**

```go
package llm

import (
    "context"

    "github.com/tmc/langchaingo/llms"
)

// langchainAdapter wraps a langchaingo llms.Model as a Provider.
type langchainAdapter struct {
    model llms.Model
    opts  Options // default options
}

func newLangchainAdapter(model llms.Model, opts Options) *langchainAdapter {
    return &langchainAdapter{model: model, opts: opts}
}

func (a *langchainAdapter) Chat(ctx context.Context, prompt string, opts ...Option) (string, error) {
    o := a.opts
    for _, fn := range opts {
        fn(&o)
    }

    llmOpts := make([]llms.CallOption, 0)
    if o.MaxTokens > 0 {
        llmOpts = append(llmOpts, llms.WithMaxTokens(o.MaxTokens))
    }
    if o.Temperature > 0 {
        llmOpts = append(llmOpts, llms.WithTemperature(float32(o.Temperature)))
    }

    resp, err := llms.GenerateFromSinglePrompt(ctx, a.model, prompt, llmOpts...)
    if err != nil {
        return "", ErrProviderError
    }
    if resp == "" {
        return "", ErrEmptyResponse
    }
    return resp, nil
}
```

- [ ] **Step 3: 创建 factory.go**

```go
package llm

import (
    "fmt"

    "github.com/StephenQiu30/hotkey-server/internal/config"
    "github.com/tmc/langchaingo/llms/openai"
)

// NewProvider creates a Provider from the given config.
// DeepSeek and OpenAI-compatible models use the openai provider with a custom base URL.
// Add cases here for anthropic, ollama, etc. when needed.
func NewProvider(cfg config.Config) (Provider, error) {
    llmCfg := cfg.LLM
    opts := Options{
        MaxTokens:   llmCfg.MaxTokens,
        Temperature: llmCfg.Temperature,
        Model:       llmCfg.Model,
    }

    switch llmCfg.Provider {
    case "openai":
        llm, err := openai.New(
            openai.WithModel(llmCfg.Model),
            openai.WithBaseURL(llmCfg.BaseURL),
            openai.WithToken(llmCfg.APIKey),
        )
        if err != nil {
            return nil, fmt.Errorf("create openai provider: %w", err)
        }
        return newLangchainAdapter(llm, opts), nil

    case "anthropic":
        // anthropic provider — add when langchaingo anthropic is imported
        // llm, err := anthropic.New(...)
        return nil, fmt.Errorf("anthropic provider not yet implemented")

    case "ollama":
        // ollama provider — add when langchaingo ollama is imported
        return nil, fmt.Errorf("ollama provider not yet implemented")

    default:
        return nil, fmt.Errorf("unknown LLM provider: %q", llmCfg.Provider)
    }
}
```

- [ ] **Step 4: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功（注意 go.sum 变化）

- [ ] **Step 5: 提交**

```bash
git add -A && git commit -m "feat: add langchaingo adapter and provider factory

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: LLM Service — 业务接口 + Summarize/LabelTopics 实现

**Files:**
- Create: `internal/llm/service.go` — Service 接口 + 业务实现
- Create: `tests/unit/llm/provider_test.go` — mock provider 测试

**Depends on:** Task 2, Task 3 (Provider 接口 + Factory)

- [ ] **Step 1: 创建 service.go**

```go
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
    Title       string
    Posts       []PostItem  // each post with content + metadata
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
    Title       string            `json:"title"`
    Date        string            `json:"date"`
    Summary     string            `json:"summary"`
    Sections    []DigestSection   `json:"sections"`
    GeneratedAt string            `json:"generated_at"`
}

// DigestSection is a topic section in the digest.
type DigestSection struct {
    Topic       string        `json:"topic"`
    Summary     string        `json:"summary"`
    Posts       []PostDigest  `json:"posts"`
}

// PostDigest is a single post entry in the digest.
type PostDigest struct {
    ID          int64   `json:"id"`
    Title       string  `json:"title"`
    Summary     string  `json:"summary"`
    URL         string  `json:"url"`
    Platform    string  `json:"platform"`
    Heat        float64 `json:"heat"`
    Labels      []string `json:"labels"`
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
```

- [ ] **Step 2: 创建单元测试（mock provider）**

```go
// tests/unit/llm/service_test.go
package llm_test

import (
    "context"
    "strings"
    "testing"

    "github.com/StephenQiu30/hotkey-server/internal/llm"
)

// mockProvider implements llm.Provider for testing.
type mockProvider struct {
    response string
    err      error
}

func (m *mockProvider) Chat(_ context.Context, prompt string, opts ...llm.Option) (string, error) {
    return m.response, m.err
}

func TestSummarize_ReturnsSummary(t *testing.T) {
    svc := llm.NewService(&mockProvider{response: "这是一个测试摘要。"})
    result, err := svc.Summarize(context.Background(), "测试内容")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result == "" {
        t.Fatal("expected non-empty summary")
    }
}

func TestSummarize_EmptyInput_ReturnsError(t *testing.T) {
    svc := llm.NewService(&mockProvider{response: ""})
    _, err := svc.Summarize(context.Background(), "")
    if err == nil {
        t.Fatal("expected error for empty input")
    }
}

func TestLabelTopics_ReturnsLabels(t *testing.T) {
    svc := llm.NewService(&mockProvider{response: "AI, 科技, 创新"})
    labels, err := svc.LabelTopics(context.Background(), "AI technology content")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(labels) == 0 {
        t.Fatal("expected non-empty labels")
    }
}

func TestLabelTopics_EmptyContent_ReturnsEmpty(t *testing.T) {
    svc := llm.NewService(&mockProvider{response: ""})
    labels, err := svc.LabelTopics(context.Background(), "")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(labels) != 0 {
        t.Fatalf("expected 0 labels, got %d", len(labels))
    }
}

func TestProviderError_Propagated(t *testing.T) {
    svc := llm.NewService(&mockProvider{err: llm.ErrProviderError})
    _, err := svc.Summarize(context.Background(), "test")
    if err != llm.ErrProviderError {
        t.Fatalf("expected ErrProviderError, got %v", err)
    }
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./tests/unit/llm/ -v -count=1`
Expected: 全部 PASS

- [ ] **Step 4: 提交**

```bash
git add -A && git commit -m "feat: add LLM Service with Summarize, LabelTopics, GenerateDigest

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Chain Pipeline — 内容聚合编排

**Files:**
- Create: `internal/llm/chain.go` — Pipeline 编排
- Modify: `tests/unit/llm/service_test.go` — 追加 chain 测试

**Depends on:** Task 4 (Service 接口)

- [ ] **Step 1: 创建 chain.go**

```go
package llm

import "context"

// Chain orchestrates multi-step LLM pipelines for content aggregation.
type Chain struct {
    svc Service
}

// NewChain creates a Chain backed by the given Service.
func NewChain(svc Service) *Chain {
    return &Chain{svc: svc}
}

// BuildDailyDigest runs the full digest pipeline: summarize each post,
// label each post, then compile the final digest.
func (c *Chain) BuildDailyDigest(ctx context.Context, input DigestInput, opts ...ChainOption) (DigestOutput, error) {
    cfg := defaultChainConfig()
    for _, fn := range opts {
        fn(&cfg)
    }

    posts := make([]PostItem, len(input.Posts))
    for i, p := range input.Posts {
        posts[i] = p

        // Summarize if not already provided
        if posts[i].Summary == "" && cfg.summarize {
            summary, err := c.svc.Summarize(ctx, truncateContent(p.Content, cfg.maxContentLen))
            if err != nil {
                // Non-blocking: skip summary on error, continue with empty
                posts[i].Summary = ""
            } else {
                posts[i].Summary = summary
            }
        }

        // Label if not already provided
        if len(posts[i].Labels) == 0 && cfg.label {
            labels, err := c.svc.LabelTopics(ctx, truncateContent(p.Content, cfg.maxContentLen))
            if err != nil {
                // Non-blocking: skip labels on error
                posts[i].Labels = nil
            } else {
                posts[i].Labels = labels
            }
        }
    }

    // Compile the final digest
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
```

- [ ] **Step 2: 追加 Chain 测试到 service_test.go**

```go
func TestChainBuildDailyDigest_CallsSummarizeAndLabel(t *testing.T) {
    svc := llm.NewService(&mockProvider{response: "test summary"})
    chain := llm.NewChain(svc)

    output, err := chain.BuildDailyDigest(context.Background(), llm.DigestInput{
        Title: "Test Digest",
        Posts: []llm.PostItem{
            {ID: 1, Title: "Post 1", Content: "Content 1", Platform: "x"},
            {ID: 2, Title: "Post 2", Content: "Content 2", Platform: "weibo"},
        },
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if output.Title != "Test Digest" {
        t.Fatalf("expected 'Test Digest', got '%s'", output.Title)
    }
    if len(output.Sections) == 0 {
        t.Fatal("expected at least one section")
    }
}

func TestChainBuildDailyDigest_EmptyPosts(t *testing.T) {
    svc := llm.NewService(&mockProvider{response: "empty digest"})
    chain := llm.NewChain(svc)

    _, err := chain.BuildDailyDigest(context.Background(), llm.DigestInput{
        Title: "Empty Digest",
        Posts: []llm.PostItem{},
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./tests/unit/llm/ -v -count=1`
Expected: 全部 PASS

- [ ] **Step 4: 提交**

```bash
git add -A && git commit -m "feat: add Chain pipeline for content aggregation

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Fx DI 集成 — 注册 Provider + Service + Chain 到应用

**Files:**
- Modify: `internal/fxapp/app.go` — 添加 Fx Provide
- Modify: `internal/config/config.go` — 暴露 LLMConfig getter
- Verify: `go build` + `make ci`

**Depends on:** Task 3, Task 4, Task 5 (所有 LLM 包组件)

- [ ] **Step 1: 在 config.go 中添加 LLMConfig getter**

```go
// 在 Config struct 旁边，已有 LLM_* 字段的基础上确认暴露接口
// 目前的 Config 已有 LLMProvider/LLMAPIKey/LLMBaseURL/LLMModel 字段
// 新增字段：
type Config struct {
    // ... 现有字段 ...

    LLMMaxTokens   int     `mapstructure:"LLM_MAX_TOKENS"`
    LLMTemperature float64 `mapstructure:"LLM_TEMPERATURE"`
}

// 新增 LLMConfig() 方法
func (c Config) LLMConfig() LLMConfig {
    return LLMConfig{
        Provider:    c.LLMProvider,
        APIKey:      c.LLMAPIKey,
        BaseURL:     c.LLMBaseURL,
        Model:       c.LLMModel,
        MaxTokens:   c.LLMMaxTokens,
        Temperature: c.LLMTemperature,
    }
}

// LLMConfig 类型
type LLMConfig struct {
    Provider    string
    APIKey      string
    BaseURL     string
    Model       string
    MaxTokens   int
    Temperature float64
}
```

- [ ] **Step 2: 在 fxapp/app.go 中注册 LLM 组件**

```go
// 添加 import
"github.com/StephenQiu30/hotkey-server/internal/llm"

// 在 fx.Provide 区追加：
fx.Provide(fx.Annotate(llm.NewProvider, fx.As(new(llm.Provider)))),
fx.Provide(fx.Annotate(llm.NewService, fx.As(new(llm.Service)))),
fx.Provide(llm.NewChain),
```

> 注意：`NewProvider` 依赖 config，Fx 会自动注入。需要确保 `NewProvider` 的参数签名正确。

- [ ] **Step 3: 调整 llm/factory.go 签名以适配 Fx**

```go
// factory.go 需要适配 Fx DI — 直接从 *config.Config 解析
func NewProvider(cfg *config.Config) (Provider, error) {
    llmCfg := cfg.LLMConfig()
    // ... 同 Task 3 的实现 ...
}
```

- [ ] **Step 4: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 5: 运行 CI 验证**

Run: `make ci`
Expected: 全部通过（build + vet + test + validate + smoke）

- [ ] **Step 6: 提交**

```bash
git add -A && git commit -m "feat: wire LLM components into Fx DI graph

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
