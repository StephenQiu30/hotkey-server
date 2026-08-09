---
layer: Acceptance
scope: shared
doc_no: "031"
title: Hacker-News热门榜单与持续观测验收
status: passed
version: v1.0
owner: HotKey Team
canonical_path: docs/acceptance/031-Hacker-News热门榜单与持续观测验收.md
design: docs/design/archive/031-Hacker-News热门榜单与持续观测设计.md
prd: docs/prd/archive/031-Hacker-News热门榜单与持续观测.md
plan: docs/plans/archive/031-Hacker-News热门榜单与持续观测计划.md
---

# Hacker-News热门榜单与持续观测验收

## 结果

031 于 2026-08-09 通过 P0 验收。管理员可在登录后通过来源 UI 创建 Hacker News 热门、最佳或最新来源；热门榜单会重复读取官方候选、刷新 Content 指标并进入既有监控匹配、事件与通知链路。

## 自动化证据

- 测试先行覆盖 Top/Best 正确端点、榜单顺序、重复观测、部分失败、历史 `new` 默认、配置枚举、UI 默认 Top、标准化分页排空与未知元数据中性处理。
- 后端 `make lint database-runtime-verify test build validate schema-verify`：通过；测试数据库初始化为 74 张表，运行态与 schema 指纹验证通过，全量 Go 测试通过。
- 前端 `npm run typecheck`、`npm run openapi:check`、`npm run test:unit`、`npm run build`：通过；48 个测试文件、204 个测试与 21 个 Next.js 路由通过。
- `git diff --check`：通过；OpenAPI 生成客户端与服务端契约一致。

## 真实运行证据

- 通过 `/dashboard/sources` 创建启用状态的“`Hacker News 热门榜单`”，健康探测为健康，配置使用官方固定 API 与 `top` 模式。
- 多轮真实 Top 采集均成功；最后一轮 CollectionRun 接收 100、接受 100、拒绝 0，NormalizeJob 完成且无错误。
- 来源形成 94 条去重 Content 和 420 个指标快照；94 条内容均至少被重复观测两次，证明 score/comment 持续刷新链路成立。
- 通过 `/dashboard/settings` 创建并发布“`HN 热点事件监控`”。关键词 `server` 与实体 `phone` 将 `My server is a phone now` 判定为 accepted，相关性 100，来源、词法与实体证据可解释。
- `/dashboard/contents` 可按该监控和“已接受”筛出唯一命中；`/dashboard/events` 已形成事件事实与指标快照；`/dashboard/notifications` 显示最新采集成功和事件更新通知。浏览器控制台无 error。

## 已知限制

- 本地网络代理会把官方域名解析到保留地址；验收运行使用既有受控 DoH 配置获得真实公开地址，SSRF 校验未放宽。该项属于部署网络配置，不是用户个性化事实源配置。
- 当前本地环境未配置向量模型，事件聚类走既有 `rule` 降级路径，语义聚类质量和来源宽度仍需由 019/020 的模型与数据配置提升；031 只验收官方热门获取、持续指标与现有流水线接入，不改写聚类算法。

## 回滚

暂停或归档“`Hacker News 热门榜单`”即可停止后续观测；把模式改回 `new` 可恢复 009 的单调增量行为。已入库 Content、指标快照、CollectionRun、Event 与 Notification 均作为审计事实保留，不执行破坏性删除。
