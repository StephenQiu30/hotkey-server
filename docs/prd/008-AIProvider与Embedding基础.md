---
layer: PRD
prd_no: "008"
doc_no: "008"
title: AI Provider与Embedding基础
audience: [PM, Dev, QA, Ops]
feature_area: AI运行基础
purpose: 定义首批可替换 AI Provider、1024 维向量、模型运行审计和安全降级的可验收范围
phase: P0
priority: P0
status: accepted
execution_status: ready
version: v1.5
owner: HotKey Server Team
depends_on: [PRD-002, PRD-007]
design_refs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/007-多语言匹配与相关性设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
canonical_path: docs/prd/008-AIProvider与Embedding基础.md
inputs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/007-多语言匹配与相关性设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
outputs:
  - intelligence 模块的 Provider、模型配置、运行和向量基础
triggers:
  - Provider、模型、向量空间、Schema、预算或回退契约变化
downstream:
  - docs/plans/008-AIProvider与Embedding基础计划.md
  - docs/operations/plan008-schema-upgrade.md
  - docs/acceptance/008-AIProvider与Embedding基础验收.md
---

# AI Provider 与 Embedding 基础

## 目标

建立可替换、可限额、可审计的 AI 基础能力，让后续 PLAN-009 可从稳定 Application 端口取得 1024 维多语言向量和受约束的扩词结果；采集、Content 查询和非 AI 规则链路在没有可用 AI 模型时继续工作。

## 首版范围与固定决策

1. `ai_model_profiles.task_type` 的完整 Schema 与 Application 都只接受 `embedding` 和 `term_expansion` 两个任务类型。事件摘要、相关性复核、聚类、实体、Claim、知识提案、报告和任务调度均不属于本 PRD；它们必须由拥有 PRD 先扩展 Schema、静态 Schema 和测试后才可创建 profile。
2. 第一个远程适配器是官方 `github.com/openai/openai-go/v3@v3.32.0`。生产请求只使用官方默认 HTTPS 端点；本任务不接受、保存或暴露自定义 Provider URL。
3. 模型名不是代码默认值。管理员创建 profile 时显式提供 `model_name` 与不可变 `model_version`；Embedding profile 必须声明 `embedding_dimensions=1024`。对 OpenAI，`model_name` 是唯一写入官方 SDK `model` 请求字段、且必须与 SDK 响应 `model` 严格相等的 provider model ID；不相等即安全失败 `70000 ai_model_profile_invalid`。`model_version` 是本地不可变的语义/reuse 元数据，绝不伪造为 SDK 请求字段；adapter 仅在 model ID 已验证后将原请求的 `model_version` 回传给 Application。实际 Provider 返回的向量长度、每个元素有限性和 profile 维度均须验证。
4. ONNX 只实现为 `onnx && cgo` build tag 下的可选本地 Embedding adapter，使用 `github.com/yalue/onnxruntime_go@v1.31.0`、`CGO_ENABLED=1`、`HOTKEY_ONNX_RUNTIME_LIBRARY`、`HOTKEY_ONNX_MODEL_PATH`、`HOTKEY_ONNX_TOKENIZER_PATH` 和 `HOTKEY_ONNX_MANIFEST_PATH`。manifest 固定 model/tokenizer SHA-256、profile model version、1024 dimensions、输入 tensor 名、`cls_l2` pooling、NFC 规范化和最大 token 数；adapter 校验四个 artifact 后才推理。默认构建或 `-tags=onnx` 但 `CGO_ENABLED=0` 时均不链接原生库；缺少 tag、CGO、库、模型、tokenizer 或 manifest 时只返回安全的“模型不可用”降级，不阻塞主链路。
5. 凭据只支持 write-only 数据库引用 `env:OPENAI_API_KEY`。它只能由 `config.AI.OpenAIAPIKey` 解析；不得直接读取任意环境变量、不得返回该引用、不得记录 key、Prompt、完整 Content、Provider 原始响应或对象存储键。`.env` 是默认环境，`.env.prod` 仅在 `HOTKEY_ENV=production` 时加载。
6. AI 请求和响应对象不写入 MinIO。本任务保留既有 `ai_runs.request_object_key` 与 `response_object_key` 为 `NULL`，只保存版本、哈希、受限结构化结果、稳定错误码、用量与耗时；后续要保存受控 payload 必须另立 PRD/Plan。
7. Provider SDK、ONNX 类型和 HTTP 响应类型不得越过 Infrastructure/Transport 边界。Domain/Application 只使用本模块定义的值对象和端口。

