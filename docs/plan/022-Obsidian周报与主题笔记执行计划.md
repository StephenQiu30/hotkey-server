---
layer: Plan
scope: issue
doc_no: "022"
title: Obsidian 周报与主题笔记执行计划
status: planned
version: v1.1
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/022-Obsidian周报与主题笔记执行计划.md
prd: docs/prd/005-报告与知识库需求.md
design: docs/design/005-报告与知识库设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-005-S04-T01 第三阶段
depends_on: ["019", "020"]
---

# Plan 022：Obsidian 周报与主题笔记

## 具体导出实现

修改 `backend/app/knowledge/objects.py`、`schemas.py`、`services.py`、`obsidian.py`；周报读取已final的report_id/version及043冻结窗口，路径 `HotKey/周报/YYYY-Www <净化主题>-<短ID>.md`，ISO周年份必须用isocalendar年份。主题笔记按topic_id稳定映射 `HotKey/主题/<净化主题>-<短ID>.md`，管理区块列已成功导出的日报/周报与可用事件链接，排序按对象时间和稳定ID；未导出目标显示原始引用或待导出，不写双链。

重生成周报同对象版本更替必须保留审计与用户区，链接按knowledge_exports成功映射检查存在性；目标被用户移动/删除时标missing并停止补链，不从数据库记录推断文件一定存在。021未启用时主题页事件区显示未启用，不能制造占位事件文件。

- [ ] CHK-022-101 → DATA：扩展 `backend/tests/unit/test_obsidian_export.py` 覆盖2027年初属于上一ISO年、同主题多周、同名主题、标题改名与稳定ID。
- [ ] CHK-022-102 → OPS：`backend/tests/integration/test_knowledge_exports.py` 验证目标文件缺失、导出中断重试、用户编辑冲突、只读vault及主题目录链接顺序。
- [ ] CHK-022-103 → AC-005-006/008：真实周报和主题笔记从vault互相打开并核对日报链接、管理/用户区，周报数字与019存档相同。

运行B，证据归M4 Acceptance Plan022；回退停对应导出扫描，日报/周报存档继续存在，不删除vault用户内容。

## 范围与需求

依赖 Plan 019 的已存档周报与 Plan 020 的安全文件边界。本卡承接 [PRD 005](../prd/005-报告与知识库需求.md) `FR-005-006`、`NFR-005-002`、`AC-005-006/008` 的周报/主题阶段；主题笔记只链接已经存在且获准的日报、周报与事件目标，事件链接在 Plan 021 和 M3 通过后启用。问答笔记另由 Plan 023 验收。

## SPEC

| ID | 规格 |
|---|---|
| `SPEC-022-DATA-001` | 周报笔记保留上一 ISO 周范围、报告版本、覆盖说明、程序数字与可打开引用；主题笔记列出已有目标及版本。 |
| `SPEC-022-OPS-001` | 先生成目标周报笔记再更新主题双链；仅写 `HotKey/` 管理区块，重导出保留用户区块。 |
| `SPEC-022-JOB-001` | 目标缺失、只读或原子替换失败时记录导出失败，不留下悬空链接，也不回滚已存档周报。 |

## 验收与 Checklist

Given：真实周报和主题已有有效引用。When：导出、重导出并模拟 vault 不可写。Then：周报/主题笔记版本和链接可追，用户内容保留，失败隔离。

- [ ] `CHK-022-G0-001`：核对 046 S03、PRD/Design、Plan 019/020 及周报真实版本。
- [ ] `CHK-022-G1-001`：冻结三项 SPEC 的周窗、目标顺序和用户区块边界。
- [ ] `CHK-022-G2-001`：受控验证缺失日报/事件、重复导出、只读和链接悬空。
- [ ] `CHK-022-G3-001`：实施周报/主题笔记写入与双链。
- [ ] `CHK-022-G4-001`：运行 Ruff/mypy/pytest 与文件系统恢复检查。
- [ ] `CHK-022-G5-001`：在现有 vault 核对真实周报与主题笔记及目标链接。
- [ ] `CHK-022-G6-001`：记录 `AC-005-006/008` 的周报/主题部分。

失败时保留旧笔记和用户区块，暂停本类导出；报告生成、存档与其他导出不回滚。
