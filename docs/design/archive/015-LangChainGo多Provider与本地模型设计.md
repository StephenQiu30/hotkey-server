---
layer: Design
doc_no: "015"
audience: [PM, Dev, QA, Ops]
feature_area: AI运行基础
purpose: 定义通过 LangChainGo 接入 DeepSeek、Ollama 与 Qwen Embedding 的长期边界
canonical_path: docs/design/archive/015-LangChainGo多Provider与本地模型设计.md
status: accepted
version: v1.4
owner: HotKey Server Team
inputs:
  - AGENTS.md
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/archive/011-AI任务证据与模型运行设计.md
  - docs/prd/archive/008-AIProvider与Embedding基础.md
outputs:
  - DeepSeek 与 Ollama Provider 契约
  - Qwen3 Embedding 1024 维模型契约
  - LangChainGo 与既有 AI 运行治理的职责边界
triggers:
  - 新增或修改 DeepSeek、Ollama、Qwen Embedding、Provider endpoint 或 LangChainGo
downstream:
  - docs/prd/archive/018-LangChainGo多模型接入.md
  - docs/plans/archive/018-LangChainGo多模型接入计划.md
  - docs/operations/007-LangChainGo多模型升级与连接.md
---

# LangChainGo 多 Provider 与本地模型设计

## 1. 目标与非目标

本设计在既有 AI 运行、预算、重试、复用、审计和 1024 维向量契约之上新增两个逻辑 Provider：远程 `deepseek` 与本地 `ollama`。二者由 `github.com/tmc/langchaingo` 适配；现有 `openai` 继续使用已验收的官方 OpenAI Responses/Embeddings SDK，`onnx` 继续作为可选本地 Embedding 实现。

LangChainGo 只负责供应商协议调用与标准消息/Embedding 转换。模型档案选择、每日预算、最大尝试、退避、运行 lease、JSON Schema 校验、一次修复、稳定错误码、向量长度校验和持久化仍由现有 Domain/Application 承担。当前不引入 Agent、Chain、Memory、Tool calling、通用 RAG 编排、动态插件或第二套模型档案。

## 2. Provider 与能力矩阵

| Provider | 协议与实现 | 允许任务 | 凭据引用 | endpoint 来源 |
|---|---|---|---|---|
| `openai` | 官方 `openai-go` | 全部现有任务 | `env:OPENAI_API_KEY` | 固定官方 HTTPS |
| `deepseek` | LangChainGo `llms/openai` Chat Completions | 除 `embedding` 外的全部现有结构化任务 | `env:DEEPSEEK_API_KEY` | 固定 `https://api.deepseek.com` |
| `ollama` | LangChainGo 原生 `llms/ollama` | 全部现有任务，包括 `embedding` | `NULL` | 受信任进程配置，默认 `http://127.0.0.1:11434` |
| `onnx` | 既有可选 ONNX adapter | 仅 `embedding` | `NULL` | 本地 artifact 路径 |

`provider`、`model_name`、`model_version`、`credential_ref` 和 Embedding 维度仍是不可变 profile 事实。切换供应商或模型必须创建新 profile，不得修改历史 profile 或重写旧向量。

DeepSeek 使用 OpenAI 兼容 Chat Completions，但它是独立逻辑 Provider，不能借用 `openai` 名称、OpenAI key 或 OpenAI 预算账本。DeepSeek endpoint 不进入数据库或 HTTP DTO，避免管理员把密钥发送到任意地址。

凭据引用与进程配置的唯一映射为：数据库中的 `env:DEEPSEEK_API_KEY` 只能由 `config.AI.DeepSeekAPIKey` 解析，而该字段只读取 `HOTKEY_DEEPSEEK_API_KEY`；adapter 不调用 `os.Getenv`，也不解析任意 `env:NAME`。这与既有 `env:OPENAI_API_KEY` → `config.AI.OpenAIAPIKey` → `HOTKEY_OPENAI_API_KEY` 边界一致。