## 功能要求

### 1. Provider、Schema 与安全结果

- `Provider` 端口必须分别接收最小化的 `EmbeddingRequest` 与 `StructuredRequest`，并返回不带 SDK 类型的 `EmbeddingResponse` 与 `StructuredResponse`。
- 任务、输入和输出 Schema 均为仓库内静态版本化 JSON Schema。首版必须提供 `embedding-input-v1`、`embedding-output-v1`、`term-expansion-input-v1`、`term-expansion-output-v1`；Schema 使用 `additionalProperties: false`，限制数组数量、字符串长度、语言标签和枚举。
- Application 在调用前校验输入，调用后校验输出。输出校验失败时仅把安全的字段级错误发送给 Provider 做一次结构化修复；第二次失败使用 `70006 ai_output_invalid`，不得写入业务事实。
- Provider outcome 映射为稳定业务码：`70000 ai_model_profile_invalid` (400)、`70001 ai_model_unavailable` (503, retryable)、`70002 ai_budget_exhausted` (429, retryable)、`70003 ai_provider_rate_limited` (429, retryable)、`70004 ai_provider_transient` (502, retryable)、`70005 ai_provider_timeout` (504, retryable)、`70006 ai_output_invalid` (502)、`70007 ai_run_in_progress` (409, retryable)、`70008 ai_embedding_invalid` (400) 和 `70009 ai_run_lease_expired` (503, retryable)。HTTP 消费者只依据 numeric `code`，不依赖 Provider 原文。
- `StructuredRequest` 除 task/model/version/input 外必须携带从静态资源编译出的 JSON Schema 与同版本 task instruction；repair 还必须携带第一次 output 和有限的 field-path violations。Provider 只收到这些受控值，不能自行推断任意 schema/prompt。

### 2. 模型 Profile 与管理员 API

- `ai_model_profiles` 是业务配置。`provider` 仅允许 `openai` 和 `onnx`；`task_type` 首版仅允许 `embedding` 和 `term_expansion`；ONNX 只能用于 embedding，OpenAI 可用于两个任务。
- `provider`、`model_name`、`model_version`、`credential_ref`、`embedding_dimensions` 和任务类型在创建后不可修改；语义变化必须 archive 原 profile 并创建新 profile，不能覆写旧向量空间或旧运行的出处。可修改项只有 `enabled`、`timeout_seconds`、`max_attempts`、`max_cost`、`daily_budget`、`fallback_priority`，且使用业务 `version` 乐观锁。
- 管理 API 是 `/api/v1/ai/model-profiles` 的管理员专用 CRUD：`GET /`、`GET /:id`、`POST /`、`PATCH /:id`、`DELETE /:id`、`POST /:id/restore`。仅 Create 请求可接受 write-only `credential_ref`；PATCH 出现该字段必须返回 `70000 ai_model_profile_invalid`，不得静默忽略或旋转凭据。任何响应、审计详情、日志、指标和 OpenAPI example 均不得包含 `credential_ref`、API key、endpoint 或原始参数。
- profile 选择顺序固定为：任务类型匹配、未删除、enabled、credentials/build 可用、单次和当日预算可预留、`fallback_priority ASC, id ASC`。没有候选时返回可观测的降级结果而非 panic 或隐式默认模型。

### 3. 运行复用、重试与预算

