---
layer: Plan
doc_no: "008"
audience: [Dev, QA, Ops]
feature_area: AI运行基础
purpose: 以可独立验证和提交的任务实施 AI Provider、模型运行与 1024 维 Embedding 基础
canonical_path: docs/plans/008-AIProvider与Embedding基础计划.md
status: review
execution_status: backlog
review_status: pending
version: v1.2
owner: HotKey Server Team
inputs:
  - docs/prd/008-AIProvider与Embedding基础.md
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/007-多语言匹配与相关性设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
  - docs/operations/plan007-schema-upgrade.md
outputs:
  - intelligence 运行基础
  - 1024 维 Embedding 存储与检索
  - 可恢复的 PLAN-008 Schema 升级与验收证据
triggers:
  - PRD-008 accepted 且 ready
downstream:
  - docs/acceptance/008-AIProvider与Embedding基础验收.md
depends_on: [PLAN-002, PLAN-007]
---

# AI Provider 与 Embedding 基础执行计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付不泄露密钥或 Provider 类型、能安全降级的模型 profile、运行复用、预算与 1024 维向量基础。

**Architecture:** `intelligence/domain` 定义不依赖第三方的 Provider/Repository 值对象和稳定错误；Application 负责静态 Schema、profile 选择、事务性预算/复用与降级；PostgreSQL、OpenAI SDK 和 ONNX 均留在 Infrastructure。Profile 的语义字段不可变，模型升级创建新 profile；向量读取以 profile+model version 隔离，并不在本计划实施匹配或事件任务。

**Tech Stack:** Go 1.26、Gin、Fx、Viper、PostgreSQL 16+/pgvector、pgx v5、`github.com/openai/openai-go/v3@v3.32.0`、`github.com/santhosh-tekuri/jsonschema/v6@v6.0.2`；可选 `onnx` tag 下的 `github.com/yalue/onnxruntime_go@v1.31.0`。

## 全局约束

- 仅 `.env` 与 `.env.prod`；新增配置只能通过 `config.Config` 和 `configKeys()` 读取。不得读取任意环境变量，也不得新增 config 文件或 Secret 服务。
- 仅 `embedding`、`term_expansion`；不实现 PLAN-009 相关性评分或 PLAN-012 事件摘要/Claim。
- OpenAI production adapter 不接受 endpoint；profile 的 `credential_ref` 仅 `env:OPENAI_API_KEY`，永不出现在响应、OpenAPI example、日志、指标或审计详情。
- 默认构建不需要 ONNX Runtime 或模型。原生 adapter 仅在 `onnx && cgo` 且 `CGO_ENABLED=1`、`HOTKEY_ONNX_RUNTIME_LIBRARY`、`HOTKEY_ONNX_MODEL_PATH`、`HOTKEY_ONNX_TOKENIZER_PATH` 与 `HOTKEY_ONNX_MANIFEST_PATH` 都设置且 manifest hash 校验通过时可用；`-tags=onnx` 但无 CGO 必须仍命中 safe unavailable adapter。
- `ai_runs` 不存 Prompt、完整 Content、Provider 原始请求/响应或对象键；既有对象键列在本计划始终为 `NULL`。
- `db/schema.sql` 是唯一执行 Schema。既有库升级仅使用 `docs/operations/plan008-schema-upgrade.md`，先备份和演练；绝不运行时 DDL、`DROP SCHEMA` 或不受控 reset。
- 任何 API 变动都同步 DTO、权限、bootstrap、`docs/openapi/swagger.json` 和 `make openapi-check`；错误必须使用 70000–79999 的稳定 numeric code。
- 每个任务完成后运行本任务 GREEN、`git diff --check`，只暂存该任务文件并提交。`make ci` 之后必须 `make clean`。

## 开工条件

- 本 Plan `status: accepted`、`review_status: approved`、`execution_status: ready`。
- PRD-008 `status: accepted`、`execution_status: ready`；Design-003、Design-007、Design-011 的本次 AI/向量契约均已独立复核为 accepted。
- PLAN-002、PLAN-007 为 done，工作区干净，且从当时 `main` 创建/同步当前工作分支。
- PostgreSQL 16+ with pgvector、Redis 已可用于以下 fixture；任何命令均不得指向生产 DSN。

## 文件结构与职责

