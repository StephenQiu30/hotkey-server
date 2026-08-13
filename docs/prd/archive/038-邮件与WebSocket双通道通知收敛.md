---
layer: PRD
scope: shared
doc_no: "038"
title: 邮件与WebSocket双通道通知收敛
status: implemented
version: v1.0
owner: HotKey Team
canonical_path: docs/prd/archive/038-邮件与WebSocket双通道通知收敛.md
design: docs/design/archive/038-邮件与WebSocket双通道通知收敛设计.md
plan: docs/plans/archive/038-邮件与WebSocket双通道通知收敛计划.md
---

# 邮件与 WebSocket 双通道通知收敛

## 用户问题

用户需要在页面打开时立即看到新热点，也需要在离开页面后通过邮箱收到真正重要的信息。当前页面暴露 Web Push 设备配置，并同时维护 WebSocket、SSE 和轮询，用户难以理解，开发侧也存在重复通道和恢复逻辑。

## 产品目标

- 用户只需要理解“站内实时通知”和“邮件通知”两种方式。
- 页面在线时 WebSocket 实时送达；连接中断不丢消息，按最后消息 ID 补齐。
- 页面关闭后，启用邮件的 Monitor 对 `high/urgent` 热点发送邮件。
- 任一投递方式失败都不影响热点和站内通知事实。

## 功能需求

- FR-038-1：通知页展示站内实时与邮件两张通道卡，显示当前连接状态、账户邮箱、邮件门槛和设置入口。
- FR-038-2：站内实时连接必须在首条应用帧完成身份认证，不在 URL 暴露 Token，并只发送当前用户的通知。
- FR-038-3：浏览器保存 `lastEventID`；首次加载和每次 WebSocket 失败后调用 REST `after_id` 补拉，再有界退避重连。
- FR-038-4：WebSocket 成功写出的消息追加 `websocket/browser_ws` 投递事实；记录失败不得重复或撤回已写消息。
- FR-038-5：邮件只发送至当前账户邮箱，且只处理当前已发布 Monitor 配置允许的 `high/urgent` 通知。
- FR-038-6：邮件 Worker 使用持久租约避免并发重复，临时失败有限重试，永久失败可审计。
- FR-038-7：邮件展示级别、Monitor、来源、相关度、安全原文链接及 HotKey 详情链接；不包含凭据、对象存储键或内部 Payload。
- FR-038-8：公开 API 和默认 UI 不再提供 SSE、Web Push、设备订阅或 VAPID 配置；旧数据库记录保持可读。

## 非目标

- 网页关闭后的浏览器系统推送、移动原生 Push、短信、企业 IM 和通知 SaaS。
- 用户自定义邮件模板、摘要邮件、静默时段或按设备配置。
- 以 Kafka/Redis Stream 替换当前 PostgreSQL 持久事实。

## 验收标准

- AC-038-1：页面能无歧义地看到两种通道及真实状态，不出现 Web Push/手机设备文案。
- AC-038-2：WebSocket 连接、首帧鉴权、heartbeat、游标重放与用户隔离测试通过。
- AC-038-3：WebSocket 不可用时，浏览器立即 REST 补拉且不弹重复 Toast；恢复后继续从最新游标连接。
- AC-038-4：高/紧急邮件安全投影、SMTP 临时/永久失败、最大次数和不可变 attempt 测试通过。
- AC-038-5：OpenAPI 和 Fx 路由不包含 `/notifications/stream`、`push-capability` 或 `push-subscriptions`。
- AC-038-6：后端编译、通知相关测试、前端类型检查、单元测试、OpenAPI 漂移检查和生产构建通过。
