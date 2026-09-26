---
layer: Plan
scope: issue
doc_no: "025"
title: SMTP 报告邮件执行计划
status: planned
version: v1.1
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/025-SMTP报告邮件执行计划.md
prd: docs/prd/006-推送需求.md
design: docs/design/006-推送设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-006-S01-T01
depends_on: ["024", "044", "019"]
---

# Plan 025：SMTP 报告邮件

## 适配器、启用门槛与真实用例

新增 `backend/app/notifications/smtp.py`，使用标准库smtplib和email.message；修改 `notifications/executor.py`、`schemas.py`、`core/config.py`、`.env.example`（只加空值/说明）和Worker装配。secret引用只允许服务器配置白名单的SMTP主机/端口/TLS方式/用户名/密码，严格验证证书，禁止悄悄降级明文。收件人/主题拒绝CRLF；生成text/plain和安全HTML替代内容，包含固定report_id/version/时间窗/真实可访问链接。

外部总调用受60秒Job硬截止，连接/读写各有更短超时。邮件固定Message-ID关联delivery identity，不能依赖SMTP去重保证；DATA前明确拒绝可failed，DATA已发送后断连/超时unknown，部分收件人接受须按接收方独立记录，不能一封批量发后全部重试。SMTP 250仅证明服务器接受，不是接收方已收到。

- [ ] CHK-025-101 → SEC/JOB：新增 `backend/tests/unit/test_smtp_notifications.py`，覆盖TLS失败、认证拒绝、CRLF、DATA前后超时、部分接收、模板版/中文内容和Message-ID稳定性。
- [ ] CHK-025-102 → DATA：`backend/tests/integration/test_notification_delivery.py` 接受受控SMTP服务器验证状态/回写/重放无重复外部发送，来源/Codex故障/vault只读不影响final报告发送。
- [ ] CHK-025-103 → AC-006-001：用户明确SMTP主机、凭据配置、获准收件人和发送启用后，连续3自然日日报在各设定时间+15分钟内真实收到，真实周一09:15前收到周报；接收方时间/内容/版本/降级标记留证。未具备条件只完成技术步骤，真实验收明确blocked。

运行B及真实SMTP隔离测试，044页面F门禁复用；结果归M5 Acceptance Plan025，不能将受控邮件服务器计作真实收件箱产品通过。

## 范围与需求

本卡承接 [PRD 006](../prd/006-推送需求.md) FR-006-002、NFR-001-106/107和AC-006-001的SMTP部分，配置FR-006-001由044提供。按本文适配器合同实施；用户未明确获准接收方和私有SMTP配置前，只允许受控实现和验证，真实验收阻塞。

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