| 动作 | 路径 | 责任 |
|---|---|---|
| 修改 | `go.mod`, `go.sum` | 固定 OpenAI、JSON Schema、pgvector 和可选 ONNX 依赖版本。 |
| 修改 | `internal/platform/config/config.go`, `internal/platform/config/config_test.go`, `.env.example` | 仅解析 OpenAI key 与 ONNX runtime/model/tokenizer/manifest path，建立 write-only 凭据解析边界。 |
| 创建 | `internal/modules/intelligence/domain/{provider,profile,run,embedding,errors}.go` | Provider/Repository 端口、不可变 profile、运行/向量值对象、稳定错误。 |
| 创建 | `internal/modules/intelligence/application/{schema_registry,model_profile_service,run_service,embedding_service}.go` | 静态 Schema、profile CRUD、复用/预算/重试、向量生成与显式降级。 |
| 创建 | `internal/modules/intelligence/schemas/v1/{embedding-input,embedding-output,term-expansion-input,term-expansion-output}.schema.json`, `internal/modules/intelligence/schemas/v1/term-expansion-instruction-v1.md` | 禁止未知字段的版本化输入输出契约和结构化生成指令。 |
| 创建 | `internal/modules/intelligence/infrastructure/postgres/{model_profile_repository,run_repository,budget_ledger_repository,embedding_repository}.go` | PostgreSQL 事务、锁、profile CRUD、run/ledger/向量持久化与检索。 |
| 创建 | `internal/modules/intelligence/infrastructure/provider/{openai,onnx_disabled,onnx_enabled,onnx_manifest,onnx_tokenizer}.go` | OpenAI SDK adapter；默认 unavailable ONNX adapter；tag 下 native ONNX、manifest 和 tokenizer adapter。 |
| 创建 | `internal/modules/intelligence/transport/http/{dto,handler,routes}.go` | 管理员 profile API 与 write-only DTO。 |
| 修改 | `internal/shared/errors/error.go`, `internal/shared/errors/error_test.go` | 注册 70000–70009 的稳定错误码。 |
| 修改 | `internal/platform/database/model/{model,model_test}.go`, `db/schema.sql` | 完整 Schema、记录映射、catalog contract 和物理列顺序。 |
| 修改 | `internal/bootstrap/app.go`, `docs/openapi/swagger.json` | Fx 装配、路由和生成的 API 契约。 |
| 创建/修改 | `docs/operations/plan008-schema-upgrade.md`, `docs/acceptance/008-AIProvider与Embedding基础验收.md` | 可演练升级/回退与最终长期证据。 |

## 数据与接口契约

### Domain 端口

Task 2 创建如下不泄露 SDK 类型的最小边界；后续任务只能消费这些类型：

```go
type Provider interface {
    Embed(context.Context, EmbeddingRequest) (EmbeddingResponse, error)
    GenerateStructured(context.Context, StructuredRequest) (StructuredResponse, error)
}

type EmbeddingRequest struct {
    ModelName, ModelVersion string
    Dimensions               int
    Inputs                   []string
}
type EmbeddingResponse struct { ModelVersion string; Vectors [][]float32 }
type SchemaViolation struct { InstancePath, Keyword string }
type RepairInput struct { PreviousOutput json.RawMessage; Violations []SchemaViolation }
type StructuredRequest struct {
    ModelName, ModelVersion, TaskType, SchemaName, SchemaVersion string
    Instruction string
    Schema json.RawMessage
    Input json.RawMessage
    Repair *RepairInput
}
type StructuredResponse struct { ModelVersion string; JSON json.RawMessage }
```

`SchemaRegistry.Structured(taskType, version)` 返回嵌入的 JSON Schema 和同版本 instruction；repair 只能复制第一次结构化 output 和最多 16 个 `instancePath/keyword`，不能拼入 Provider 错误。`Embedding.Validate()` 只接受恰好 1024 个非 `NaN`、非 `Inf` 的元素。`Provider` 的错误在 Infrastructure 内转换为 Task 2 的 domain 错误，Application 从不检查 SDK HTTP/status 类型。

OpenAI adapter 只将 `EmbeddingRequest.ModelName`/`StructuredRequest.ModelName` 写入官方 SDK 的 `model` 字段；SDK 返回的 `response.model` 必须与该值精确相等，否则 adapter 返回 `CodeAIModelProfileInvalid (70000)`，不返回向量或结构化结果。`ModelVersion` 是 profile 的本地不可变语义/reuse 元数据，不写入 SDK 请求；仅在 provider model ID 校验通过后，adapter 才在无 SDK 类型的 response 中原样携带请求的 `ModelVersion`。fixture 必须断言这一点，而不是假设存在 OpenAI 的 version 参数。

### ONNX bundle

