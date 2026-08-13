---
layer: Acceptance
scope: shared
doc_no: "038"
title: 邮件与WebSocket双通道通知收敛验收
status: passed
version: v1.0
owner: HotKey Team
canonical_path: docs/acceptance/038-邮件与WebSocket双通道通知收敛验收.md
design: docs/design/038-邮件与WebSocket双通道通知收敛设计.md
prd: docs/prd/038-邮件与WebSocket双通道通知收敛.md
plan: docs/plans/038-邮件与WebSocket双通道通知收敛计划.md
---

# 邮件与 WebSocket 双通道通知收敛验收

## 结果

038 于 2026-08-13 完成代码与自动化验收。默认用户通知现只有站内 WebSocket 实时通知和 `high/urgent` 邮件；REST `after_id` 负责断线恢复。SSE、Web Push API、设备订阅 UI、VAPID 配置及 Web Push Worker 已退出运行时，旧数据库结构保留兼容历史数据。

## 验收矩阵

| 项目 | 证据 | 结果 |
|---|---|---|
| 统一通知事实 | WebSocket 与 Email 都消费 `user_notifications`，各自追加不可变 delivery attempt | passed |
| WebSocket | 首帧 Token 鉴权、子协议、游标重放、heartbeat、容量与安全帧测试 | passed |
| 断线补齐 | 前端单元测试模拟 WebSocket 失败，确认立即调用 REST 当前游标、无重复 Toast | passed |
| 邮件门槛 | 当前用户/Monitor/发布配置重检，只有 high/urgent 可领取 | passed |
| 邮件内容 | 标题去标签，HTML 转义；Monitor、来源、87.5% 相关度、原文与站内链接测试 | passed |
| 邮件失败 | SMTP 临时、永久和尝试耗尽分别记录有限重试/永久失败 | passed |
| 页面 | 两张通道卡展示实时状态、账户邮箱和邮件管理入口；无 Web Push 设备文案 | passed |
| 公开契约 | OpenAPI 只保留通知列表和 WebSocket upgrade；生成 Client 无 SSE/Push | passed |
| 历史兼容 | 不删除旧 Web Push 表、旧通道枚举和已有投递事实 | passed |

## 自动化命令

以下命令已通过：

```text
cd backend && go test ./internal/... ./cmd/...
cd backend && go run ./test/runner test ./internal/modules/notification/application ./internal/modules/notification/domain ./internal/modules/notification/transport/http ./internal/modules/notification/infrastructure/jobs ./internal/platform/config ./internal/platform/http -count=1
cd backend && go test ./test/architecture -run 'Test(OpenAPIContract|GeneratedOpenAPIRegistryMatchesCommittedArtifact)$' -count=1
cd backend && HOTKEY_TEST_DSN=... HOTKEY_TEST_REDIS_URL=... go run ./test/runner test ./internal/bootstrap -run 'TestAPIFxGraphRegistersExactDocumentMatchRoutes$' -count=1
cd backend && HOTKEY_TEST_DSN=... HOTKEY_TEST_REDIS_URL=... go run ./test/runner test -tags=integration ./internal/modules/notification/infrastructure/postgres -run 'Test(ContentBindingProjectsOneMonitorScopedHotspotNotificationAndHighEmail|EmailDeliveryRepositoryClaimsWithCurrentPermissionBackoffAndTerminalAttempt)$' -count=1
cd frontend && npm run openapi:generate
cd frontend && npm run openapi:check
cd frontend && npm run typecheck
cd frontend && npm run test:unit
cd frontend && npm run build
```

前端全量 41 个测试文件、185 个测试通过，Next.js 生产构建生成 20 条路由；OpenAPI 契约和生成 Client 无漂移。PostgreSQL 集成测试使用一次性 `pgvector/pgvector:pg16` 与 `redis:7-alpine` 容器通过，结束后容器和临时数据库已删除。

## Agent Browser 真实服务验收

2026-08-13 使用 `agent-browser` 对生产构建容器执行端到端回归，完整报告位于 `dogfood-output/notification-038/report.md`：

| 场景 | 结果 |
|---|---|
| 页面双通道收敛 | 只显示站内 WebSocket 与邮件通知；旧 SSE/Web Push 请求为 0 |
| WebSocket 实时送达 | 页面保持打开时收到新卡片与 toast，delivery attempt 为 `succeeded` |
| 邮件真实投递 | Mailpit 通过 STARTTLS 接收 `urgent` 与 `high` 邮件，email attempt 为 `succeeded` |
| 邮件正文 | 包含监控、来源、97.5% 相关度、原文和站内链接 |
| REST 断线补拉 | 阻断 WebSocket 后显示“REST 补拉中”，从 `after_id=3` 补到 ID 4 |
| 故障恢复 | 恢复 WebSocket 后重新显示“WebSocket 已连接” |
| 最终浏览器健康度 | 清空网络日志后重新加载：失败请求 0、浏览器错误 0 |

初次浏览器验收发现运行中的前端与后端镜像落后于当前源码；重新构建两个镜像后复验通过，未发现仍开放的问题。

## 已知限制

- Hacker News 来源探测被验收容器的出口策略拒绝；通知事件使用与用户、监控、来源、内容和相关度绑定的确定性夹具产生，REST、WebSocket、数据库投递记录与 SMTP 均走真实服务边界。
- 页面关闭后依靠邮件，不承诺浏览器系统 Push。这是明确产品边界，不是降级故障。

## 回滚验证

- 停止 WebSocket 后，通知列表仍可通过 REST 游标读取。
- 禁用 SMTP 后，热点与站内通知继续提交，邮件 Worker 不影响业务事务。
- 本轮没有删除历史表或数据，无数据恢复步骤。
