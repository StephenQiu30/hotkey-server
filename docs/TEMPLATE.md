# HotKey 正式文档模板

所有正式文档遵守根 `AGENTS.md`。PRD、Design 与里程碑 Acceptance 依需求主题保持原编号；执行 Plan 以一份 Issue 一份文件，在 `docs/plan/` 从 `001` 全局连续编号，`doc_no` 是 Plan 自己的编号，不要求与源 PRD 同号。除索引 `README.md` 外，文件名使用 `NNN-中文主题.md`。SPEC 与 CHECKLIST 写入对应 Plan。

根目录 `PROJECT.md` 是技术选型入口，`HANDOVER.md` 是交接快照；两者不占用 doc_no，不代替正式 Design/Acceptance。

根目录 `BACKLOG.md` 是唯一进度看板（阶段、验收、任务卡、待办、风险，≤ 10 KB），`HANDOVER.md` 只记录当前实现快照（≤ 5 KB），不占用 doc_no。

**逐 Issue 计划流程（2026-09-26 起）**：总 PRD/Design 为 001，M1—M6 PRD/Design 为 002—007；Design 是 Epic，执行 Plan 独立从 001 起连续编号，每份只对应一个 Issue。`docs/plan/README.md` 索引 Design、PRD、Plan 与整体 AC；任务开工前在该 Issue 的 Plan 补精确文件、SPEC、Checklist 和证据门槛。旧里程碑 Plan 从 Git 历史对照，不作为执行入口。

编号以 `docs/README.md` 和 `docs/plan/README.md` 的当前台账为准。PRD/Design/Acceptance 的需求编号与 Plan 的执行序号是两条独立序列；Plan 用 `prd`、`design`、`source_task` 追溯来源，不能因 Plan 重编号改动原 FR/NFR/AC 含义。纠错须同步文件名、元数据、内部 ID 与引用，并在台账记录旧号去向。

## 状态枚举

| Layer | 状态 |
|---|---|
| Research | `draft`、`reviewed` |
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
| 规格 | `SPEC-NNN-TYPE-001`，TYPE 为 API、DATA、JOB、UI、SEC、OBS 或 OPS | `SPEC-003-JOB-001` |
| 检查 | `CHK-NNN-G0-001` 至 `CHK-NNN-G6-001` | `CHK-005-G5-002` |
| 证据 | `EV-NNN-001` | `EV-005-004` |

编号一旦使用不得改变含义；新 Issue Plan 取下一个三位序号，不使用 `001-A` 或 `1.1`。被替代的执行计划在台账记旧号与新号映射，旧原文可从 Git 历史查阅。`OPEN` 只用于尚未决策的问题；关闭时保留原条目、结论与日期，并映射到承接结论的 `DEC`、`RSK`、`BR`/`FR`/`NFR`，不得静默删除。

## Research

```yaml
---
layer: Research
scope: shared
doc_no: "NNN"
title: 主题调研
status: draft
version: v1.0
owner: HotKey Team
canonical_path: docs/research/NNN-主题调研.md
prd: docs/prd/NNN-主题.md
---
```

正文至少包含调研对象、日期与版本、一手资料、已确认功能、文档与实现差异、与产品需求的映射、费用和许可条件、验证边界。区分源码事实、项目方声明、产品建议与真实运行结果；未运行不得写成已验收。

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
plan_index: docs/plan/README.md
parent: docs/design/001-热点舆情监控平台总体设计.md # 里程碑 Design
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
plan_index: docs/plan/README.md
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

拆分为逐项 PRD 时，新文件使用独立编号，并在元数据中增加 `parent`（父级文档路径）与 `source_requirement`（原始需求 ID）。原始需求及其优先级不重编号，本地规则使用新文档编号。保留父级验收场景，新增正常、边界、失败与权限场景；按真实覆盖关系关联细化规则，不把父级笼统场景当成全部细节已覆盖。尚未建立的 `design` / `plan_index` 使用 `null`，不链接不存在的文件。

## Plan（含 SPEC 与 CHECKLIST）

一份 Plan 对应一个可独立实施或取证的 Issue。必须映射源 PRD 的相关 BR/FR/NFR/AC，写明局部完成边界、依赖与非目标；SPEC 使用本 Plan 编号，Checklist 逐 SPEC 和 G0—G6 对应预期证据。跨多个 Issue 的父级 AC 仍由里程碑 Acceptance 统一关闭。未获准的来源或渠道先写准入门禁，条件明确后再建逐来源或逐渠道 Plan。

所有新建或调整的实施 Plan 均登记 `architecture_prerequisite: "046 S03"`；旧 Design 046 全局异常与响应契约和旧 Plan 046 前置计划已删除，见 Git 历史。除旧 046 前置计划本身外，Plan 进入实现前须有 046 S03 真实通过证据；研究/设计准备不受阻塞。公共技术前置不新增产品 FR/NFR 分母，领域成功/错误/分页/任务响应必须复用 PROJECT.md 规定的类型化契约与稳定 `ErrorView`，运行响应、OpenAPI 和生成客户端一致。该规则随模板传播到后续计划，不能只依赖 BACKLOG 的临时备注。

```yaml
---
layer: Plan
scope: issue
doc_no: "NNN"
title: 单项执行计划
status: planned
version: v1.0
owner: HotKey Team
canonical_path: docs/plan/NNN-单项执行计划.md
design: docs/design/002-信息获取主链路设计.md # 源 Epic
prd: docs/prd/002-信息获取主链路需求.md # 源需求
source_task: TASK-002-S01-T01 # 旧卡有对应关系时填写；新增 Issue 说明来源缺口
architecture_prerequisite: "046 S03"
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

每个执行阶段都应有任务、进入条件、完成输出及对应 checklist；依赖明确到先行 Plan 的具体切片，避免把关联关系写成循环前置。未建立 Design 时可以先写 planned 的计划，将设计列为实施前置并使用 design: null；不因此允许跳过设计直接实现。Plan 编写进度与产品验收进度分开，BACKLOG 以真实 AC/Acceptance 更新完成状态。

```markdown
- [ ] `CHK-001-G3-001` → `SPEC-001-API-001` / `AC-002-002`：失败测试已保存；证据为测试名称和失败摘要。
- [ ] `CHK-001-G4-001` → `AC-002-002`：OpenAPI 与客户端一致；证据为生成和校验命令。
```

## Acceptance

只有实现和验证真实发生后才创建与 PRD 里程碑同编号的 Acceptance；它汇总该里程碑关联 Plan 的证据。必须记录代码版本、环境、日期、每条 AC 的结果、命令摘要、人工/性能/安全/恢复证据、已知限制和回滚验证。没有证据的目标不得标记为 `passed`。

## Operations

Operations 必须可重复运行，包含适用环境、前置检查、操作步骤、成功判据、失败停止条件、回滚/恢复、审计证据和演练频率。`planned` 不得作为生产手册使用；只有随 Acceptance 演练通过后才能改为 `active`。