ONNX profile 只在完整 bundle 校验后可选。`HOTKEY_ONNX_MANIFEST_PATH` 指向 JSON `{"version":"v1","model_sha256":"<64 hex>","tokenizer_sha256":"<64 hex>","model_version":"<profile value>","dimensions":1024,"max_tokens":8192,"input_names":["input_ids","attention_mask","token_type_ids"],"output_name":"last_hidden_state","pooling":"cls_l2","normalization":"nfc"}`。`onnx_tokenizer.go` 只接受 manifest 指定的 HuggingFace `tokenizer.json`，执行 NFC、special token、截断至 `max_tokens`、padding 与 attention mask；`onnx_enabled.go` 检查三个 input tensors、last_hidden_state、CLS pooling、L2 normalization 和 manifest/profile version 一致。任何 artifact hash、tensor 名、pooling、维度或版本不符均为 `CodeAIModelUnavailable`，不向 ONNX Runtime 提交推理。

### Profile 与 API

`POST /api/v1/ai/model-profiles` 的管理员请求字段为 `name`、`task_type`、`provider`、`model_name`、`model_version`、`credential_ref`、`embedding_dimensions`、`timeout_seconds`、`max_attempts`、`max_cost`、`daily_budget`、`fallback_priority`、`enabled`；`credential_ref` 是仅此请求可写入的字段。`PATCH /:id` 仅接收 `version` 加可变字段；任何语义字段（包括 `credential_ref`）出现即为 `70000`。所有成功响应只包含 `id`、`version`、`name`、`task_type`、`provider`、`model_name`、`model_version`、`embedding_dimensions`、timeout/budget/fallback/enabled、timestamps 和 deleted state。

### 完整 Schema 目标

- `ai_model_profiles` 在保留当前列的基础上、且**仅在当前 `deleted_at` 之后**增加 `model_version varchar(64) NOT NULL`、`embedding_dimensions smallint`、`max_attempts smallint NOT NULL DEFAULT 1`、`max_cost numeric(12,4) NOT NULL`；task check **恰为** `embedding|term_expansion`，provider check 为 `openai|onnx`，endpoint 必为 NULL，embedding 必为 1024，max_cost/attempt 为正数，daily 为 NULL 或不小于 max，credential/provider 组合只允许 OpenAI 的 `env:OPENAI_API_KEY` 或 ONNX 的 NULL。
- `ai_runs` 删除旧自由文本 `error`，保留原有列相对顺序后、且**仅在 `finished_at` 之后**增加 `model_profile_version`、`model_version`、`parameters_version`、`input_schema_version`、`evidence_set_hash`、`reuse_key`、`attempt`、`max_attempts`、`repair_attempted`、`retry_after`、`error_code`、`budget_day`、`reserved_cost`、`lease_expires_at`；status check 使用 `queued/running/validating/retry_wait/succeeded/failed/cancelled`。`request_object_key` 和 `response_object_key` 有 `CHECK (... IS NULL)`；`cost` 记录真实结算且不得被 `reserved_cost` 限制。
- 新建 operational `ai_budget_ledgers(id, model_profile_id, budget_day, reserved_cost, settled_cost, overage_blocked, updated_at)`，对 `(model_profile_id, budget_day)` 唯一；两金额非负，`overage_blocked` 默认 false。reserve 前置条件为 `overage_blocked=false` 且（daily 为 NULL 或 `settled + reserved + max <= daily`）；success/overage 都写真实 cost 到 settled，failed/cancelled/reclaim 只释放 reservation。overage 原子置 blocked=true，封锁同 profile+UTC-day 后续 reserve，即使 daily 为 NULL 或仍有余额；只有下一 UTC 日新 ledger 自动恢复 false。
- 四张 embedding 表都在当前 `created_at` 之后增加 `model_profile_version`；每张增加 `(target_id, model_profile_id) WHERE active` partial unique index。近邻查询固定为 `WHERE active AND model_profile_id=$1 AND model_version=$2 ORDER BY embedding <=> $3::halfvec LIMIT $4`。
- 替换旧 ai_runs 普通 unique 为 `ai_runs_reuse_succeeded_uq(reuse_key) WHERE status='succeeded'` 与 `ai_runs_reuse_inflight_uq(reuse_key) WHERE status IN ('queued','running','validating','retry_wait')`。实现前必须同步记录模型和 catalog integration test；禁止为迁移重排列物理列。

## Task 1：配置、错误码、完整 Schema 与可恢复升级

**Files:**
- Modify: `go.mod`, `go.sum`, `.env.example`, `internal/platform/config/config.go`, `internal/platform/config/config_test.go`, `internal/shared/errors/error.go`, `internal/shared/errors/error_test.go`, `db/schema.sql`, `internal/platform/database/model/model.go`, `internal/platform/database/model/model_test.go`, `internal/platform/database/database_integration_test.go`
- Create: `docs/operations/plan008-schema-upgrade.md`
- Test: `internal/platform/config/config_test.go`, `internal/platform/database/database_integration_test.go`, `internal/platform/database/model/model_test.go`, `tests/architecture/schema_contract_test.go`