- `reuse_key` 必须是 `sha256(task_type, target_type, target_id, model_profile_id, model_profile_version, model_version, prompt_version, input_schema_version, schema_version, parameters_version, input_hash, evidence_set_hash)` 的稳定序列化结果。
- 只有 `succeeded` 且所有上述版本仍匹配的运行可复用。profile 语义变更只能新建 profile；输入、证据、Prompt、输入/输出 Schema、parameters 或 model version 任一变化均创建新运行。
- 对相同 `reuse_key` 的**所有** claim、retry、settle、cancel 与 reclaim 事务，一律先取得 `pg_advisory_xact_lock(hashtext('ai-budget:' || profile_id || ':' || utc_day))`，再取得 `pg_advisory_xact_lock(hashtext('ai-run:' || reuse_key))`；不得存在反向路径。claim 再检查成功运行和 in-flight 运行。成功运行直接返回；in-flight 返回 `70007`，绝不第二次调用 Provider；否则 reserve 并创建 queued 运行，在提交后调用 Provider。
- 状态只能按 `queued -> running -> validating -> succeeded`、`running|validating -> retry_wait -> running`、`queued|running|validating|retry_wait -> failed|cancelled` 转换。429、超时和临时 5xx 在 `max_attempts` 内以 `min(2^(attempt-1), 4)` 秒确定性退避；配置、预算、输入、Schema 和证据错误不重试。创建 queued、`queued -> running`、`running -> validating` 与 `retry_wait -> running` 都在同一锁定事务将 lease 刷新为 `now + timeout_seconds + 30s`；`running|validating -> retry_wait` 原子地写入 `retry_after=now+backoff` 与 `lease_expires_at=retry_after+timeout_seconds+30s`。`attempt`、`retry_after`、`lease_expires_at`、`error_code` 和 `repair_attempted` 必须持久化。
- 每个 profile 必须有正数 `max_cost`，它既是每次调用的硬预留额度也是 Provider 请求的输出/token 上限来源；`daily_budget` 可以为 NULL，表示没有每日上限但仍记账。每个 profile+UTC 日 ledger 还有 `overage_blocked=false` 初始状态；reserve 前置条件固定为 `overage_blocked=false` 且（daily 为空或 `settled_cost + reserved_cost + max_cost <= daily_budget`）。reserve 令 `reserved_cost += max_cost`，失败/取消/lease 回收令 `reserved_cost -= reserved_cost_of_run`，成功令 `reserved_cost -= reserved_cost_of_run` 且 `settled_cost += actual_cost`。
- 实际 cost 大于预留时不得伪造更小数值：运行以 `failed/70002` 终止，记录真实 `cost=actual_cost`，释放预留、将真实成本加入 `settled_cost` 并原子写 `overage_blocked=true`。这会封锁**该 profile 的该 UTC 日**后续 reserve，即使 `daily_budget` 为 NULL 或仍有余额；新 UTC 日创建新的 `overage_blocked=false` ledger，是唯一自动重置规则。因此 `ai_runs.cost` 允许大于 `reserved_cost`。预算不足不调用 Provider。并发请求不得突破预留上限或重复调用。
- queued、running、validating 与 retry_wait 运行都拥有 `lease_expires_at`。worker-only `RunLeaseReclaimer` 每 30 秒按同一固定 `ai-budget(profile,day) -> ai-run(reuse_key)` 锁顺序回收过期运行：标记 `failed`、写 `70009 ai_run_lease_expired` 并释放该运行 reservation；不会重放外呼。测试必须证明未到期的 retry_wait 不会被回收，而进程崩溃后过期的 retry_wait 会被回收。下一次请求可获得新 claim，进程崩溃不能永久产生 `70007` 或冻结预算。

### 4. 向量空间与查询

- 每个 Content、Monitor、Event、Topic 向量记录 profile、profile version、model version、输入哈希、`halfvec(1024)` 和 `active`。长度不是 1024、存在 `NaN`/`Inf`、已验证的 Provider model ID 与 `model_name` 不同，或 Application 回传的本地 `model_version` 与 profile 不同的向量必须在写库前以 `70000` 拒绝。
- 写入新版本在一个 PostgreSQL 事务内取得 `ai-embedding:<target-type>:<target-id>:<profile-id>` advisory lock，先停用同 target/profile 的旧 active 向量再插入新行；每张 embedding 表使用 partial unique index 保证同 target/profile 至多一条 active 行。
- 近邻查询必须同时过滤 `active=true`、`model_profile_id` 和 `model_version`，使用余弦 `<=>`，且只将同一 profile/version 的结果交给下游。HNSW 只服务 active 集；验收须以 `EXPLAIN (COSTS OFF)` 证明 active 查询使用对应 `*_active_hnsw_idx`。
- 模型升级不是 update。管理员先创建新 profile，后续 PLAN-009/010 以新 profile 批量重算并在完整覆盖后切换读取 profile；PLAN-008 不批量重算 Event/Topic，也不让新旧空间混合检索。

