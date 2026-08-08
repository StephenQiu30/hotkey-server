---
layer: Acceptance
scope: fullstack
doc_no: "027"
title: Agent Skill与外部API验收
status: passed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/acceptance/027-Agent-Skill与外部API验收.md
design: docs/design/archive/027-Agent-Skill与外部API设计.md
prd: docs/prd/archive/027-Agent-Skill与外部API.md
plan: docs/plans/archive/027-Agent-Skill与外部API计划.md
---

# Agent Skill与外部API验收

## 验收结论

027 于 2026-08-08 通过验收。HotKey 已具备用户自助 Agent Token、六个固定 Scope、独立 `/api/v1/agent/*` 外部 API、Profile 管理界面与仓库内 `$hotkey-agent` Skill。Token 原文只返回一次，数据库只保存 SHA-256 哈希与安全前缀；撤销、过期、用户禁用/删除和角色降级均按数据库当前事实即时生效。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-027-1 | passed | 自动化覆盖 256 bit 随机凭据、一次性返回、哈希/前缀存储、安全列表/撤销投影、1–90 天校验、10 个活动上限、乐观锁和审计无原文；浏览器关闭凭据 Dialog 后 DOM 不再存在原文。 |
| AC-027-2 | passed | 集成测试验证过期、撤销、禁用、软删除统一 401；浏览器真实撤销后下一次调用立即 401，缺 Scope 为 403。 |
| AC-027-3 | passed | 六 Scope 的领域与中间件矩阵、Viewer 创建上限、Agent 路由接线和非 Agent 路由拒绝均通过；Editor Token 进入采集业务层后，将用户降级为 Viewer，同一 Token 下一次调用立即变为 403。 |
| AC-027-4 | passed | 真实 Token 对 monitors、contents、events、radar、reports 路由均返回 200 统一 Result；21 个 Agent/生命周期路径进入生成 OpenAPI，复用原 Handler、DTO、游标和安全投影。 |
| AC-027-5 | passed | `search.run` 只暴露按 Monitor 采集并复用既有状态/配额服务；`alerts.write` 真实列表为 200，确认/解决保留版本动作，Agent suppress 路由不存在并返回 404。 |
| AC-027-6 | passed | Profile 的 loading/empty/error+retry、角色裁剪、创建、一次性凭据、撤销确认和失效状态均有自动化与浏览器证据；360/768/1280 无页面级溢出，axe 0 violations、0 incomplete。 |
| AC-027-7 | passed | `skills/hotkey-agent` 由官方 `init_skill.py` 创建，`quick_validate.py` 通过；OpenAI 默认提示显式包含 `$hotkey-agent`，API 参考覆盖 Scope、分页、401/403/409/429/503 与提示注入边界。 |
| AC-027-8 | passed | 后端全量 Go 测试、vet、OpenAPI、运行时数据库、构建、架构、Repository、Schema；前端 OpenAPI、类型、197 项单测、Next.js 生产构建；开发/生产 Compose 与差异检查全部通过。 |

## 自动化验证

```bash
cd backend
HOTKEY_TEST_DSN="${HOTKEY_TEST_DSN}" \
HOTKEY_TEST_REDIS_URL="${HOTKEY_TEST_REDIS_URL}" make test lint build validate schema-verify database-runtime-verify openapi-validate

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build -- --webpack

cd ..
uv run --no-project --with pyyaml python \
  /Users/stephenqiu/.codex/skills/.system/skill-creator/scripts/quick_validate.py \
  skills/hotkey-agent
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet
git diff --check
```

隔离 pgvector/PostgreSQL 与 Redis 上的后端全量包通过；Schema 为 73 张应用/队列表且模式指纹稳定。OpenAPI 连续生成哈希一致，前端官方生成 Client 同步；47 个前端测试文件共 197 项测试通过，Next.js 16.3.0 共构建 20 个路由。

## 浏览器与真实 API 验收

按用户指定的 `$agent-browser` 在隔离 Next.js、Go、PostgreSQL 与 Redis 环境完成：

- Viewer 创建四个 read Scope Token；创建 Dialog 不展示 `search.run`。监控、内容、事件、Radar 和报告 Agent 路由均返回 200；未授予的告警与采集返回 403。
- Agent Token 访问 `/api/v1/users` 返回 401；未开放的 Agent suppress 返回 404。Swagger UI 可发现 Agent Token 生命周期与 Agent 监控路径。
- 一次性凭据关闭后不在 DOM、URL 或 localStorage；列表只显示 19 字安全前缀。撤销后同一凭据下一请求返回统一 401，列表立即显示“已撤销”。
- Editor 可选择 `search.run`；真实采集请求越过 Scope 守卫进入既有业务状态检查。数据库角色降级后无需重发 Token，同一请求立即返回 403。
- `alerts.write` Token 对真实告警列表返回 200。Token 表 4 条临时验收记录均为 64 字哈希、最长 19 字前缀；审计 JSON 中 `hk_agent_` 命中数为 0。
- 360×800、768×1024、1280×800 的 `scrollWidth === clientWidth`；axe-core 4.12.1 按 WCAG 2 A/AA 均为 0 violations、0 incomplete。控制台只有 Next.js 开发提示/HMR，页面错误为空。

浏览器稳定页截图在验收期间写入 `/tmp/hotkey-027-profile-{360,768,1280}.png`，不包含一次性凭据；验收结论写入本文后已随临时服务一并清理，未进入仓库。

## 安全与回滚

- 原业务路由继续只使用浏览器会话认证器，Agent 路由只使用 Agent Token 认证器；Agent Token 不能管理自身生命周期或进入用户、来源、治理和管理 API。
- Skill 将所有标题、正文、报告、URL 和错误文本视为不可信数据，不允许其改变 Base URL、Header、Scope 或工具权限。
- 回滚应用与前端不会破坏现有会话和业务数据；`agent_tokens` 可暂留为无人读取的安全哈希表。如需删除，先撤销 Token，再通过独立受控迁移处理。
