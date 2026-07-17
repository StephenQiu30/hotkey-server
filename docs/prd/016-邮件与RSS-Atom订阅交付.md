---
layer: PRD
prd_no: "016"
doc_no: "016"
title: 邮件与RSS-Atom订阅交付
audience: [PM, Dev, QA, Ops]
feature_area: 订阅与交付
purpose: 定义邮件及 RSS、Atom 订阅交付任务
phase: P1
priority: P1
status: review
execution_status: backlog
version: v1.0
owner: HotKey Server Team
depends_on: [PRD-014, PRD-015]
design_refs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/008-Obsidian知识库治理与报告交付设计.md
  - docs/design/012-监控调度与River流水线设计.md
canonical_path: docs/prd/016-邮件与RSS-Atom订阅交付.md
inputs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/008-Obsidian知识库治理与报告交付设计.md
  - docs/design/012-监控调度与River流水线设计.md
outputs:
  - 邮件与 Feed 交付需求
triggers:
  - 订阅、邮件、Feed 或投递重试规则变化
downstream:
  - docs/plans/016-邮件与RSS-Atom订阅交付计划.md
  - docs/acceptance/016-邮件与RSS-Atom订阅交付验收.md
---

# 邮件与 RSS/Atom 订阅交付

## 目标

把已发布报告通过可重试邮件和支持条件请求的私有 RSS/Atom 订阅安全交付。

## 范围

- 实现 delivery 模块、report_subscriptions、report_deliveries、delivery_attempts。
- 支持订阅范围、频率、时区、渠道、启停和私有 Feed Token 轮换。
- 定义 MailSender 端口并实现 SMTP HTML 与纯文本邮件。
- 实现 deliver_email River Job、幂等投递、退避、永久失败和管理员重试。
- 提供 RSS 2.0 与 Atom Feed、ETag、Last-Modified 和条件请求。
- 提供订阅安全业务 API 和投递运行查询。

## 非范围

- 不实现前端订阅页面、营销群发或第三方消息渠道。
- 不允许订阅直接访问未发布草稿。
- 不把 SMTP 或 Feed Token 明文写入日志与普通列表 API。

## 功能要求

1. report_id + subscription_id 构成投递幂等键。
2. 邮件成功后重跑不得再次发送；临时错误重试，永久错误停止。
3. 每次尝试追加不可变 delivery_attempt，保留响应分类但不保存敏感正文。
4. Feed 只返回订阅允许范围内的已发布报告。
5. ETag 和 Last-Modified 与发布版本稳定对应，支持 304。
6. Token 轮换立即使旧 Token 失效，并写审计记录。
7. 取消订阅停止新投递，但保留历史审计。

## 交付物

- Delivery 领域、订阅、邮件、Feed 与管理 API。
- SMTP 适配器、HTML/纯文本模板和私有 Token 管理。
- Schema、记录模型、OpenAPI、River Job 和投递指标。
- SMTP 临时/永久错误、重复执行、Feed 304、Token 轮换和权限测试。

## 验收标准

- 同一报告和订阅最多一次成功邮件。
- 失败可追踪、可分类重试，永久失败不形成无限任务。
- RSS 与 Atom 输出有效，条件请求返回正确 304。
- Token 轮换后旧地址立即不可用。
- 邮件、日志和 API 不泄露密钥、Token 或不必要正文。

## 完成定义

知识、报告和交付形成完整 P1 链路，可由 PRD-017 统一执行故障与容量验收。