**Consumes:** PLAN-007 canonical catalog and `Config.Load`; **Produces:** fixed dependency/config/error/schema contract for every later task.

- [ ] **Step 1: Write failing configuration and catalog tests.** Assert `HOTKEY_OPENAI_API_KEY`, all four ONNX artifact keys and no generic `HOTKEY_LLM_*` are bound by `configKeys()`/`.env`/`.env.prod`. Assert 70000–70009 status/retryability. Extend schema tests to reject every task type except `embedding|term_expansion`, require non-null positive max_cost, NULL-or-at-least-max daily budget, ledger `overage_blocked`, lease, no `ai_runs.error`, four partial unique indexes and exact physical add-only order.

- [ ] **Step 2: Run RED.**

  Run: `go test ./internal/platform/config ./internal/shared/errors ./internal/platform/database ./internal/platform/database/model ./tests/architecture -count=1`

  Expected: FAIL because AI config keys, AI code catalog, ledger/constraints and target catalog are absent.

- [ ] **Step 3: Implement the smallest complete contract.** Add `AIConfig` to `config.Config`, bind OpenAI plus four ONNX artifact keys, remove unbound `HOTKEY_LLM_API_KEY`, `HOTKEY_LLM_BASE_URL`, `HOTKEY_LLM_MODEL`, and leave runtime AI credentials optional. Register ten codes in `shared/errors`. Update full Schema/record metadata exactly as “完整 Schema 目标” specifies; add no migration runtime. Pin OpenAI `v3.32.0`, JSON Schema `v6.0.2`, pgvector `v0.4.0`, and ONNX `v1.31.0` in `go.mod`.

- [ ] **Step 4: Write the upgrade/rollback runbook and its real integration rehearsal.** The Operations document must pin PLAN-007 baseline `53d7f01`, create a disposable detached worktree from it, run its `db init`/verifier, and retain its custom `pg_dump`. It must require every existing AI table count to be zero, then run one `psql -v ON_ERROR_STOP=1` transaction that drops only the old ai_runs unique, drops `ai_runs.error`, adds columns in physical order, creates ledger/indexes/constraints, runs current `db verify`, and asserts empty counts. Rollback stops services, first proves unprepared `pg_restore --single-transaction` fails atomically, drops only PLAN-008 indexes/ledger, restores the custom backup in one transaction, then invokes `go -C "$PLAN007_WORKTREE" run ./cmd/hotkey db verify`. The integration test must invoke this exact detached-worktree verifier, not recreate a hand-built legacy Schema.

- [ ] **Step 5: Run GREEN.**

  Run: `HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' go test -tags=integration ./internal/platform/database ./tests/architecture -count=1`

  Expected: PASS; it exercises PLAN-007 backup -> exact PLAN-008 upgrade -> current verifier -> prepared restore -> PLAN-007 verifier. Then run `go test ./internal/platform/config ./internal/shared/errors ./internal/platform/database/model -count=1` and `make schema-verify` successfully.

- [ ] **Step 6: Commit.**

  ```bash
  git add go.mod go.sum .env.example internal/platform/config internal/shared/errors \
    internal/platform/database/model internal/platform/database db/schema.sql \
    docs/operations/plan008-schema-upgrade.md tests/architecture
  git commit -m "feat: define AI runtime schema and configuration"
  ```

## Task 2：领域值对象、静态 Schema 与可控错误分类

**Files:**
- Create: `internal/modules/intelligence/domain/{provider,profile,run,embedding,errors}.go`, `internal/modules/intelligence/domain/{provider,profile,run,embedding,errors}_test.go`
- Create: `internal/modules/intelligence/application/schema_registry.go`, `internal/modules/intelligence/application/schema_registry_test.go`
- Create: `internal/modules/intelligence/schemas/v1/{embedding-input,embedding-output,term-expansion-input,term-expansion-output}.schema.json`, `internal/modules/intelligence/schemas/v1/term-expansion-instruction-v1.md`

**Consumes:** Task 1 codes/config; **Produces:** provider-neutral values and the static schemas consumed by repositories/adapters/services.

- [ ] **Step 1: Write failing unit tests.** Cover the two allowed task types and rejection of every future type, `env:OPENAI_API_KEY` as the only valid OpenAI reference, NULL as only valid ONNX reference, non-null max_cost, immutable semantic fields, 1024 finite-vector validation, exact reuse-key canonicalization, malformed/unknown/oversized JSON, static instruction/schema delivery, and bounded repair. Include one repaired output and a second invalid output yielding `CodeAIOutputInvalid`.

- [ ] **Step 2: Run RED.**

  Run: `go test ./internal/modules/intelligence/domain ./internal/modules/intelligence/application -count=1`

  Expected: FAIL because the domain package and Schema registry do not exist.