Ollama endpoint 是进程级受信任部署配置，不进入数据库、模型档案响应、审计详情或指标标签。URL 必须在装配前验证：仅 `http`/`https`、必须有 host、不得含 userinfo、query 或 fragment；路径只允许空或 `/`，统一移除末尾 `/`。无效 URL 使 Ollama Provider 不注册并输出安全配置诊断，不得触发 LangChainGo 的 fatal 路径。

## 3. Qwen Embedding 空间

当前数据库、Domain 和近邻查询的唯一向量契约仍为 `halfvec(1024)`。Ollama Embedding profile 固定使用 `model_name=qwen3-embedding:0.6b`、`embedding_dimensions=1024`；该模型原生输出 1024 维，适配器仍必须在每次返回后验证数量、顺序和有限性。

所有 Ollama profile 的 `model_version` 不是可变 tag 或人工语义名，而是 `/api/tags` 为该精确 `model_name` 返回 digest 去掉 `sha256:` 前缀后的 64 个小写十六进制字符。这保持既有 `varchar(64)` 数据契约，不扩大列迁移。adapter 在每次生成或 Embedding 调用前通过同一个安全 HTTP client 读取 `/api/tags`，规范化并要求名称唯一匹配且 digest 与 profile 相等；不相等时以 `70000` 失败且不调用 `/api/chat` 或 `/api/embed`。fixture 必须覆盖匹配、缺失和 tag 漂移。这样 profile/run/vector 的 `model_version` 绑定不可变模型内容，而不是可能被重新指向的 tag。

`qwen3-embedding:4b` 与 `qwen3-embedding:8b` 的原生输出不是 1024 维，而 LangChainGo 原生 Ollama Embedding 请求当前没有 dimensions 参数，因此本阶段禁止用这两个 tag 创建 Embedding profile。未来若需要更大模型，必须先另立 Design 修改向量维度、Schema、索引、查询隔离、升级与回退流程。

Ollama 的结构化生成 profile 可使用部署中存在的任意显式模型名；服务不自动 pull 模型，也不把缺失模型伪装为成功。模型缺失映射为稳定的 profile invalid 或 unavailable 结果，原始响应正文不向外泄露。

## 4. 结构化生成与修复

DeepSeek 与 Ollama adapter 将既有 `StructuredRequest` 的 instruction、输入 JSON 和有限 repair violations 编码为受控消息。首次调用要求 JSON 输出；Application 继续用仓库静态 JSON Schema 验证。首次结果非法时，Application 按既有规则发起唯一一次修复；第二次失败为 `70006 ai_output_invalid`。

LangChainGo 或供应商的原始类型、错误正文、Prompt、完整输入、完整输出和 endpoint 不穿透 Infrastructure。adapter 只回传 `json.RawMessage`、经校验的 provider model ID、本地 model version、标准 Usage 与稳定 ProviderError。

## 5. HTTP、超时与错误映射

每个 adapter 使用由 HotKey 创建的独立 `http.Client`，其 deadline 来自请求上下文和 profile timeout；LangChainGo/SDK 内部重试必须关闭，避免与 Application 的 `max_attempts` 重复。共享安全 RoundTripper 在状态码非 2xx 时关闭并丢弃有限响应体，只返回不包含正文、header 或 URL query 的 typed status error。

稳定映射为：context deadline/transport timeout → `70005`；HTTP 429 → `70003`；HTTP 408、500–599 与临时 transport error → `70004`；认证、请求、模型不存在及其他 4xx → `70000`；无法装配/未启用 → `70001`。任何日志、Result 或指标标签不得包含 API key、Authorization、endpoint、供应商正文或用户输入。

## 6. 配置与装配

新增配置：

- `HOTKEY_DEEPSEEK_API_KEY`：DeepSeek 唯一凭据，空值时不注册 Provider。
- `HOTKEY_OLLAMA_ENABLED`：显式启用本地 Provider，默认 `false`。
- `HOTKEY_OLLAMA_BASE_URL`：启用时读取，默认 `http://127.0.0.1:11434`。

