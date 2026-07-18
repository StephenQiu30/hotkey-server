---
layer: Acceptance
doc_no: "018"
audience: [Dev, QA, Ops]
feature_area: AI运行基础
purpose: 保存 DeepSeek、Ollama 与 Qwen Embedding 的长期验收证据
canonical_path: docs/acceptance/archive/018-LangChainGo多模型接入验收.md
status: archived
version: v1.1
owner: HotKey Server Team
result: accepted_with_risk
inputs:
  - docs/design/archive/015-LangChainGo多Provider与本地模型设计.md
  - docs/prd/archive/018-LangChainGo多模型接入.md
  - docs/plans/archive/018-LangChainGo多模型接入计划.md
outputs:
  - Provider、配置、Schema、OpenAPI、升级和连接证据
triggers:
  - PLAN-018 开工或验收状态变化
downstream:
  - docs/operations/007-LangChainGo多模型升级与连接.md
---

# LangChainGo 多模型接入验收

## 当前结论

`accepted_with_risk`。DeepSeek、Ollama 与 Qwen Embedding 的代码、数据契约、升级路径和无外部凭据 fixture 已完成；当前 `.env` 中迁移后的 DeepSeek 凭据访问官方 `/models` 返回 HTTP 401，本机未安装/启动 Ollama，因此两项实机业务探测保持未通过，不以 fixture 冒充外部可用性。

## 被验收对象

- Design：Design-015
- PRD：PRD-018
- Plan：PLAN-018
- 提交：`WORKTREE`（用户未要求提交）
- 环境：macOS arm64、Go 1.26.3、PostgreSQL 18.4、Redis localhost:6379、Docker MinIO
- 依赖：`github.com/tmc/langchaingo@v0.1.14`
- Ollama/Qwen：本机 Ollama 未安装且 11434 不可达；目标模型固定为 `qwen3-embedding:0.6b`、1024 维、profile 保存 `/api/tags` digest 去前缀后的 64 位小写 hex

## 实施前验收清单

| 项目 | 状态 | 预期证据 |
|---|---|---|
| DeepSeek Chat Completions、JSON、usage、修复 | passed | `deepseek_test.go` fixture 与 Application 既有修复测试 |
| DeepSeek 429/5xx/deadline/脱敏 | passed | typed status、deadline 与受限正文 fixture |
| Ollama 原生 chat/embed、usage | passed | `ollama_test.go` 原生 API fixture |
| Ollama `/api/tags` digest 绑定、缺失与漂移 | passed | fixture + Domain/Schema 测试，漂移时模型端点调用为零 |
| Qwen3 0.6B 1024 维及非法向量拒绝 | passed | 1024、1023、1025、NaN、Inf 与模型名约束测试 |
| 配置覆盖、URL 校验、Bootstrap 降级 | passed | config/bootstrap 定向测试与 race |
| Profile Schema、API、OpenAPI、write-only secret | passed | database、handler、OpenAPI 与 architecture 测试 |
| 既有库升级、verify、回退 preflight | passed | Operations-007 + 旧约束到新约束的 integration 测试 |
| DeepSeek 实机连接 | blocked | 当前密钥访问官方 `/models` 返回 401；未发起生成调用 |
| Ollama/Qwen 实机连接 | blocked | 本机无 Ollama CLI，127.0.0.1:11434 不可达 |
| 完整测试、build、lint、race、diff、clean | passed | 下列可复现命令；`make ci` 的工作树 OpenAPI 检查例外单列说明 |
| 成功/429/5xx 请求计数与零内部重试 | passed | fixture 请求计数断言 |
| 非主要编写者最终复核 | passed | 第一轮 findings 关闭后复审结论 `APPROVED` |

## 红灯与绿灯

红灯实际信号：首次定向测试因 `deepseek`/`ollama` enum、配置字段和 adapter 缺失而无法编译；数据库红灯以 SQLSTATE 22001 证明带 `sha256:` 前缀的 digest 超过 `varchar(64)`；全量测试随后证明旧架构门禁仍全局禁止 LangChainGo。实现分别补齐接入、改为保存去前缀 digest，并将依赖 allowlist 严格限制在 intelligence provider infrastructure。

绿灯结果：

```bash
go mod verify
make lint
HOTKEY_TEST_DSN='postgresql:///hotkey_plan018_test_20260718?sslmode=disable' make database-runtime-verify
HOTKEY_TEST_DSN='postgresql:///hotkey_plan018_test_20260718?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' make test build validate schema-verify
go run ./test/runner test -race ./internal/modules/intelligence/infrastructure/provider ./internal/modules/intelligence/domain ./internal/platform/config ./internal/bootstrap -count=1
go test -race ./test/architecture -count=1
git diff --check
make clean
```

上述依赖校验、lint、运行时 Schema、全量测试、build、architecture、OpenAPI、race 与 diff 均通过；临时数据库 fingerprint 为 `fbf72249003644104c60cd6d469f8888a38288f7ee6e37dace873268569d3f50`。`make ci` 执行到 `openapi-check` 时按设计使用 `git diff --exit-code -- docs/openapi/...`，因本交付尚未提交且恰好包含预期生成文件差异而退出；生成器一致性与 OpenAPI contract 测试单独通过，提交后该工作树检查才有可满足的基线。

## 未执行与残余风险

替换为有效 `HOTKEY_DEEPSEEK_API_KEY` 后仍需执行一次最小 JSON 生成；安装并启动 Ollama、pull `qwen3-embedding:0.6b` 后仍需记录 `/api/tags` digest 并执行 1024 维实机 Embedding。外部条件补齐前，验收结论保持 `accepted_with_risk`。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 创建实施前验收模板。 |
| v0.2 | 2026-07-18 | 按独立审核加入 Ollama digest、数据库矩阵、请求计数和 Provider 范围 race 验收。 |
| v1.0 | 2026-07-18 | 记录代码、Schema、OpenAPI、升级、fixture、race 与本机连接证据；外部连接缺失，结论 accepted_with_risk。 |
| v1.1 | 2026-07-18 | 补齐 Provider 错误、零重试、真实升级回退与安全诊断证据；独立复审 APPROVED 后归档。 |