- [ ] **Step 3: Implement smallest values and schemas.** Implement the Domain interface shown above, task/profile validation, canonical JSON hashing, and error classification. Embed four JSON files and `term-expansion-instruction-v1.md` with `go:embed`; compile schemas once with `jsonschema/v6`; require `additionalProperties:false`, max 32 expanded terms, max 120 characters per term, `zh|en|und` language enum, and no raw evidence/object fields. The repair API accepts only schema error paths and the original structured JSON, not Provider errors, prompt or secret.

- [ ] **Step 4: Run GREEN.**

  Run: `go test ./internal/modules/intelligence/domain ./internal/modules/intelligence/application -count=1`

  Expected: PASS; 1023/1025, `NaN`, `Inf`, unknown JSON fields and the second repair are rejected deterministically.

- [ ] **Step 5: Commit.**

  ```bash
  git add internal/modules/intelligence/domain internal/modules/intelligence/application/schema_registry.go \
    internal/modules/intelligence/application/schema_registry_test.go internal/modules/intelligence/schemas
  git commit -m "feat: add AI provider domain contracts"
  ```

## Task 3：PostgreSQL Profile、运行复用、预算与向量 Repository

**Files:**
- Create: `internal/modules/intelligence/infrastructure/postgres/{model_profile_repository,run_repository,budget_ledger_repository,embedding_repository}.go`
- Create: `internal/modules/intelligence/infrastructure/postgres/{model_profile_repository,run_repository,budget_ledger_repository,embedding_repository}_test.go`
- Create: `internal/modules/intelligence/infrastructure/postgres/repository_integration_test.go`

**Consumes:** Tasks 1–2 schema/domain; **Produces:** transaction-safe persistence ports for Tasks 4–6.

- [ ] **Step 1: Write failing PostgreSQL integration tests.** In one disposable database, assert profile task/max/daily constraints, admin optimistic conflict/soft delete/restore, semantic-field rejection, success-only reuse, one in-flight row for concurrent same key, `overage_blocked=false` plus `settled+reserved+max` daily reserve, release, real overage recording and later reserve rejection. Cover both a NULL daily budget and a daily budget with remaining balance: both must reject after overage, then a next-UTC-day ledger must allow reserve. Simulate process death at queued/running/retry_wait and assert worker reclaimer marks 70009/release in `ai-budget -> ai-run` order. With a controllable clock, assert a valid retry_wait before its refreshed lease is not reclaimed while a crashed retry_wait after its lease is reclaimed. Assert each target’s atomic deactivate/insert and HNSW `EXPLAIN (COSTS OFF)` after `SET LOCAL enable_seqscan = off`.

- [ ] **Step 2: Run RED.**

  Run: `HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' go test -tags=integration ./internal/modules/intelligence/infrastructure/postgres -count=1`

  Expected: FAIL because no intelligence PostgreSQL Repository exists.

- [ ] **Step 3: Implement transactions, never Provider calls.** Every claim, retry, settle, cancellation and reclaim takes `ai-budget:<profile>:<UTC-day>` before `ai-run:<reuse_key>`; no inverse lock path exists. In that transaction reclaim expired runs for the profile/day, reject `overage_blocked` ledgers, reserve by the stated equation, then create queued with `lease_expires_at=now+timeout+30s`. Atomically refresh the lease on queued→running, running→validating and retry_wait→running; for running|validating→retry_wait write `retry_after=now+min(2^(attempt-1),4)s` and `lease_expires_at=retry_after+timeout+30s`. A worker reclaimer uses the same lock order every 30 seconds, marks only expired in-flight rows `failed/70009`, and releases their reservation without calling Provider. Commit claim/reserve before Application network work. For each embedding table use its target/profile lock, deactivate then insert. Repository methods accept caller transactions and never query unrelated module tables.

- [ ] **Step 4: Run GREEN.**

  Run: `HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' go test -race -tags=integration ./internal/modules/intelligence/infrastructure/postgres -count=1`

  Expected: PASS; at most one Provider work claim and no budget/vector invariant violation under the deterministic interleavings.

- [ ] **Step 5: Commit.**

  ```bash
  git add internal/modules/intelligence/infrastructure/postgres
  git commit -m "feat: persist AI runs budgets and embeddings"
  ```

## Task 4：OpenAI 与可选 ONNX Infrastructure 适配器

