---
layer: Plan
doc_no: "008"
audience: [Dev, QA, Ops]
feature_area: AI运行基础
purpose: 实施 AI Provider、模型配置、运行记录与 Embedding 基础
canonical_path: docs/plans/008-AIProvider与Embedding基础计划.md
status: review
execution_status: backlog
review_status: pending
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/008-AIProvider与Embedding基础.md
  - docs/plans/002-单一Schema与数据库平台计划.md
  - docs/plans/007-内容标准化去重与MinIO证据计划.md
outputs:
  - intelligence 运行基础
  - 1024 维 Embedding 存储与检索
triggers:
  - PRD-008 accepted 且 ready
downstream:
  - docs/acceptance/008-AIProvider与Embedding基础验收.md
depends_on: [PLAN-002, PLAN-007]
---

# AI Provider 与 Embedding 基础计划

## 计划目标

交付可替换、可预算、可审计的 Provider 和版本化 Embedding 能力，不让 AI 成为 P0 非 AI 事实的依赖。

## 开工条件

- 当前 Plan 的 status 为 accepted、review_status 为 approved、execution_status 为 ready
- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/modules/intelligence/domain/provider.go | Provider 端口 |
| 创建 | internal/modules/intelligence/domain/run.go | AI 运行与复用键 |
| 创建 | internal/modules/intelligence/domain/embedding.go | 向量空间契约 |
| 创建 | internal/modules/intelligence/application/model_selector.go | 模型、预算与回退 |
| 创建 | internal/modules/intelligence/application/embedding_service.go | 生成、复用与失效 |
| 创建 | internal/modules/intelligence/infrastructure/provider/*.go | 官方 SDK 适配 |
| 创建 | internal/modules/intelligence/infrastructure/onnx/*.go | 可选本地 Embedding |
| 创建 | internal/modules/intelligence/infrastructure/postgres/*.go | 运行与向量 Repository |
| 创建 | internal/modules/intelligence/schemas/*.json | 版本化 JSON Schema |
| 修改 | db/schema.sql | model profiles、ai_runs、embeddings |
| 创建 | internal/modules/intelligence/**/*_test.go | Provider、预算和向量测试 |

## 执行步骤

1. 先写 Provider 成功、超时、429、5xx、非法 JSON 与预算红灯测试。
2. 同步 AI 运行和 halfvec(1024) Schema。
3. 实现 Provider 端口、模型选择和凭据引用。
4. 实现 reuse_key、一次结构修复、重试和失效。
5. 实现内容与 Monitor Embedding 的版本隔离和 HNSW 查询。
6. 接入可选 ONNX，实现无 Provider 的显式降级。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/modules/intelligence/... -count=1 | Provider 与向量测试失败 |
| 绿灯 | go test ./internal/modules/intelligence/... -count=1 | 全部通过 |
| 集成 | go test -tags=integration ./internal/modules/intelligence/... | pgvector 写入与检索通过 |
| Schema | make validate | 模型、向量和 Repository 一致 |
| 全量 | make ci | 全部通过 |

## 验收清单

- 相同 reuse_key 不重复调用
- 模型版本切换后不混用向量空间
- 预算、429 和 5xx 分类与回退正确
- 凭据和 Provider 原始响应不泄露
- 无 LLM 时采集和 Content 查询保持可用

## 提交边界

- test: 定义 Provider 与 Embedding 契约
- impl: 实现 intelligence 运行基础
- feat: 接入首个 Provider 与可选 ONNX


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