## 非范围

- 不默认启用真实模型、不提交 API key、不调用用户未配置的远程服务。
- 不引入任意环境变量 SecretResolver、自定义 Provider endpoint、独立向量数据库、在线训练、自动 Prompt 优化或 Prompt 管理 UI。
- 不写入 Event 摘要、Claim、Vault、报告、调度 Job、跨语言匹配评分或用户反馈事实。
- 不把 AI 可用性作为 ingestion、Content 查询、规则匹配、基础聚类或热度计算的前提。

## 交付物

- `intelligence` 模块的 Profile、Provider、运行、Schema、Embedding 与 PostgreSQL Repository。
- OpenAI SDK 适配器、默认构建的 ONNX unavailable adapter 与 `onnx` tag 下的本地 adapter。
- 管理员 profile API、统一错误码、OpenAPI、无密钥 fixture 和安全日志/指标。
- 完整 `db/schema.sql`、记录模型、PLAN-008 既有库升级/回退手册和长期 Acceptance 模板。

## 验收标准

1. 用零真实密钥的 `httptest` Provider fixture 验证成功、429、5xx、deadline、非法 JSON、一次修复成功和修复失败；fixture 断言 adapter 收到静态 instruction/JSON Schema 与受限 repair violations，SDK/原生错误不出现在 Result、日志或指标标签。
2. 同一个 `reuse_key` 的并发请求只产生一次 Provider 调用；成功运行可复用，任何版本或证据哈希变化都不能复用。
3. 并发预算预留满足 `overage_blocked=false` 且（daily 为空或 `settled + reserved + max <= daily`），失败释放、成功精确结算；overage 记录真实成本并封锁同 profile+UTC 日后续 reserve，即使日预算为空或仍有余额。崩溃/lease expiry 被 worker 回收，未到期的 retry 不得被回收。预算不足和无可用 Provider 均不影响 ingestion 或 Content read API。
4. 1024 个有限值可以写入并经 HNSW 查回；1023/1025、`NaN`、`Inf`、不同 model version 和被停用的向量均不能混入结果。
5. 默认 `go test ./...` 和 `CGO_ENABLED=0 go test -tags=onnx ./...` 都不需要 ONNX 原生库；`onnx && cgo` matrix 分别验证缺 runtime、模型、tokenizer、manifest 的安全降级，以及已校验完整 bundle 的 1024 输出。
6. API 认证、管理员授权、write-only `credential_ref`、乐观锁、软删除/恢复、Result 和 OpenAPI 全部有契约测试。
7. PLAN-007 数据库可在备份、preflight、受控升级、`hotkey db verify` 和精确 `pg_restore` 回退后恢复；不得通过 `DROP SCHEMA`、运行时 DDL 或全量重置实现。

## 完成定义

PRD-009 可以只依赖 `intelligence` 的公开 Application 端口取得同一 1024 维向量空间及 `term_expansion` 结果；实现不暴露 Provider SDK、密钥、Prompt、原始响应或旧向量空间，并已由 Acceptance-008 保存可复现证据。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-15 | 建立 AI Provider、Embedding、复用、预算与降级范围。 |
| v1.1 | 2026-07-17 | 补齐首个 SDK/ONNX 构建、最小凭据边界、稳定错误码、profile API、并发复用/预算、向量隔离、升级回退和可执行验收契约。 |
| v1.2 | 2026-07-17 | 收紧任务类型；补齐 static structured request、ONNX bundle、max/daily/overage ledger 方程、worker lease 回收与固定历史 verifier。 |
| v1.3 | 2026-07-17 | 统一 budget→run 锁序，定义 profile+UTC-day overage 封账、重试 lease 刷新和 `onnx && cgo` 双向 build tag。 |
| v1.4 | 2026-07-17 | 收紧 credential_ref 为创建时只写；明确 OpenAI 的 model ID 校验与本地 model_version 元数据边界。 |
| v1.5 | 2026-07-17 | 独立复审通过，状态提升为 accepted/ready。 |