**Files:**
- Create: `internal/modules/intelligence/infrastructure/provider/openai.go`, `internal/modules/intelligence/infrastructure/provider/openai_test.go`
- Create: `internal/modules/intelligence/infrastructure/provider/onnx_disabled.go`, `internal/modules/intelligence/infrastructure/provider/onnx_enabled.go`, `internal/modules/intelligence/infrastructure/provider/onnx_manifest.go`, `internal/modules/intelligence/infrastructure/provider/onnx_tokenizer.go`, `internal/modules/intelligence/infrastructure/provider/onnx_test.go`
- Modify: `internal/platform/config/config.go`, `internal/platform/config/config_test.go`

**Consumes:** Task 2 Provider port and Task 1 AIConfig; **Produces:** selectable OpenAI and build-gated ONNX providers without SDK leakage.

- [ ] **Step 1: Write failing fixture tests.** Use `httptest.Server` plus SDK test-only base URL and dummy key; assert OpenAI receives the exact `model=ModelName` (and embedding dimensions where supported), never serializes local `ModelVersion`, and receives the exact static instruction/JSON Schema/repair payload, while logs never contain a key. Return matching and mismatched SDK `response.model`: matching returns the local request `ModelVersion`; mismatch must be `70000` with no result. Simulate timeout, 429, 503 and invalid structured JSON. In default build assert ONNX unavailable without native loading. In `onnx && cgo`, separately assert missing runtime, model, tokenizer, manifest, wrong SHA, wrong tensor names, wrong pooling and profile-version mismatch all fail before inference; a complete fixture bundle produces 1024 finite normalized values.

- [ ] **Step 2: Run RED.**

  Run: `go test ./internal/modules/intelligence/infrastructure/provider -count=1`

  Expected: FAIL because neither provider adapter exists.

- [ ] **Step 3: Implement adapters.** Construct OpenAI clients only with the resolved key and official `openai-go/v3`; use `StructuredRequest.Schema`, `Instruction` and bounded `Repair` for Responses structured output; convert SDK errors at this boundary and discard raw bodies. `onnx_disabled.go` has `//go:build !onnx || !cgo`; `onnx_enabled.go` has `//go:build onnx && cgo`. The enabled adapter loads/validates the explicit manifest/model/tokenizer/runtime bundle, creates `input_ids`/`attention_mask`/`token_type_ids`, uses `last_hidden_state` CLS then L2 normalization, and only supports embedding. No adapter writes MinIO, SQL or HTTP Result values.

- [ ] **Step 4: Run GREEN and optional matrix.**

  Run: `go test ./internal/modules/intelligence/infrastructure/provider -count=1`

  Expected: PASS with no real credential, network call or ONNX native dependency. On a disposable host with both native inputs installed, run:

  ```bash
  CGO_ENABLED=1 HOTKEY_ONNX_RUNTIME_LIBRARY=/opt/onnxruntime/lib/libonnxruntime.dylib \
  HOTKEY_ONNX_MODEL_PATH=/secure-fixtures/bge-m3-1024.onnx \
  HOTKEY_ONNX_TOKENIZER_PATH=/secure-fixtures/tokenizer.json \
  HOTKEY_ONNX_MANIFEST_PATH=/secure-fixtures/manifest.json \
  go test -tags=onnx ./internal/modules/intelligence/infrastructure/provider \
    -run TestONNXProvider -count=1
  ```

  Expected optional result: PASS with 1024 finite elements; otherwise the default suite remains the supported CI path.

  The tag-only negative matrix is also mandatory on that host; every command must PASS by asserting the named safe unavailable result before inference:

  ```bash
  CGO_ENABLED=1 go test -tags=onnx ./internal/modules/intelligence/infrastructure/provider \
    -run 'TestONNXProviderRejects(MissingRuntime|MissingModel|MissingTokenizer|MissingManifest)' -count=1
  CGO_ENABLED=1 HOTKEY_ONNX_RUNTIME_LIBRARY=/opt/onnxruntime/lib/libonnxruntime.dylib \
  HOTKEY_ONNX_MODEL_PATH=/secure-fixtures/bge-m3-1024.onnx \
  HOTKEY_ONNX_TOKENIZER_PATH=/secure-fixtures/tokenizer.json \
  HOTKEY_ONNX_MANIFEST_PATH=/secure-fixtures/manifest-wrong-sha.json \
  go test -tags=onnx ./internal/modules/intelligence/infrastructure/provider \
    -run TestONNXProviderRejectsManifestContract -count=1
  ```

  Expected: each test passes only because adapter construction returns `CodeAIModelUnavailable`; no inference session is created in these cases.

  The no-CGO tagged command is mandatory in ordinary CI and must select the same unavailable adapter without compiling or loading native code:

  ```bash
  CGO_ENABLED=0 go test -tags=onnx ./internal/modules/intelligence/infrastructure/provider \
    -run TestONNXProviderUnavailableWithoutCGO -count=1
  ```

  Expected: PASS only because `onnx_disabled.go` is selected; no native headers, library, model or tokenizer are required.

