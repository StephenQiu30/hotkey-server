# LLM 集成设计 — langchaingo + Design Patterns

> **日期:** 2026-07-07
> **项目:** hotkey-server
> **状态:** 草案 → 审批

## 目标

在 hotkey-server 中集成 LLM 能力，基于 langchaingo 框架，通过设计模式实现 Provider 可切换、业务场景可组合的内容聚合管道。首个落地场景为**内容聚合**（摘要 + 主题标签 + 日报生成）。

## 非目标

- 不支持实时流式输出（Streaming），MVP 阶段仅做 request/response
- 不引入 Python sidecar 或外部 LLM 编排服务
- 不接入 langchaingo 的 Agent / Memory / Tool 等高级特性
- 不修改现有的内容采集、存储、计算数据流

## 技术选型

| 组件 | 选择 | 理由 |
|------|------|------|
| LLM 框架 | langchaingo | 社区主力 Go 方案，多 provider 支持 |
| 模型 | DeepSeek v4-flash | OpenAI-compatible，langchaingo 原生支持 |
| 设计模式 | Strategy + Factory + Adapter + Pipeline | 各自解决一个明确的关注点 |
| DI | go.uber.org/fx | 项目已有，保持一致 |

## 架构

```
┌──────────────────────────────────────────────────────┐
│                  业务调用方                            │
│  topic.Service  /  hotevent.Service  /  digest        │
└─────────────────────┬────────────────────────────────┘
                      │ (依赖 llm.Service 接口)
┌─────────────────────▼────────────────────────────────┐
│  internal/llm/                                       │
│                                                      │
│  ┌──────────────┐   ┌───────────────────────────┐   │
│  │  Service     │──▶│  Summarize()               │   │
│  │  (接口)      │   │  LabelTopics()             │   │
│  │              │   │  GenerateDigest()          │   │
│  └──────────────┘   └───────────┬───────────────┘   │
│                                 │                     │
│  ┌──────────────────────────────▼────────────────┐   │
│  │  Chain (Pipeline)                             │   │
│  │  FetchPost → Summarize → Label → Compile      │   │
│  └──────────────────────────────┬────────────────┘   │
│                                 │                     │
│  ┌──────────────────────────────▼────────────────┐   │
│  │  Provider  (Strategy 接口)                     │   │
│  │  Chat(ctx, prompt) → (string, error)          │   │
│  └──────────────────────────────┬────────────────┘   │
│                                 │                     │
│  ┌──────────────────────────────▼────────────────┐   │
│  │  Factory                                       │   │
│  │  NewProvider(cfg) → Provider                  │   │
│  │  ├── "deepseek" → langchaingo openai.New()     │   │
│  │  ├── "anthropic" → langchaingo anthropic.New() │   │
│  │  └── "ollama" → langchaingo ollama.New()       │   │
│  └───────────────────────────────────────────────┘   │
│                                                      │
│  ┌───────────────────────────────────────────────┐   │
│  │  Adapter (langchaingo 封装层)                   │   │
│  │  langchainAdapter struct → 实现 Provider       │   │
│  └───────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────┘
```

## 设计模式

### Strategy — `Provider` 接口

```go
type Provider interface {
    Chat(ctx context.Context, prompt string, opts ...Option) (string, error)
}
```

不同模型通过同一个接口调用，切换 Provider 即切换后端模型。

### Factory — `NewProvider`

```go
func NewProvider(cfg config.LLMConfig) (Provider, error)
```

- `cfg.Provider = "openai"` → `langchaingo openai.New(baseURL=deepseek-api, model=deepseek-v4-flash)`
- `cfg.Provider = "anthropic"` → `langchaingo anthropic.New(claude-sonnet-4-8)`
- 新增 provider 只加一个 `case` branch

### Adapter — `langchainAdapter`

将 langchaingo `llms.Model` 的差异 API 统一到 `Provider` 接口，解耦业务代码与 langchaingo 的具体类型。

### Pipeline — `Chain`

```go
type DigestInput struct {
    Posts  []PostRaw       // 原始帖文
    Topics []TopicRaw      // 已有话题
}

type DigestOutput struct {
    Summary      string          // LLM 摘要
    Labels       []string        // 自动标签
    PostDigests  []PostDigest    // 每篇文章：摘要 + 标签 + 原文链接
    GeneratedAt  time.Time
}
```

Pipeline 步骤：
1. `SummarizePosts(ctx, posts)` → 每篇文章的摘要
2. `LabelPosts(ctx, posts, summaries)` → 每篇文章的主题标签
3. `CompileDigest(ctx, posts, summaries, labels, topics)` → 合并为结构化日报

## 文件清单

```
internal/llm/
├── provider.go       — Provider 接口 + Option 类型      ~30 行
├── factory.go        — Factory 创建 langchaingo model   ~50 行
├── adapter.go        — langchaingo 适配 Provider 接口    ~40 行
├── service.go        — Summarize / LabelTopics 业务     ~80 行
├── chain.go          — Pipeline 编排：日报完整流        ~70 行
├── errors.go         — 自定义错误                       ~20 行

internal/config/config.go  — 追加 LLM 配置字段            ~10 行
.env.example               — 更新注释                     ~5 行
internal/fxapp/app.go      — Fx Provide                  ~4 行
```

总计约 **310 行新增**。

## 配置

```go
type LLMConfig struct {
    Provider    string   // "openai" | "anthropic" | "ollama"
    APIKey      string
    BaseURL     string   // DeepSeek v4-flash: https://api.deepseek.com
    Model       string   // "deepseek-v4-flash"
    MaxTokens   int
    Temperature float64
}
```

`.env.example` 追加：

```env
# --- LLM 内容聚合 ---
LLM_PROVIDER=openai
LLM_API_KEY=
LLM_BASE_URL=https://api.deepseek.com
LLM_MODEL=deepseek-v4-flash
```

> DeepSeek 走 OpenAI-compatible API，因此 `LLM_PROVIDER=openai` + 改 `BASE_URL` 和 `MODEL` 即可。

## 错误处理

- `ProviderError`：底层 API 调用失败（超时、认证、限流）
- `ContentTooLongError`：输入超过模型上下文窗口
- `EmptyResponseError`：LLM 返回空内容

调用方（业务层）统一捕获 `ProviderError`，日志记录后降级：摘要为空则跳过，标签为空返回空列表，日报失败不阻塞整体流程。

## 依赖

```bash
go get github.com/tmc/langchaingo
go get github.com/tmc/langchaingo/llms/openai
```

langchaingo 的 Anthropic / Ollama provider 在需要时再添加具体依赖。

## 测试策略

- **`Provider` 层**：mock 一个 `Provider` 实现，验证业务逻辑
- **`Chain` 层**：mock `Provider`，验证 Pipeline 步骤顺序和输出结构
- **集成**：仅 CI 中用真实 DeepSeek API（可选，用 `X_BEARER_TOKEN` 类似方式）

## Spec 自检

1. ✅ 无 TBD/TODO 占位符
2. ✅ 内部一致：设计模式与文件结构对应
3. ✅ 范围聚焦：单个 LLM 集成子项目，不含无关改造
4. ✅ 无歧义：DeepSeek 兼容 OpenAI API 已明确说明