Bootstrap 仍构造一个按 `ProviderName` 索引的 registry。每个 Provider 是否可用只影响对应 profile 选择，不得阻塞采集、规则匹配、报告读取或其他非 AI 主链路。启用 Ollama 但 endpoint 不可达时允许服务启动；调用时返回安全的 unavailable/transient/timeout，连接健康由 Operations 的显式探测确认。

## 7. 数据、API 与兼容升级

`ai_model_profiles` 扩展 provider CHECK、凭据 CHECK、provider/task CHECK 与 Ollama 模型 CHECK；`endpoint` 继续必须为 `NULL`，`parameters` 继续为空对象。数据库必须完整表达：provider 仅 `openai/deepseek/ollama/onnx`；OpenAI/DeepSeek 分别使用固定 credential reference，Ollama/ONNX 为 `NULL`；ONNX 仅 embedding，DeepSeek 禁止 embedding；Ollama embedding 仅允许 `qwen3-embedding:0.6b`、1024 维，Ollama 所有 profile 的 `model_version` 必须是 64 位小写 hex，Ollama 非 embedding 仍允许其他显式模型名。模型档案 Create DTO/OpenAPI provider enum 同步加入 `deepseek`、`ollama`，但响应仍不暴露 credential reference 或 endpoint。

既有数据库通过 Operations-007 在维护窗口仅替换 provider、credential、provider/task 和 Ollama model-version/model-name CHECK；不改列、不回填、不删除历史 profile/run/vector。升级前备份并停止写入，升级后运行 `hotkey db verify`。任何代码或约束回退之前先对仍运行目标版本的数据库做只读 preflight，证明不存在 `deepseek`/`ollama` profile；非零即停止而不是删除数据，只有零记录才允许停服并恢复旧约束和代码。

## 8. 验证与运行边界

依赖固定为 `github.com/tmc/langchaingo@v0.1.14`；升级版本必须重新执行协议 fixture。该版本的原生 Ollama adapter 使用 `/api/chat` 和 `/api/embed`，Embedding 对每个输入发出一次请求并保持输入顺序，且默认不 pull 模型。其 Embedding 抽象只返回向量、不暴露 `/api/embed` 的 `prompt_eval_count`，所以生成 usage 必须完整保存，Embedding usage 明确为零值且由 fixture 冻结；不能估算或伪造 token 数。零密钥 fixture 必须覆盖这些路径、批量顺序、DeepSeek Chat Completions、Ollama digest 匹配/缺失/漂移、1024 维 Qwen、生成 usage、Embedding 零 usage、429、5xx、deadline、非法 JSON和错误正文脱敏；请求计数必须证明每次生成只调用一次、Embedding 每个输入只调用一次，429/5xx 不发生 SDK 内部重试。运行连接验证分为配置/fixture 门禁与部署实机探测；没有本机 Ollama 或模型时，Acceptance 必须记录为未执行风险，不能伪造实机通过。

## 9. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 建立 LangChainGo、DeepSeek、Ollama 与 Qwen3 1024 维 Embedding 边界，等待独立审核。 |
| v0.2 | 2026-07-18 | 按独立审核补齐凭据映射、Ollama digest、数据库矩阵、依赖兼容性和回退前置检查。 |
| v1.0 | 2026-07-18 | 非主要编写者复核 v0.2 无剩余阻塞项，设计 accepted。 |
| v1.1 | 2026-07-18 | 数据库红灯发现 digest 带前缀超过 varchar(64)，改为保存去前缀的 64 位小写 hex，重新审核。 |
| v1.2 | 2026-07-18 | 非主要编写者复核 digest 规范与既有列契约一致，结论 APPROVED。 |
| v1.3 | 2026-07-18 | 明确固定 LangChainGo 版本不暴露 Ollama Embedding token usage，禁止估算并以零值契约验收。 |
| v1.4 | 2026-07-18 | 实施与独立最终复审完成，移入长期设计归档。 |
