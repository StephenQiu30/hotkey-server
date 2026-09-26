---
layer: Plan
scope: issue
doc_no: "020"
title: Obsidian 日报笔记执行计划
status: planned
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/020-Obsidian日报笔记执行计划.md
prd: docs/prd/005-报告与知识库需求.md
design: docs/design/005-报告与知识库设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-005-S04-T01 第一阶段
---

# Plan 020：Obsidian 日报笔记

## 范围与需求

日报导出已有代码和受控验证，但连续真实 vault 写入未证。本卡把旧 Obsidian 大卡的第一阶段单独验收，依据 [PRD 005](../prd/005-报告与知识库需求.md) `FR-005-006`、`NFR-005-002` 和 `AC-005-006/008`。依赖 Plan 018 存档的三自然日日报；事件、周报与主题笔记在 Plan 021/022。只写现有 `~/Desktop/Markdown/Obsidian/HotKey/`，不创建第二个 vault。预计复核 `knowledge/` 导出器、报告触发、文件边界和 tests。

## SPEC

| ID | 规格 |
|---|---|
| `SPEC-020-OPS-001` | 仅写 `HotKey/` 管理区块，使用原子替换；重导出保留“我的笔记”用户区块，不越界写其他 vault 路径。 |
| `SPEC-020-DATA-001` | 日报笔记含主题、日窗、报告版本、frontmatter、有效原帖引用与覆盖缺口；重复导出不复制或悬空双链。 |
| `SPEC-020-JOB-001` | vault 只读或不可用时导出失败可见，可重试；报告存档和信息获取不回滚。 |

## 验收与 Checklist

Given：连续三自然日日报已存档，现有 vault 有用户区块。When：逐日报导出、重复导出并模拟只读。Then：三篇真实笔记、用户区块和链接可核对；只读失败独立且不影响报告。

- [ ] `CHK-020-G0-001`：核对 046 S03、PRD/Design、Plan 018 和现有 vault 边界。
- [ ] `CHK-020-G1-001`：冻结三项 SPEC 的路径、原子替换、版本和失败契约。
- [ ] `CHK-020-G2-001`：受控验证路径越界、重复导出、用户区块和只读失败。
- [ ] `CHK-020-G3-001`：按错例修复日报导出，保留报告与用户内容。
- [ ] `CHK-020-G4-001`：运行 Ruff/mypy/pytest 与文件系统边界验证。
- [ ] `CHK-020-G5-001`：在现有 vault 核对三篇日报笔记、frontmatter、引用和重导出。
- [ ] `CHK-020-G6-001`：记录 `AC-005-006/008` 日报笔记部分；其他笔记类型另验。

失败时停止写入并保留临时失败状态，恢复前不覆盖用户区块；报告和采集继续运行。
