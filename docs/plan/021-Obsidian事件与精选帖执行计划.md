---
layer: Plan
scope: issue
doc_no: "021"
title: Obsidian 事件与精选帖执行计划
status: planned
version: v1.1
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/021-Obsidian事件与精选帖执行计划.md
prd: docs/prd/005-报告与知识库需求.md
design: docs/design/005-报告与知识库设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-005-S04-T01 第二阶段
depends_on: ["020", "042"]
---

# Plan 021：Obsidian 事件与精选帖

## 导出对象与引用顺序

修改 `backend/app/knowledge/schemas.py`、`services.py`、`obsidian.py`，新增 `knowledge/objects.py` 装配只读content/events/reports DTO。只导出被报告或事件实际引用的帖子，保留content_version和来源URL，不把全库批量写进vault；事件使用已确认event_id/revision、热度公式和成员引用。稳定映射type/object_id→relative_path，帖子路径 `HotKey/帖子/<来源>/<净化标题前40字>-<短ID>.md`，事件 `HotKey/事件/<净化标题>-<短ID>.md`。

同一次导出先完成精选帖子并登记hash，再生成引用这些成功目标的事件；最后刷新需要补链的日报管理区块，不能改已存档报告的事实数字/版本。未成功帖子链接原帖并保留export失败，不写悬空双链。合并旧事件笔记保留历史与新事件链接，不删除用户笔记；拆分新事件用新ID/路径，不覆盖旧对象。

- [ ] CHK-021-101 → DATA/OPS：扩展 `backend/tests/unit/test_obsidian_export.py` 验证同名标题、标题变更路径稳定、未引用帖不导出、帖子失败事件不写悬链、合并/拆分映射。
- [ ] CHK-021-102 → NFR-005-002：`backend/tests/integration/test_knowledge_exports.py` 验证两对象部分成功重试、已有用户区保留、DB记录与文件哈希一致，沿用020并发写保护。
- [ ] CHK-021-103 → AC-005-006/008：真实M3事件与代表帖子，在现有vault打开目标笔记→事件双链→日报链接，核对原帖/版本/公式和用户区。

运行B；无新增HTTP契约，导出状态沿用Job和后续023展示方式。事件M3产品未通过时只可受控测试，真实事件导出退出保持待验收。证据归M4 Acceptance Plan021。

## 范围与需求

事件及精选帖子笔记真实验收须等M3的014—016、042及AC-004-001通过，不以日报导出或热榜命中替代。本卡承接 [PRD005](../prd/005-报告与知识库需求.md) FR-005-006、NFR-005-002及AC-005-006/008的事件阶段；仅导出被报告/事件引用的精选帖子，不批量镜像。文件边界复用020，技术前置消费042读取DTO。

## SPEC

| ID | 规格 |
|---|---|
| `SPEC-021-DATA-001` | 事件笔记引用已验收事件版本、来源平台、代表帖子及时间；精选帖笔记引用原生身份和可打开原帖。 |
| `SPEC-021-OPS-001` | 先写目标事件与精选帖笔记，再补报告/事件双链；只在 `HotKey/` 原子替换管理区块并保留用户区块。 |
| `SPEC-021-JOB-001` | 事件未验收、引用目标缺失或 vault 不可写时明确失败/待处理；不产出悬空链接，不回滚原事件或报告。 |

## 验收与 Checklist

Given：M3 真实事件已验收且报告引用明确。When：导出事件和精选帖、重导出并检查双链。Then：每条链接目标先存在，版本与原帖可追，用户区块保留。

- [ ] `CHK-021-G0-001`：核对 046 S03、PRD/Design、Plan 020 和 M3 产品证据。
- [ ] `CHK-021-G1-001`：冻结三项 SPEC 的事件/内容版本、笔记命名与链接时序。
- [ ] `CHK-021-G2-001`：受控验证缺失目标、旧事件版本、只读 vault 和重导出。
- [ ] `CHK-021-G3-001`：实现事件/精选帖导出与原子写入，不改用户区块。
- [ ] `CHK-021-G4-001`：运行 Ruff/mypy/pytest、路径与文件恢复检查。
- [ ] `CHK-021-G5-001`：在现有 vault 核对真实事件、精选帖、原帖和双链。
- [ ] `CHK-021-G6-001`：记录 `AC-005-006/008` 的事件阶段，其他笔记仍独立验收。

缺少已验收事件时保持 planned，不造虚拟事件笔记；写入失败只回退本次文件操作，保留业务事实。
