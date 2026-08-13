---
layer: Plan
scope: shared
doc_no: "038"
title: 邮件与WebSocket双通道通知收敛计划
status: completed
version: v1.0
owner: HotKey Team
canonical_path: docs/plans/archive/038-邮件与WebSocket双通道通知收敛计划.md
design: docs/design/archive/038-邮件与WebSocket双通道通知收敛设计.md
prd: docs/prd/archive/038-邮件与WebSocket双通道通知收敛.md
---

# 邮件与 WebSocket 双通道通知收敛计划

## 实现原则

复用现有 `user_notifications`、投递 attempt、邮件租约、SMTP Mailer、WebSocket Handler 和前端 Zustand Store。先写失败测试锁定页面与路由边界，再完成最小实现和冗余删除；不创建第二套通知模型。

## Slice 1：现状与外部实现调研

- [x] 追踪 Hotspot 提交、通知事实、WebSocket/SSE/Web Push、邮件 Worker 和页面状态完整链路。
- [x] 固定复核 Gotify、Novu 和 yupi-hot-monitor 的持久消息、实时流及邮件分层。
- [x] 决定 REST 只作为 WebSocket 恢复协议，用户可见通道严格收敛为两种。

## Slice 2：后端通道收敛

- [x] 保留经过首帧鉴权、容量限制、heartbeat 和游标重放的 WebSocket。
- [x] 删除 SSE 路由、帧输出和特殊超时豁免。
- [x] 删除 Web Push 领域/应用/基础设施/API/Fx/Worker/配置和 Go 依赖。
- [x] 新领域校验只允许 `websocket/email`，旧 Schema/历史事实不破坏性删除。
- [x] 路由测试明确 SSE 与 Web Push 端点不可达。

## Slice 3：邮件可读性与安全

- [x] 邮件领取查询投影 Monitor、来源、最新 accepted 相关度和原文 URL。
- [x] 主题展示级别、Monitor 与无标签标题；HTML 转义正文。
- [x] 原文链接只接受 HTTP(S)，HotKey 深链继续使用领域白名单。
- [x] 保留发布配置重检、2 分钟租约、最多 5 次和 1/2/4/8 分钟退避。

## Slice 4：前端双通道体验

- [x] 新增双通道状态卡，展示 WebSocket/REST 恢复状态与当前账户邮箱。
- [x] WebSocket 失败后立即 REST 补拉，再退避重连；删除 SSE 解析器。
- [x] 删除 Web Push 设备管理、浏览器订阅与 VAPID 前端代码。
- [x] Service Worker 只保留公开离线壳。

## Slice 5：契约、测试与文档

- [x] 更新通知页面、WebSocket、邮件安全投影、路由边界和 REST 降级测试。
- [x] 重新生成 Swaggo、发布 OpenAPI 和 TypeScript Client。
- [x] 更新环境模板并执行 Go module tidy。
- [x] 新增 038 Design/PRD/Plan/Acceptance 与索引。
- [x] 执行通知相关后端测试、前端全量门禁和构建；无隔离 SMTP/数据库凭据的真实集成项明确留证。

## 回滚

- WebSocket 可由前端停止连接，通知仍可通过 REST 查询；SMTP 可禁用而不影响站内事实。
- 旧 Web Push 表和历史 attempt 未删除，无需数据恢复。
- 若必须恢复旧公开端点，应从版本控制整体恢复代码、契约、配置与测试，不能只恢复 UI。
