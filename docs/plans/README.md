---
layer: Plan
doc_no: "000"
audience: [PM, Dev, QA, Ops]
feature_area: 文档治理
purpose: 定义 HotKey Server 执行计划的结构、状态和验收映射
canonical_path: docs/plans/README.md
status: review
review_status: pending
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/README.md
  - docs/prd/README.md
outputs:
  - Plan 编写规范
  - PRD 到 Plan 的一一映射
triggers:
  - 新增或拆分执行任务
  - 修改执行文件、步骤、验证命令或提交边界
downstream:
  - docs/plans/001-模块化单体启动与工程门禁计划.md
  - docs/acceptance/README.md
---

# 执行计划规范与索引

Plan 把一个 PRD 转换为可直接实施的文件级步骤。执行者只读 Design 和 PRD 仍无法确定修改文件或验证命令时，Plan 就不完整。

## Plan 必须包含

1. 计划目标和完成后的可观察结果
2. 对应 PRD、Design、前置 Plan 和开工条件
3. 明确的创建、修改、删除文件清单
4. 按测试先行排列的执行步骤
5. Schema、记录模型、OpenAPI 和文档同步范围
6. 红灯命令、预期失败信号、绿灯命令和通过标准
7. 验收清单、提交边界、回滚点和残余风险

文件清单必须使用仓库相对路径。允许使用受限 glob 表达同一模块内的一组文件，但不能只写“修改相关代码”。

## 状态门禁

- `status: accepted`、`review_status: approved` 且 `execution_status: ready` 才能开工
- 前置 Plan 未 done 时，下游 Plan 只能 backlog 或 blocked
- 开工后只更新 ticket 或 Workpad 的过程状态，Plan 正文仅在执行契约变化时更新
- 完成全部验收并生成 Acceptance 后，execution_status 才能改为 done
- approved Plan 的目标、范围、文件、步骤、依赖或验收发生变化时，review_status 必须重置为 pending

## Plan Review 门禁

Plan 必须由非本计划主要编写者的 Reviewer 再次审核。Reviewer 可以是独立 Agent 或人工，但审核结论必须保存在 PR、ticket 或 Workpad 中并可追溯。

审核至少覆盖：

1. 目标是否完整映射 PRD，且没有遗漏用户价值或扩大非目标。
2. 依赖是否完整、无环，并且不会要求尚未实现的下游能力。
3. 创建、修改、删除文件是否明确，模块所有权与依赖方向是否符合 Design。
4. Schema、记录模型、Repository、OpenAPI、错误码和文档是否同步。
5. 正常路径、失败路径、权限、并发、幂等、删除、恢复和降级是否有验收。
6. 每项验收是否对应可执行红灯、绿灯或替代证据。
7. 提交边界和回滚是否不会恢复旧双轨或隐藏兼容路径。

存在未解决的高风险问题、循环依赖、不可执行命令或不可验证验收时，review_status 必须为 changes_requested。

## 执行纪律

1. 先运行红灯测试或结构校验，记录需求尚未满足的信号。
2. 实施让红灯通过的最小代码。
3. 运行范围测试、全量测试、Lint、构建和架构校验。
4. 涉及数据库时同步完整 `db/schema.sql`、记录模型和 Repository 测试。
5. 涉及 API 时同步 OpenAPI、错误码和契约测试。
6. 单个 Plan 不得顺带实施下游任务。

## 当前计划

001–017 与 [PRD 索引](../prd/README.md) 一一对应。计划依赖顺序与 PRD DAG 相同。

| 编号 | PRD | Plan | 前置 Plan | 执行状态 | 审核状态 |
|---|---|---|---|---|---|
| 001 | [模块化单体、Schema基线与工程门禁](../prd/001-模块化单体启动与工程门禁.md) | [执行计划](001-模块化单体启动与工程门禁计划.md) | 无 | done | approved |
| 002 | [数据库运行时、事务与兼容性平台](../prd/002-单一Schema与数据库平台.md) | [执行计划](002-单一Schema与数据库平台计划.md) | 001 | done | approved |
| 003 | [HTTP契约安全与可观测基础](../prd/003-HTTP契约安全与可观测基础.md) | [执行计划](003-HTTP契约安全与可观测基础计划.md) | 001, 002 | ready | approved |
| 004 | [身份认证会话与权限](../prd/004-身份认证会话与权限.md) | [执行计划](004-身份认证会话与权限计划.md) | 002, 003 | backlog | pending |
| 005 | [监控主题规则与来源配置](../prd/005-监控主题规则与来源配置.md) | [执行计划](005-监控主题规则与来源配置计划.md) | 002, 003, 004 | backlog | pending |
| 006 | [查询规划与RSS-HN采集](../prd/006-查询规划与RSS-HN采集.md) | [执行计划](006-查询规划与RSS-HN采集计划.md) | 005 | backlog | pending |
| 007 | [内容标准化去重与MinIO证据](../prd/007-内容标准化去重与MinIO证据.md) | [执行计划](007-内容标准化去重与MinIO证据计划.md) | 002, 006 | backlog | pending |
| 008 | [AIProvider与Embedding基础](../prd/008-AIProvider与Embedding基础.md) | [执行计划](008-AIProvider与Embedding基础计划.md) | 002, 007 | backlog | pending |
| 009 | [多语言相关性匹配与反馈](../prd/009-多语言相关性匹配与反馈.md) | [执行计划](009-多语言相关性匹配与反馈计划.md) | 005, 007, 008 | backlog | pending |
| 010 | [事件聚类生命周期与人工治理](../prd/010-事件聚类生命周期与人工治理.md) | [执行计划](010-事件聚类生命周期与人工治理计划.md) | 009 | backlog | pending |
| 011 | [热度趋势与监控排序](../prd/011-热度趋势与监控排序.md) | [执行计划](011-热度趋势与监控排序计划.md) | 010 | backlog | pending |
| 012 | [证据化事件摘要实体与主张](../prd/012-证据化事件摘要实体与主张.md) | [执行计划](012-证据化事件摘要实体与主张计划.md) | 008, 010 | backlog | pending |
| 013 | [Cron与River主链路编排](../prd/013-Cron与River主链路编排.md) | [执行计划](013-Cron与River主链路编排计划.md) | 006–012 | backlog | pending |
| 014 | [Obsidian知识提案修订与对账](../prd/014-Obsidian知识提案修订与对账.md) | [执行计划](014-Obsidian知识提案修订与对账计划.md) | 010, 012, 013 | backlog | pending |
| 015 | [日报周报与发布快照](../prd/015-日报周报与发布快照.md) | [执行计划](015-日报周报与发布快照计划.md) | 011, 012, 013 | backlog | pending |
| 016 | [邮件与RSS-Atom订阅交付](../prd/016-邮件与RSS-Atom订阅交付.md) | [执行计划](016-邮件与RSS-Atom订阅交付计划.md) | 014, 015 | backlog | pending |
| 017 | [运行治理容量与端到端验收](../prd/017-运行治理容量与端到端验收.md) | [执行计划](017-运行治理容量与端到端验收计划.md) | 001–016 | backlog | pending |
