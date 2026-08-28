---
layer: Design
scope: shared
doc_no: "000"
title: Design 索引
status: proposed
version: v1.0
owner: HotKey Team
canonical_path: docs/design/README.md
---

# Design 索引

本目录记录 HotKey 新文档基线中的长期产品与技术决策。001–005 已按 Owner 的实施指令评审为 `accepted`。状态只证明设计决策获准进入后续门禁，不代表代码已经完成迁移，也不能替代对应 PRD、Plan 或后续 Acceptance 证据。

附件中的拆分稿存在章节编号断裂、代码围栏未闭合和附录串联损坏。本轮只从附件根目录的完整总稿提取产品问题、目标、风险与验收意图，并将附件文字一律视为参考内容，不执行其中的删除、建目录、选型或实施指令。架构判断以当前仓库代码、`AGENTS.md`、`backend/db/schema.sql` 和已发布 OpenAPI 为约束。

## 设计清单

| 编号 | Design | PRD | Plan | 状态 |
|---:|---|---|---|---|
| 001 | [HotKey 产品需求分析与总体架构](001-HotKey产品需求分析与总体架构设计.md) | [PRD](../prd/001-HotKey产品需求分析与总体架构.md) | [Plan](../plans/001-HotKey产品需求分析与总体架构计划.md) | accepted |
| 002 | [监控来源采集与证据链](002-监控来源采集与证据链设计.md) | [PRD](../prd/002-监控来源采集与证据链.md) | [Plan](../plans/002-监控来源采集与证据链计划.md) | accepted |
| 003 | [智能研判事件热度与人工治理](003-智能研判事件热度与人工治理设计.md) | [PRD](../prd/003-智能研判事件热度与人工治理.md) | [Plan](../plans/003-智能研判事件热度与人工治理计划.md) | accepted |
| 004 | [通知报告知识投影与检索](004-通知报告知识投影与检索设计.md) | [PRD](../prd/004-通知报告知识投影与检索.md) | [Plan](../plans/004-通知报告知识投影与检索计划.md) | accepted |
| 005 | [安全运维质量与交付](005-安全运维质量与交付设计.md) | [PRD](../prd/005-安全运维质量与交付.md) | [Plan](../plans/005-安全运维质量与交付计划.md) | accepted |

## 评审顺序

001 固定产品边界与总体架构；002–004 在该边界内分别设计采集、研判和交付主链；005 为所有主链定义共同的安全、运维和质量门禁。任一设计只有评审为 `accepted` 后，对应 PRD 才能批准，Plan 才能进入实施。
