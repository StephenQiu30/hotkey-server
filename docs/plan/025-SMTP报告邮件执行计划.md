---
layer: Plan
scope: issue
doc_no: "025"
title: SMTP 报告邮件执行计划
status: blocked
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/025-SMTP报告邮件执行计划.md
prd: docs/prd/006-推送需求.md
design: docs/design/006-推送设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-006-S01-T01
---

# Plan 025：SMTP 报告邮件

## 范围与需求

SMTP 尚未实现，凭据和启用条件待定。本卡承接 [PRD 006](../prd/006-推送需求.md) `FR-006-001/002`、`NFR-001-106/107` 和 `AC-006-001` 的 SMTP 部分；先行 Plan 018/019 的已存档报告和 Plan 024 的投递身份。用户未给出获准接收方和 SMTP 启用条件前只做受控实现，不发送真实邮件。预计新增 `notifications/` SMTP 适配器、配置与 tests，不在路由直接发送。

## SPEC

| ID | 规格 |
|---|---|
| `SPEC-025-SEC-001` | 接收方、SMTP 凭据、发件身份、预算和启用开关显式配置；凭据不入 Git、日志、前端或模型输入。 |
| `SPEC-025-JOB-001` | 只发送已定稿报告版本；成功、拒绝、超时与结果未知映射 Plan 024 状态，重复调度不重复送达。 |
| `SPEC-025-OBS-001` | 获准后逐接收方核对连续三自然日日报在设定时间后 15 分钟内及周一周报在 09:15 前真实收到，记录版本和接收时间。 |

## 验收与 Checklist

Given：报告已定稿，SMTP 凭据与接收方获准。When：受控故障验证后按日/周发送。Then：收到的版本、时间、数字与存档一致；unknown 待人工确认，来源/Codex/vault 故障不回滚已定稿报告。

- [ ] `CHK-025-G0-001`：核对 046 S03、PRD/Design、Plan 024、报告版本和启用条件。
- [ ] `CHK-025-G1-001`：冻结三项 SPEC 的凭据、接收方、状态与超时契约。
- [ ] `CHK-025-G2-001`：先用受控 SMTP 验证成功、拒绝、超时、unknown、重放和预算。
- [ ] `CHK-025-G3-001`：实现适配器与 Worker 装配，禁止传输层全局重试写操作。
- [ ] `CHK-025-G4-001`：运行 Ruff/mypy/pytest、真实 PostgreSQL/Kafka 与配置脱敏门禁。
- [ ] `CHK-025-G5-001`：获准后保存真实接收方确认、报告版本、发送/接收时间及故障隔离证据。
- [ ] `CHK-025-G6-001`：记录 `AC-006-001` SMTP 独立结论；飞书另验。

未获准前保持 blocked。失败或 unknown 时停止自动重发、保留 Outbox 与投递记录，按 Plan 024 人工确认。
