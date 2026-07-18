---
layer: Plan
doc_no: "018"
audience: [Dev, QA, Ops]
feature_area: AI运行基础
purpose: 以测试优先方式实施 LangChainGo、DeepSeek、Ollama 与 Qwen Embedding
canonical_path: docs/plans/archive/018-LangChainGo多模型接入计划.md
status: archived
execution_status: done
review_status: approved
version: v1.6
owner: HotKey Server Team
inputs:
  - docs/design/archive/015-LangChainGo多Provider与本地模型设计.md
  - docs/prd/archive/018-LangChainGo多模型接入.md
  - docs/operations/003-AIProvider与Embedding升级.md
outputs:
  - DeepSeek 与 Ollama Provider 实现
  - Qwen Embedding、配置、Schema、OpenAPI 和升级验证
triggers:
  - PRD-018 accepted 且 ready
downstream:
  - docs/operations/007-LangChainGo多模型升级与连接.md
  - docs/acceptance/archive/018-LangChainGo多模型接入验收.md
depends_on: [PLAN-008, PLAN-017]
---

# LangChainGo 多模型接入执行计划

## 1. 目标与开工门禁

交付 `deepseek`、`ollama` 与 `qwen3-embedding:0.6b`，不改变既有 OpenAI/ONNX 行为。开工要求 Design-015 与 PRD-018 accepted、Plan 本身 accepted/approved/ready、PLAN-008/017 done，并已由非主要编写者审核本计划的文件、红绿灯、升级、回退和验收范围。

## 2. 文件边界

创建：

- `internal/modules/intelligence/infrastructure/provider/langchain_transport.go`
- `internal/modules/intelligence/infrastructure/provider/deepseek.go`
- `internal/modules/intelligence/infrastructure/provider/ollama.go`
- `test/_suite/internal/modules/intelligence/infrastructure/provider/deepseek_test.go`
- `test/_suite/internal/modules/intelligence/infrastructure/provider/ollama_test.go`
- `docs/operations/007-LangChainGo多模型升级与连接.md`

修改：

- `go.mod`、`go.sum`
- `.env.example`
- `internal/platform/config/config.go`
- `internal/modules/intelligence/domain/provider.go`
- `internal/modules/intelligence/domain/profile.go`
- `internal/modules/intelligence/transport/http/dto.go`
- `internal/bootstrap/app.go`
- `db/schema.sql`
- `docs/openapi/docs.go`、`docs/openapi/swagger.json`
- `docs/design/README.md`、`docs/prd/README.md`、`docs/plans/README.md`、`docs/acceptance/README.md`、`docs/operations/README.md`
- `docs/design/archive/015-LangChainGo多Provider与本地模型设计.md`
- `docs/prd/archive/018-LangChainGo多模型接入.md`
- `docs/plans/archive/018-LangChainGo多模型接入计划.md`
- `docs/acceptance/archive/018-LangChainGo多模型接入验收.md`
- `test/_suite/internal/platform/config/config_test.go`
- `test/_suite/internal/modules/intelligence/domain/profile_test.go`
- `test/_suite/internal/modules/intelligence/transport/http/handler_test.go`
- `test/_suite/internal/modules/intelligence/transport/http/handler_integration_test.go`
- `test/_suite/internal/bootstrap/app_test.go`
- `test/_suite/internal/platform/database/database_integration_test.go`
- `test/architecture/schema_test.go`、`test/architecture/openapi_test.go`、`test/architecture/layout_test.go`
- `test/tools/validate-architecture.sh`

删除：无。现有 OpenAI/ONNX adapter、profile 和向量不得清理或迁移。

## 3. 测试先行步骤

### Task 1：冻结 Domain、配置与数据库契约

先增加失败测试：provider enum/能力/凭据矩阵、`HOTKEY_DEEPSEEK_API_KEY` 到固定 credential reference 的映射、Qwen 1024 维限制、Ollama 去前缀 64 位 digest 格式、DeepSeek/Ollama 配置覆盖、无效 URL、安全 Bootstrap 注册、Schema CHECK 与 API/OpenAPI 枚举。数据库约束测试必须分别证明 ONNX 仅 embedding、DeepSeek 禁止 embedding、Ollama embedding 仅 `qwen3-embedding:0.6b`/1024、Ollama 生成允许其他模型且所有 Ollama model version 为 64 位小写 hex。RED 预期为 provider 无效、字段缺失或 schema whitelist 仍只有 openai/onnx。

最小实现扩展 ProviderName、profile validation、AIConfig、Bootstrap、DTO/OpenAPI 与 `db/schema.sql`，但不改现有 OpenAI 路径。同步 Operations-007 的约束升级/回退 SQL。

同步历史依赖门禁时，只从 `layout_test.go` 与 `validate-architecture.sh` 的 forbidden module 列表移除 `github.com/tmc/langchaingo`，保留 Kafka 和所有其他既有架构检查；新增 Layout 测试遍历生产 Go 文件，要求 LangChainGo import 只能位于 `internal/modules/intelligence/infrastructure/provider/`，任何 Domain/Application/Transport/Bootstrap 或其他模块引用都失败。

### Task 2：实现共享安全传输与 DeepSeek

先以 `httptest` 固定 `/chat/completions` 请求、Authorization、模型、JSON mode、usage、修复输入、429、5xx、deadline、transport failure 和原始正文不泄露。RED 预期为 adapter 不存在。

固定加入 `github.com/tmc/langchaingo@v0.1.14`，运行 `go mod verify`；创建无内部重试的安全 HTTP client/transport，实现固定 `https://api.deepseek.com` 的 DeepSeek adapter。fixture 仅通过注入 client/测试 server 替换 transport，不开放生产 Base URL 配置，并以请求计数证明成功和 429/5xx 每次 Application attempt 都只有一次 `/chat/completions` 调用。

