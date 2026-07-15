---
layer: Plan
doc_no: "007"
audience: [Dev, QA, Ops]
feature_area: 内容与证据
purpose: 实施 Content 标准化、三层去重、MinIO 证据和删除同步
canonical_path: docs/plans/007-内容标准化去重与MinIO证据计划.md
status: review
execution_status: backlog
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/007-内容标准化去重与MinIO证据.md
  - docs/plans/002-单一Schema与数据库平台计划.md
  - docs/plans/006-查询规划与RSS-HN采集计划.md
outputs:
  - ingestion 模块
  - MinIO ObjectStore 适配器
triggers:
  - PRD-007 accepted 且 ready
downstream:
  - docs/acceptance/007-内容标准化去重与MinIO证据验收.md
depends_on: [PLAN-002, PLAN-006]
---

# 内容标准化、去重与 MinIO 证据计划

## 计划目标

把 SourceItem 幂等转换为 active Content，并保留允许存储的原始证据、重复关系、指标和删除状态。

## 开工条件

- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/modules/ingestion/domain/content.go | Content 与状态 |
| 创建 | internal/modules/ingestion/domain/repository.go | Content Repository |
| 创建 | internal/modules/ingestion/application/normalizer.go | 字段标准化 |
| 创建 | internal/modules/ingestion/application/deduplicator.go | 三层去重 |
| 创建 | internal/modules/ingestion/application/deletion.go | 删除与过期同步 |
| 创建 | internal/modules/ingestion/infrastructure/postgres/*.go | 内容持久化 |
| 创建 | internal/platform/objectstore/object_store.go | 对象存储端口 |
| 创建 | internal/platform/objectstore/minio.go | MinIO 适配器 |
| 创建 | internal/modules/ingestion/transport/http/*.go | Content 查询 API |
| 修改 | db/schema.sql | authors、contents、assets、metric snapshots |
| 创建 | internal/modules/ingestion/**/*_test.go、testdata/contents/* | 去重、证据和故障测试 |

## 执行步骤

1. 先写 URL、哈希、来源幂等、近似重复和独立报道红灯测试。
2. 同步 Schema、记录模型与 Repository。
3. 实现标准化和三层去重，保存判断版本。
4. 实现 MinIO 确定性对象键、哈希和数据库引用事务。
5. 实现指标缺失语义、删除、过期和孤儿对账。
6. 发布受限 Content 查询 API 与 OpenAPI。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/modules/ingestion/... -count=1 | 标准化与去重测试失败 |
| 绿灯 | go test ./internal/modules/ingestion/... -count=1 | 全部通过 |
| 集成 | go test -tags=integration ./internal/modules/ingestion/... ./internal/platform/objectstore | PostgreSQL/MinIO 通过 |
| 契约 | make openapi && make openapi-validate | contents 契约通过 |
| 全量 | make ci | 全部通过 |

## 验收清单

- 来源项重跑只保留一条 Content
- 精确与近似重复可解释且不折叠独立报道
- 缺失指标保持未知
- MinIO 超时、重复上传和数据库回滚无悬空引用
- deleted/expired 不再进入下游候选

## 提交边界

- test: 定义 Content、去重与证据契约
- impl: 实现 ingestion 与对象存储
- feat: 发布 Content 安全查询 API


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
