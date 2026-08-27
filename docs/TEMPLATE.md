# HotKey 正式文档模板

所有正式文档遵守根 `AGENTS.md`。同一交付项复用一个三位 `doc_no` 和同一主题；除索引 `README.md` 外，文件名使用 `NNN-中文主题.md`。SPEC 与 CHECKLIST 不建立独立目录，统一写入对应 Plan。

## 状态枚举

| Layer | 状态 |
|---|---|
| Design | `proposed`、`accepted`、`superseded` |
| PRD | `draft`、`approved`、`implemented`、`cancelled` |
| Plan | `planned`、`in_progress`、`blocked`、`completed` |
| Acceptance | `passed`、`failed`；仅分类索引 `README.md` 可使用 `active` |
| Operations | `planned`、`active` |

## ID 规范

| 类型 | 格式 | 示例 |
|---|---|---|
| 业务需求 | `BR-NNN-001` | `BR-002-001` |
| 功能需求 | `FR-NNN-001` | `FR-002-003` |
| 非功能需求 | `NFR-NNN-001` | `NFR-005-002` |
| 决策/风险 | `DEC-NNN-001` / `RSK-NNN-001` | `DEC-003-001` |
| 未决项 | `OPEN-NNN-001` | `OPEN-003-002` |
| 验收 | `AC-NNN-001` | `AC-004-002` |
| 任务 | `TASK-NNN-S01-T01` | `TASK-002-S02-T03` |
| 规格 | `SPEC-NNN-{API|DATA|JOB|UI|SEC|OBS|OPS}-001` | `SPEC-003-JOB-001` |
| 检查 | `CHK-NNN-G0-001` 至 `CHK-NNN-G6-001` | `CHK-005-G5-002` |
| 证据 | `EV-NNN-001` | `EV-005-004` |

编号一旦使用不得改变含义；拆分大需求时申请新 `doc_no`，不得使用 `001-A` 或 `1.1`。`OPEN` 只用于尚未决策的问题；关闭时保留原条目、结论与日期，并映射到承接结论的 `DEC`、`RSK`、`BR`/`FR`/`NFR`，不得静默删除。

## Design

```yaml
---
layer: Design
scope: shared
doc_no: "NNN"
title: 主题设计
status: proposed
version: v1.0
owner: HotKey Team
canonical_path: docs/design/NNN-主题设计.md
prd: docs/prd/NNN-主题.md
plan: docs/plans/NNN-主题计划.md
---
```

正文至少包含：

1. 当前事实与问题；
2. 目标与非目标；
3. 约束、假设和未决项；
4. 方案比较与核心决策；
5. 模块边界、依赖方向、数据与状态机；
6. API、异步任务和前端交互；
7. 安全、合规、可观测性；
8. 失败、降级、迁移、兼容与回滚；
9. 风险与验收边界。

## PRD

```yaml
---
layer: PRD
scope: shared
doc_no: "NNN"
title: 主题
status: draft
version: v1.0
owner: HotKey Team
canonical_path: docs/prd/NNN-主题.md
design: docs/design/NNN-主题设计.md
plan: docs/plans/NNN-主题计划.md
---
```

正文至少包含：

1. 用户问题、用户角色与目标 KPI；
2. P0、P1、P2 范围和非目标；
3. 用户故事；
4. 带 ID 的 BR、FR、NFR；
5. 权限矩阵与正常、空、加载、错误、权限不足状态；
6. 依赖、风险和待决策；
7. Given-When-Then 验收标准；
8. `需求 → 决策 → 验收` 追踪矩阵。

不得把未校准的性能数字写成无条件承诺；必须同时记录数据规模、并发、硬件、缓存冷热、统计窗口与排除条件。

## Plan（含 SPEC 与 CHECKLIST）

```yaml
---
layer: Plan
scope: shared
doc_no: "NNN"
title: 主题计划
status: planned
version: v1.0
owner: HotKey Team
canonical_path: docs/plans/NNN-主题计划.md
design: docs/design/NNN-主题设计.md
prd: docs/prd/NNN-主题.md
---
```

正文至少包含：

1. 当前实现与目标差距；
2. G0–G3 前置门禁、依赖和明确不实施项；
3. 预计变更文件及其职责；
4. 按垂直切片组织的测试先行任务；
5. 数据、OpenAPI、生成客户端、配置与部署的变更顺序；
6. 实施规格（SPEC）：
   - API：方法、权限、DTO、错误码、幂等和兼容；
   - DATA：约束、状态机、保留、回填和对账；
   - JOB：幂等键、超时、重试、取消、恢复和并发；
   - UI：桌面/移动及正常、空、加载、错误、权限不足；
   - SEC/OBS/OPS：权限、敏感数据、指标、日志、告警、发布和恢复；
7. 执行检查清单（CHECKLIST）：每项包含 CHK ID、AC 映射和预期证据；
8. 验证命令、灰度、迁移、回滚和完成定义。

Checklist 示例：

```markdown
- [ ] `CHK-NNN-G3-001` → `AC-NNN-001`：失败测试已保存；证据为测试名称和失败摘要。
- [ ] `CHK-NNN-G4-001` → `AC-NNN-002`：OpenAPI 与客户端一致；证据为生成和校验命令。
```

## Acceptance

只有实现和验证真实发生后才创建同编号 Acceptance。必须记录代码版本、环境、日期、每条 AC 的结果、命令摘要、人工/性能/安全/恢复证据、已知限制和回滚验证。没有证据的目标不得标记为 `passed`。

## Operations

Operations 必须可重复运行，包含适用环境、前置检查、操作步骤、成功判据、失败停止条件、回滚/恢复、审计证据和演练频率。`planned` 不得作为生产手册使用；只有随 Acceptance 演练通过后才能改为 `active`。
