---
layer: Acceptance
doc_no: "000"
audience: [Dev, QA, Ops]
feature_area: 文档治理
purpose: 定义长期验收证据的结构、结论和归档边界
canonical_path: docs/acceptance/README.md
status: review
version: v2.3
owner: HotKey Server Team
inputs:
  - docs/README.md
  - docs/plans/README.md
outputs:
  - Acceptance 编写规范
triggers:
  - Plan 完成实施并准备验收
  - 回归测试改变既有验收结论
downstream:
  - docs/operations/README.md
---

# 验收文档规范

Acceptance 保存可长期复核的完成证据，不保存完整终端流水或临时调试记录。

## 必需内容

1. 被验收 PRD、Plan、Design 和准确提交 SHA
2. 验收环境、依赖版本和数据 Fixture
3. 红灯命令、失败信号和对应验收项
4. 绿灯命令、结果摘要和证据路径
5. Schema、OpenAPI、运行时或浏览器证据
6. 未执行项目、原因和残余风险
7. 最终结论：accepted、rejected 或 accepted_with_risk

## 实施前验收标准审核

代码开工前，PRD 与 Plan 必须先定义可验证验收标准。Reviewer 需要确认：

- 每项核心需求至少对应一个正常路径验收
- 状态、权限、输入边界、并发、幂等、删除和恢复有适用的异常验收
- 数据库变更同时验证完整 Schema、记录模型、约束和 Repository
- API 变更同时验证 HTTP、业务码、Result、OpenAPI 和敏感信息边界
- 外部依赖具备超时、限流、重试、永久失败和降级验收
- 红灯信号能够证明需求尚未满足，绿灯命令能够证明实现满足需求
- 无法自动化的项目写明替代证据、责任人和残余风险

验收标准不完整时，Plan Review 不能 approved，任务不能进入 ready。

## 命名

使用 `序号-主题-验收.md`。编号必须与被验收 PRD 和 Plan 一致。

## 收录边界

- 可收录稳定测试结果、回归基线、截图索引、性能基线和故障恢复结论
- 不收录重复日志、临时命令输出、会议讨论或无法关联提交的结果
- PR 中的短期 CI 结果只有形成长期质量门禁时才进入本目录

## 验收记录

| 编号 | 验收 | 结论 |
|---|---|---|
| 005 | [监控主题规则与来源配置](005-监控主题规则与来源配置验收.md) | accepted |
| 006 | [查询规划与 RSS/HN 采集](006-查询规划与RSS-HN采集验收.md) | accepted |
| 007 | [内容标准化、去重与 MinIO 证据](007-内容标准化去重与MinIO证据验收.md) | accepted |
| 008 | [AI Provider 与 Embedding 基础](008-AIProvider与Embedding基础验收.md) | accepted |
| 009 | [多语言相关性匹配与反馈](009-多语言相关性匹配与反馈验收.md) | accepted |
| 010 | [事件聚类生命周期与人工治理](010-事件聚类生命周期与人工治理验收.md) | accepted |
| 011 | [热度趋势与监控排序](011-热度趋势与监控排序验收.md) | accepted |
| 012 | [证据化事件摘要实体与主张](012-证据化事件摘要实体与主张验收.md) | accepted |
| 013 | [Cron与River主链路编排](013-Cron与River主链路编排验收.md) | pending |
| 014 | [Obsidian知识提案修订与对账](014-Obsidian知识提案修订与对账验收.md) | pending |
| 015 | [日报周报与发布快照](015-日报周报与发布快照验收.md) | pending |
| 016 | [邮件与RSS-Atom订阅交付](016-邮件与RSS-Atom订阅交付验收.md) | pending |
| 017 | [运行治理容量与端到端验收](017-运行治理容量与端到端验收.md) | pending |

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.1 | 2026-07-16 | 收录已 accepted 的 Acceptance-006。 |
| v1.2 | 2026-07-17 | 收录独立最终复核通过的 Acceptance-007。 |
| v1.3 | 2026-07-17 | 创建 PLAN-008 实施前验收模板，结论仍为 pending。 |
| v1.4 | 2026-07-17 | 收录独立最终复核通过的 Acceptance-008。 |
| v1.5 | 2026-07-17 | 创建 PLAN-009 实施前验收模板，结论仍为 pending。 |
| v1.6 | 2026-07-17 | 同步 PLAN-009 Task 1–5 已验证、Task 6 最终验收待执行的状态；结论保持 pending。 |
| v1.7 | 2026-07-17 | 同步 PLAN-009 Task 1–6 自动门禁通过、独立最终复核待执行的状态；结论保持 pending。 |
| v1.8 | 2026-07-17 | 同步 PLAN-009 独立最终复核提出的两项 P1 已完成定向整改；完整门禁与复审仍待完成，结论保持 pending。 |
| v1.9 | 2026-07-17 | 记录 `d4efda5` 的 relevance-review 序列化修复及完整 `make ci`、PostgreSQL/Redis/MinIO integration race 通过；独立复审仍待完成，结论保持 pending。 |
| v2.0 | 2026-07-17 | Acceptance-009 在 `d4efda5` 经非主要编写者 APPROVED 并归档为 accepted；PLAN-010 仍须独立完成自身 readiness。 |
| v2.1 | 2026-07-17 | 创建 PLAN-010 实施前验收模板，固定候选上限、跨语言 F1、生命周期、事务回滚、人工锁、API 安全和独立复审门禁；结论保持 pending。 |
| v2.2 | 2026-07-17 | PLAN-010 文档经非主要编写者复核 APPROVED 并进入 `accepted/ready`；Acceptance-010 仍保持实施前 pending。 |
| v2.3 | 2026-07-17 | `804cac2` 通过全量门禁及独立实现复审，Acceptance-010 归档为 accepted；长期流水线质量由 PLAN-013/017 验收。 |
| v2.4 | 2026-07-17 | `59e85fe` 固定事件智能离线评测并通过完整质量门禁，Acceptance-012 归档为 accepted。 |
