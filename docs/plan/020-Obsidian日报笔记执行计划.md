---
layer: Plan
scope: issue
doc_no: "020"
title: Obsidian 日报笔记执行计划
status: planned
version: v1.1
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/020-Obsidian日报笔记执行计划.md
prd: docs/prd/005-报告与知识库需求.md
design: docs/design/005-报告与知识库设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-005-S04-T01 第一阶段
depends_on: ["018"]
---

# Plan 020：Obsidian 日报笔记

## 安全写入、触发与测试合同

修改 `backend/app/knowledge/obsidian.py`、`services.py`、`schemas.py`、`models.py`、`worker/scheduler.py`；仅通过独立扫描final日报创建 `knowledge.export`，operation按对象ID/version/hash固定，硬截止60秒，不在日报事务里写文件。`knowledge_exports` 的owner/object_type/object_id映射复用现有表；需要保存exported_version和失败原因时同改schema，任务失败仍由Job持有。

路径为 `HotKey/日报/YYYY-MM-DD <净化主题名>-<短ID>.md`，对象已有映射则稳定沿用，主题改名不制造重复文件；净化分隔符/控制字符/限长后校验realpath仍在HotKey根内，拒绝任意父目录符号链接和文件符号链接。frontmatter按Design005字段生成，未知值保留null；仅替换唯一合法hotkey管理区块，用户区块与未知frontmatter字段保留，重复/损坏标记返回冲突不覆盖全文。

写前读取现文件hash，写同目录临时文件并fsync，替换前重新比对原文件hash；并发编辑发现变化返回conflict，不丢用户输入。rename后刷新目录持久性，再写export记录；文件写成功但DB失败的重试通过对象ID与内容hash修复记录，不重复追加正文。源内容hash应仅表示HotKey管理内容，不因用户笔记变化重写用户区。

- [ ] CHK-020-101 → SEC/OPS：扩展 `backend/tests/unit/test_obsidian_export.py`，覆盖../、绝对路径、符号链接、同名主题、损坏标记、用户区/未知frontmatter、hash不变和替换前并发编辑。
- [ ] CHK-020-102 → DATA/JOB：新增 `backend/tests/integration/test_knowledge_exports.py`，验证文件成功DB失败重放、只读vault、磁盘失败、导出取消以及日报事务不回滚。
- [ ] CHK-020-103 → AC-005-006/008：现有真实vault连续3个自然日日报文件，手工加“我的笔记”后重导出原文保留，引用只指原帖或确实存在的目标笔记。

运行B门禁及真实文件系统/库验证。证据M4 Acceptance Plan020；系统路径与内容只在受控验收记录中展示，日志不输出vault正文。失败保留原文件，回退只关闭导出扫描。

## 范围与需求

本卡承接 [PRD 005](../prd/005-报告与知识库需求.md) FR-005-006、NFR-005-002和AC-005-006/008的日报笔记部分；依赖018技术存档，产品联验需连续三自然日日报。按本文安全文件合同只写现有vault的HotKey目录，021/022分别扩展其他对象。

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