### Task 3：实现原生 Ollama 与 Qwen Embedding

先以 `httptest` 固定 `/api/tags`、`/api/chat`、`/api/embed`、显式模型、usage、非法 JSON、模型不存在、429/5xx/deadline。每次调用先匹配 profile model name，并把 tags 的 `sha256:` digest 规范为 64 位小写 hex 后比较；缺失/漂移时断言 chat/embed 计数为零。Embedding 覆盖两个输入保持顺序且恰好两次 `/api/embed`、1024 成功、1023/1025/NaN/Inf 失败和非 `qwen3-embedding:0.6b` profile 失败；429/5xx 的单输入请求计数必须为一。

实现经 URL 预验证的 LangChainGo Ollama adapter，并让结构化与 Embedding 复用现有 Application 校验、预算和运行终态。不得自动 pull 模型或在 adapter 内重试。

### Task 4：Schema/API/升级与连接验证

运行 profile API、数据库 integration、schema、OpenAPI 与 Bootstrap 测试。Operations-007 必须包含备份、停写、升级 preflight、仅替换 CHECK、`db verify`、实机 probe 和回退命令，且不包含真实密钥。数据库 integration 必须从当前旧约束开始执行真实升级 block，证明原 profile 保留且四类新约束生效；再插入新 Provider profile，证明回退 preflight 拒绝；清理该 fixture 后证明 preflight 通过并可恢复旧约束。任何生产回退都必须先在仍运行目标代码/Schema 时做只读 preflight，只有计数为零才允许停服并回退约束与代码。

在本机条件允许时验证 DeepSeek 最小 JSON 生成、Ollama `/api/tags`/生成和 `qwen3-embedding:0.6b` 1024 维；任何缺失 binary、模型或 key 都记录为未执行，不安装系统软件或伪造通过。

### Task 5：完整门禁与验收

运行定向测试、`make ci`、相关 `go test -race`、`git diff --check`、`make clean`。更新 Acceptance-018 的准确命令、结果、提交 SHA（未提交时标 `WORKTREE`）、实机状态与残余风险；再由非主要编写者逐项复核实现与验收。

## 4. 红灯与绿灯

红灯命令：

```bash
go run ./test/runner test ./internal/modules/intelligence/domain
go run ./test/runner test ./internal/modules/intelligence/infrastructure/provider
go run ./test/runner test ./internal/platform/config
go test ./test/architecture -run 'Schema|OpenAPI|Layout|LangChainGo|ForbiddenRuntime'
```

实现前至少出现 `deepseek/ollama` 未定义、配置字段缺失、fixture adapter 缺失、Schema enum 不匹配或历史 Layout 门禁拒绝 LangChainGo 之一。绿灯为上述命令全部通过，且 `TestLangChainGoStaysInsideIntelligenceProviderInfrastructure` 证明 import allowlist 后，随后：

```bash
make ci
go run ./test/runner test -race ./internal/modules/intelligence/infrastructure/provider ./internal/modules/intelligence/domain ./internal/platform/config ./internal/bootstrap -count=1
go test -race ./test/architecture -count=1
git diff --check
make clean
```

## 5. 验收映射

| 验收 | 证据 |
|---|---|
| DeepSeek 协议、usage、修复与错误 | `deepseek_test.go` fixture |
| Ollama 生成/Embedding 与错误 | `ollama_test.go` fixture |
| Qwen 1024 维隔离 | Ollama fixture + profile/domain/schema 测试 |
| 配置和安全 URL | config + bootstrap 测试 |
| Profile/API/OpenAPI/凭据脱敏 | handler、OpenAPI、数据库测试 |
| 既有库升级与回退 | Operations-007 + 旧约束→新约束→回退拒绝/允许的 database integration |
| 无外部条件安全降级 | provider/bootstrap 测试与 Acceptance 实机状态 |

## 6. 提交边界、回滚与风险

单一职责边界为“LangChainGo 多 Provider 接入”，不顺带实现 Agent/RAG。回退前必须先在目标版本运行中查询 DeepSeek/Ollama profile；存在新数据必须停止并保留新版本或另立迁移，不得删除 profile/run/vector。只有计数为零才可停服、恢复旧 CHECK 和回退代码/完整 Schema。主要风险是本机缺少 Ollama/Qwen 或 DeepSeek 外部可用性，fixture 门禁可完成但实机证据可能为 pending。

## 7. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 建立文件级测试先行计划，等待非主要编写者审核。 |
| v0.2 | 2026-07-18 | 按独立审核补齐 digest、数据库矩阵、依赖版本、请求计数、race 与安全回退顺序。 |
| v1.0 | 2026-07-18 | 非主要编写者复核 v0.2 全部原 findings 已关闭，结论 APPROVED，计划 accepted/ready。 |
| v1.1 | 2026-07-18 | 数据库红灯将 Ollama model version 改为去前缀 64 位 digest，审核状态重置为 pending。 |
| v1.2 | 2026-07-18 | 非主要编写者复核 v1.1 digest 变更无阻塞风险，结论 APPROVED。 |
| v1.3 | 2026-07-18 | 全量红灯发现旧门禁禁止 LangChainGo，加入架构测试与脚本同步并重置审核。 |
| v1.4 | 2026-07-18 | 按独立审核明确只移除 LangChainGo 全局禁令、保留其他门禁并加入 provider-only allowlist 红绿灯。 |
| v1.5 | 2026-07-18 | 非主要编写者复核 v1.4 门禁边界完整，结论 APPROVED。 |
| v1.6 | 2026-07-18 | 补齐错误 fixture、真实升级回退和配置诊断后通过独立最终复审，execution done 并归档。 |
