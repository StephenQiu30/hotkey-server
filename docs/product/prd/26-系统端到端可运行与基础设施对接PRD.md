---
layer: PRD
doc_no: "26"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:infra area:api area:frontend"
purpose: "将 HotKey 从契约原型推进到端到端可运行状态，后端接真实 PostgreSQL/Redis，Web 前端接真实 API，提供 Docker 部署能力。"
canonical_path: "docs/product/prd/26-系统端到端可运行与基础设施对接PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/engineering/1-Go后端重建与开源仓库治理设计.md
  - docs/superpowers/specs/2026-05-28-system-running-design.md
outputs:
  - 系统端到端可运行需求边界
  - 系统端到端可运行验收标准
triggers:
  - "基础设施对接范围变更"
  - "对应 issue 拆分或合并"
downstream:
  - docs/plans/31-系统端到端可运行与基础设施对接实现计划.md
  - docs/plans/32-Web前端接真实API实现计划.md
  - docs/plans/33-Docker部署能力实现计划.md
---

# 26-系统端到端可运行与基础设施对接 PRD

## 1. 背景

HotKey Go 后端重构已完成 P0-P3 全部契约层工作：17 个领域服务、49 个 API 端点、完整 OpenAPI 规范和数据库 Schema。但所有服务使用内存存储（Go map），重启即丢失；前端全部使用硬编码 mock 数据，未调用任何后端 API。系统目前无法实际使用。

本 PRD 将系统从"契约原型"推进到"端到端可运行"，使创作者能通过 Web 工作台使用热点监测核心功能。

## 2. 目标

1. 后端连接真实 PostgreSQL 和 Redis，核心数据持久化
2. 重构为标准 Go 分层项目结构（model/service/repo/handler）
3. 实现认证中间件、CORS、请求日志、优雅关闭
4. Web 前端通过 `@umijs/openapi` 生成客户端，接通真实 API
5. 提供 Docker Compose 一键部署能力

## 3. 范围

### 3.1 后端基础设施（PRD 26 → Plan 31）

- 重构项目结构：model/、service/、repo/、handler/、store/、middleware/、infrastructure/
- PostgreSQL 连接池（pgx/v5）和 Redis 客户端（go-redis/v9）初始化
- 4 个核心服务实现 PG repository：keyword、source、content、hotspot
- 数据库迁移工具（golang-migrate）
- 认证中间件（JWT BearerAuth）
- CORS 中间件
- 请求日志中间件（slog）
- 优雅关闭（SIGINT/SIGTERM）
- 其余 13 个服务保持内存实现，接口不变

### 3.2 Web 前端接真实 API（PRD 26 → Plan 32）

- 使用 `@umijs/openapi` 从后端 OpenAPI 规范生成 TypeScript 客户端
- 请求层重构（axios 封装 + token 自动注入 + 401 拦截）
- 认证体系（zustand store + useAuth hook）
- 页面路由拆分（登录、热点榜单、热点详情、关键词管理、日报、设置）
- 数据获取（swr + 生成的 API 函数）
- 组件拆分（从 CreatorWorkbench 巨型组件拆分为独立组件）
- UI 设计使用 `/frontend-design` 技能，兼具灵动和美观
- 引入第三方库：swr、zustand、react-hook-form、zod、axios、sonner、recharts

### 3.3 Docker 部署能力（PRD 26 → Plan 33）

- 后端 Dockerfile（多阶段构建）
- 前端 Dockerfile（多阶段构建）
- docker-compose.yml（pgvector:pg16 + redis:7-alpine + server + web）
- 根目录 Makefile（up/down/build/logs/dev-server/dev-web）
- 环境变量模板（.env.example）

## 4. 非目标

- 不对接 P1-P3 非核心服务的 PostgreSQL（tenant、rbac、billing、eventgraph、realtime、workqueue 等保持内存实现）
- 不对接小程序真实 API
- 不运行 n8n 工作流
- 不集成 AI/LLM（DashScope 调用）
- 不启用 pgvector 向量相似度召回
- 不实现实际数据采集（RSS/web scraping）
- 不拆分 Worker 进程
- 不引入 Playwright E2E 测试

## 5. 用户故事

1. 作为创作者，我启动 `docker compose up` 后可以访问 Web 工作台，查看热点榜单
2. 作为创作者，我可以用邮箱登录，登录态在页面刷新后保持
3. 作为创作者，我可以查看热点详情，包括证据链和 AI 摘要
4. 作为创作者，我可以管理关键词（关注/屏蔽/添加），设置在重启后保持
5. 作为创作者，我可以查看每日热点日报
6. 作为开发者，我可以在本地用 `go run` 启动后端，连接本地 PG/Redis 进行开发
7. 作为开发者，我修改前端代码后可以热重载查看效果

## 6. 数据与 API 边界

- 数据模型以 `db/schema.sql` 为事实源，迁移文件从该 schema 派生
- API 以 `hotkey-server` 导出的 OpenAPI 为事实源
- 前端通过 `@umijs/openapi` 生成客户端，不得手写后端 API 类型
- 核心 4 个服务（keyword、source、content、hotspot）的 PG repository 必须覆盖现有内存实现的全部接口
- 认证使用 JWT BearerAuth，与 OpenAPI 声明一致

## 7. 验收标准

### 后端

- `go run ./cmd/server` 启动成功，`/healthz` 返回 200
- PG 连接正常，`/healthz` 包含数据库连接状态
- Redis 连接正常，`/healthz` 包含 Redis 连接状态
- 数据持久化：创建关键词 → 重启后端 → 关键词仍存在
- 认证生效：无 token 请求受保护接口返回 401
- `go test ./...` 全绿
- 项目结构符合标准 Go 分层（model/service/repo/handler/store/middleware）

### Web 前端

- `npm run dev` 启动成功，页面可访问
- 登录成功后跳转工作台，token 持久化到 localStorage
- 热点榜单展示真实数据（来自后端 PG）
- 热点详情展示证据链和 AI 摘要
- 关键词管理（关注/屏蔽/添加）功能正常，刷新后保持
- `tsc --noEmit` 无报错
- `npm run build` 构建成功

### Docker

- `docker compose up` 全部 4 个服务健康启动
- 通过 `http://localhost:3000` 可访问 Web 工作台
- 通过 `http://localhost:18080/healthz` 可检查后端健康
- 数据持久化：`docker compose down && docker compose up` 后数据不丢失

## 8. 风险与降级

- PG 连接失败时后端拒绝启动，返回明确错误信息
- Redis 连接失败时降级为内存模式，记录警告日志
- 前端 API 调用失败时展示错误态，不崩溃
- Docker 环境变量缺失时使用 `.env.example` 中的默认值

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-28 | StephenQiu30 | 1.0.0 | 初版，系统端到端可运行与基础设施对接 |