- [ ] **Step 5: Commit.**

  ```bash
  git add go.mod go.sum internal/platform/config internal/modules/intelligence/infrastructure/provider
  git commit -m "feat: add AI provider adapters"
  ```

## Task 5：Application 编排、重试、降级与 bootstrap

**Files:**
- Create: `internal/modules/intelligence/application/{model_profile_service,run_service,embedding_service}.go`
- Create: `internal/modules/intelligence/application/{model_profile_service,run_service,embedding_service}_test.go`, `internal/modules/intelligence/application/service_integration_test.go`
- Modify: `internal/bootstrap/app.go`

**Consumes:** Tasks 2–4 ports/adapters; **Produces:** public Application service used by PLAN-009 and HTTP transport.

- [ ] **Step 1: Write failing tests.** Cover selection ordering, absent config/provider degradation, identical concurrent requests, success reuse, model/schema/input/evidence invalidation, 429/5xx/deadline retry count/backoff, no retry for configuration/budget/schema errors, one repair, exact budget settle/overage, and successful response write. Use a controllable clock to prove retry_wait refreshes its lease through `retry_after+timeout+30s`, cannot be reclaimed before it expires, and is reclaimed after a simulated process crash. Assert capture/Content query services remain available.

- [ ] **Step 2: Run RED.**

  Run: `go test ./internal/modules/intelligence/application -count=1`

  Expected: FAIL because no orchestration service or Fx wiring exists.

- [ ] **Step 3: Implement smallest orchestration.** `RunService` obtains a DB claim/lease before network work; it reserves, calls one provider, validates/repairs once, settles/releases or records exact overage plus profile+UTC-day `overage_blocked`, and writes only safe structured results. Every state transition that remains in-flight calls the Task 3 atomic lease refresh rule; it never invents a second lock order. `RunLeaseReclaimer` is a worker-only Fx lifecycle goroutine with a 30-second ticker; it calls the Task 3 reclaimer but never replays Provider calls. It returns `EmbeddingResult{Status:"degraded", ReasonCode:"ai_model_unavailable"}` on no candidate/build/credential availability. `EmbeddingService` invokes the validated provider then atomic writer. Fx registers services/reclaimer only with a database runtime; API startup remains valid with an empty key because no profile is selected implicitly.

- [ ] **Step 4: Run GREEN.**

  Run: `HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' go test -race -tags=integration ./internal/modules/intelligence/application ./internal/bootstrap -count=1`

  Expected: PASS; retry/repair never exceeds its bound, only one call owns a reuse key, and AI absence leaves non-AI services available.

- [ ] **Step 5: Commit.**

  ```bash
  git add internal/modules/intelligence/application internal/bootstrap/app.go
  git commit -m "feat: orchestrate AI runs and safe degradation"
  ```

## Task 6：管理员模型 API、OpenAPI 与敏感边界

**Files:**
- Create: `internal/modules/intelligence/transport/http/{dto,handler,routes}.go`, `internal/modules/intelligence/transport/http/{handler,routes}_test.go`, `internal/modules/intelligence/transport/http/handler_integration_test.go`
- Modify: `internal/bootstrap/app.go`, `docs/openapi/swagger.json`

**Consumes:** Task 5 model profile service and Task 1 codes; **Produces:** fully documented administrator-only model profile control plane.

- [ ] **Step 1: Write failing handler and contract tests.** Assert unauthenticated = 401, viewer/editor = 403, admin CRUD/restore works, stale `version` = 409, every semantic PATCH including `credential_ref` = `70000`, and every JSON body/OpenAPI schema lacks `credential_ref`, `api_key`, `endpoint`, `parameters`, Prompt and raw response fields. Assert all route annotations produce the six required paths and numeric Result codes.

- [ ] **Step 2: Run RED.**

  Run: `go test ./internal/modules/intelligence/transport/http -count=1 && make openapi-check`

  Expected: FAIL because routes/DTOs/OpenAPI paths are absent.

- [ ] **Step 3: Implement routes and DTOs.** Mount `/api/v1/ai/model-profiles` with `RequireAuthentication` and `RequireRoles(RoleAdmin)`. Use a write-only input DTO for `credential_ref`; response mapper deliberately has no equivalent field. Add Swag annotations, Result/error conversion and Fx route registration. The handler never resolves credentials itself and never returns an internal cause.

- [ ] **Step 4: Run GREEN.**

  Run: `HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' go test -tags=integration ./internal/modules/intelligence/transport/http -count=1 && make openapi-check`

  Expected: PASS; generated `docs/openapi/swagger.json` is clean and the route/role/redaction matrix passes.

