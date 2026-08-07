---
layer: Acceptance
scope: backend
doc_no: "009"
title: Hacker-News官方来源连接器验收
status: passed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/acceptance/009-Hacker-News官方来源连接器验收.md
design: docs/design/009-Hacker-News官方来源连接器设计.md
prd: docs/prd/009-Hacker-News官方来源连接器.md
plan: docs/plans/009-Hacker-News官方来源连接器计划.md
---

# Hacker-News官方来源连接器验收

## 验收结论

009 通过验收。连接器只访问 Hacker News 官方 Firebase API，支持五类官方 item、评论/投票父项、可选公开指标、4 worker 有界并发、单调 checkpoint 和确定性故障分类。单项失败只提交首个未完成 ID 之前的连续前缀，下一次运行从失败位置重试；全部 item 请求失败时运行失败且不移动 checkpoint。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-009-1 | passed | 连接器回归覆盖连续成功前缀、部分失败后的 `next_cursor` 和下一次只请求未完成窗口；应用集成测试确认部分运行 accepted=1、rejected=1，全部请求失败不推进 checkpoint。 |
| AC-009-2 | passed | 网络回归覆盖固定官方根端点、跨主机重定向和预拨号私网 DNS 拒绝；来源创建页面把 Hacker News 接口地址锁定为官方地址。 |
| AC-009-3 | passed | 应用与 PostgreSQL 集成测试确认部分成功形成成功运行并保存安全诊断，全部 item 请求失败形成失败运行；CapturedItem v2 JSON 向后兼容地保存 `parent_external_id`。 |

## 自动化验证

```bash
cd backend
PATH=/opt/homebrew/opt/postgresql@16/bin:$PATH \
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:55432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:56379/15' make ci

go run ./test/runner test -race \
  ./internal/modules/source/infrastructure/hackernews -count=1

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod \
  -f docker-compose-prod.yml config --quiet
git diff --check
```

后端全量 CI 通过 OpenAPI 再生成一致性、vet、PostgreSQL 16 运行时与 67 表 Schema 指纹、全量测试、构建、架构和仓库门禁；Hacker News 包额外通过竞态检测。前端 OpenAPI 契约、类型检查、37 个测试文件共 136 项单元测试和 Next.js 生产构建全部通过，两套 Compose 配置有效。

## 浏览器验收

使用用户指定的 `$agent-browser` 连接真实 Next.js、Go、PostgreSQL 与 Redis：

- 未认证访问来源页会跳转到带 redirect 的登录页；管理员登录后可创建 Hacker News 来源，接口地址只读且固定为 `https://hacker-news.firebaseio.com/v0`。
- 创建结果、官方条款链接和来源类型正确显示。测试环境无法连接官方上游时，健康探测安全显示“不可用”和可操作反馈，不泄露底层网络错误。
- 查看者只能读取来源目录，新增、探测、启停和删除操作均不可见。
- 发现并修复一项中优先级问题：列表请求失败曾被误呈现为真实空态；现在持续显示 destructive Alert、错误原因和“重新加载”，恢复网络后已有来源重新出现。
- 1440×1000 桌面和 390×844 移动视口均无水平溢出；移动对话框初始焦点位于可滚动配置区，Escape 可关闭。
- axe-core 4.12.1 对来源列表执行 WCAG 2 A/AA 审计为 0 violations、0 incomplete；对话框为 0 violations，Radix focus guard 和透明背景文字仅列为需人工判断的 incomplete。清理历史热更新日志并重载后，两种角色均无页面错误，控制台只有 React 开发提示和 HMR 连接信息。

截图、录像和 dogfood 报告保存在本地 `/tmp/hotkey-009-dogfood/`，不提交为仓库事实源。

## 失败、降级与回滚

- `deleted`、`dead`、`null`、无效项目和未知类型产生安全诊断且不生成内容；它们作为已完成读取可推进连续 checkpoint。
- `score` 与 `descendants` 缺失保持未知，显式零值保持已知零值；`maxitem` 不作为传播或热度信号。
- `429` 保留 `Retry-After`；认证、限流、临时、解析和永久错误按稳定优先级选择根因，持久化信息不包含上游响应或传输原文。
- 本条不增加数据库表、列或公开 API。回滚应用版本会忽略可选 `parent_external_id` JSON 字段，不删除 CapturedItem、Content、CollectionRun 或 checkpoint。