- [ ] **Step 5: Commit.**

  ```bash
  git add internal/modules/intelligence/transport/http internal/bootstrap/app.go docs/openapi/swagger.json
  git commit -m "feat: expose AI model profile administration"
  ```

## Task 7：完整验收、运行手册演练与归档

**Files:**
- Modify: `docs/acceptance/008-AIProvider与Embedding基础验收.md`, `docs/operations/README.md`, `docs/acceptance/README.md`, `docs/prd/008-AIProvider与Embedding基础.md`, `docs/plans/008-AIProvider与Embedding基础计划.md`, `docs/prd/README.md`, `docs/plans/README.md`, `docs/README.md`, `README.md`
- Test: `internal/modules/intelligence/...`, `internal/platform/database/...`, `tests/architecture/...`

**Consumes:** Tasks 1–6; **Produces:** independently reviewable Acceptance-008 and only then archived/done task metadata.

- [ ] **Step 1: Capture deliberate RED evidence before accepting.** Preserve named tests showing missing provider/config becomes degraded, a second same-key request is in flight, budget is exhausted/overage is recorded, expired lease is reclaimed, 1023/`NaN` vectors are refused, stale model-version query returns no old vector, and non-admin/write-only API checks fail. Do not manufacture a regression by weakening production code.

- [ ] **Step 2: Execute real dependency evidence.** On a disposable PostgreSQL/Redis fixture, run the exact Task 1 legacy PLAN-007 upgrade/current verify/prepared rollback/legacy verify rehearsal; run Task 3 HNSW `EXPLAIN`; use only an `httptest` OpenAI fixture with a dummy key. Record commands, commit range, fixtures, result summaries and unexecuted ONNX-host matrix in Acceptance-008. No live OpenAI request or real credential is permitted.

- [ ] **Step 3: Run final GREEN gates.**

  ```bash
  HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
  HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
  make ci
  HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
  go test -race -tags=integration ./internal/modules/intelligence/... \
    ./internal/platform/database/... ./tests/architecture -count=1
  make clean
  git diff --check
  git status --short
  ```

  Expected: all commands pass, no generated binary/diff/fixture remains, and Acceptance documents unsupported optional ONNX evidence as a residual risk rather than a pass.

- [ ] **Step 4: Obtain independent final review and archive.** A non-author reviews code, docs, upgrade/rollback, SDK fixture, security boundaries, concurrency tests, OpenAPI and final worktree. Only an `APPROVED` review changes Acceptance result to `accepted`, PRD/Plan to `archived/done`, and indexes/README from 008 `review/backlog` to complete. PLAN-009 readiness is a separate gate.

- [ ] **Step 5: Commit.**

  ```bash
  git add docs/acceptance/008-AIProvider与Embedding基础验收.md docs/operations/README.md \
    docs/acceptance/README.md docs/prd docs/plans docs/README.md README.md
  git commit -m "docs: archive AI provider and embedding plan"
  ```

## 提交边界与禁止合并

Task 1–6 每项必须是一个可回滚、通过其 GREEN 命令的提交，禁止把 SDK、Schema、API 与验收揉成不可审查的大提交。Task 7 只能在完整证据与独立最终复核后提交。遇到契约缺口、新任务类型、需要自定义 endpoint、非 1024 向量或保存 AI 原始 payload 时立即停止，先修订 Design → PRD → Plan 并重新审核；不要以兼容分支、默认模型或运行时 DDL 绕过本计划。

## 自检

- PRD 中所有可交付项分别映射到 Task 1–7；Provider/配置、Schema/记录、静态 JSON、复用/预算、ONNX、向量、API、升级/回退与 Acceptance 均有独立任务。
- 每个实现 Task 明确了路径、输入输出、RED、GREEN 和提交；不存在占位项、泛化文件范围或未定义接口。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-15 | 初始六步 AI Provider 与 Embedding 计划。 |
| v1.1 | 2026-07-17 | 重写为七个可独立验收/提交任务，冻结 SDK、ONNX、凭据、Schema、并发预算、API、升级回退与最终验收边界；仍待独立审核。 |
| v1.2 | 2026-07-17 | 按独立复审收紧 task-type Schema，增加 structured request/ONNX manifest、完整预算方程与 lease reclaimer，并固定 53d7f01 historical verifier。 |
| v1.3 | 2026-07-17 | 统一所有运行路径的 budget→run 锁序，增加 UTC-day overage 封账和 retry lease 刷新，并闭合 ONNX 的 `onnx && cgo` build matrix。 |
| v1.4 | 2026-07-17 | 固化 create-only credential_ref，并定义 OpenAI model ID 严格校验与本地 ModelVersion 的 fixture 契约。 |
