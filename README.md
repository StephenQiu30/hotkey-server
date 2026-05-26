# hotkey-server

`hotkey-server` 是 HotKey AI 实时热点监测小程序的后端仓库，当前处于 Go 后端全面重建阶段。

本仓库是跨仓规范主源，也是未来 OpenAPI 契约事实源。`hotkey-web` 和 `hotkey-miniapp` 必须以后端导出的 OpenAPI 为准生成客户端，不手写后端 API 类型。

## 当前状态

- 旧 FastAPI 运行时、Python 测试、旧 Docker/Compose、旧 SQL 初始化和旧 OpenSpec 实现内容已移除。
- 当前只保留开源治理文件、Go 重建 PRD/Plan、工程设计和 OpenSpec 配置入口。
- 新实现必须从 `docs/product/prd/` 与 `docs/plans/` 的连续编号任务开始推进。

## 目标技术栈

- Go
- Gin HTTP framework
- PostgreSQL
- pgvector
- Redis
- OpenAPI 生成/导出

## 文档入口

- [AGENTS.md](./AGENTS.md)：跨仓主规范源。
- [AGENTS.local.md](./AGENTS.local.md)：当前仓库局部补充规则。
- [docs/README.md](./docs/README.md)：Go 重建后的长期文档入口。
- [docs/engineering/1-Go后端重建与开源仓库治理设计.md](./docs/engineering/1-Go后端重建与开源仓库治理设计.md)：目标架构与任务编排规则。

## 任务编号

- `1-13`：P0 开源核心闭环。
- `14-16`：P1 平台化能力。
- `17-19`：P2 商业化与规模化能力。
- `20-22`：P3 高级实时与事件图谱。

每个任务必须同时维护：

```text
docs/product/prd/N-能力名称PRD.md
docs/plans/N-能力名称实现计划.md
```

## 本地验证

提交前至少执行：

```bash
git status --short
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
go test ./...
```

OpenAPI 可通过启动服务后访问 `/openapi.json` 导出；涉及接口变更时还需要补充端侧客户端生成验证。

## 本地启动

```bash
HOTKEY_HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/openapi.json
```

默认配置：

- `HOTKEY_HTTP_ADDR=:8080`
- `HOTKEY_DATABASE_URL=postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable`
- `HOTKEY_REDIS_URL=redis://localhost:6379/0`

## 当前 API

- `GET /healthz`
- `GET /openapi.json`
- `GET /api/v1/admin/keywords`
- `POST /api/v1/admin/keywords`
- `PATCH /api/v1/admin/keywords/{id}`
- `GET /api/v1/admin/sources`
- `PATCH /api/v1/admin/sources/{id}`
- `GET /api/v1/admin/source-items`
- `POST /api/v1/admin/source-items`
- `POST /api/v1/admin/event-candidates`
- `GET /api/v1/admin/event-clusters`
- `POST /api/v1/admin/event-evidence`
- `POST /api/v1/admin/events/{id}/ai-summary`
- `GET /api/v1/admin/task-runs`
- `POST /api/v1/admin/reports/daily`
- `POST /api/v1/admin/tenants`
- `GET /api/v1/admin/tenants`
- `POST /api/v1/admin/tenants/{id}/members`
- `GET /api/v1/admin/tenants/{id}/keywords`
- `POST /api/v1/admin/tenants/{id}/keywords`
- `GET /api/v1/admin/tenants/{id}/sources`
- `POST /api/v1/admin/tenants/{id}/sources`
- `PATCH /api/v1/admin/tenants/{id}/sources/{sourceId}`
- `POST /api/v1/admin/tenants/{id}/roles`
- `POST /api/v1/admin/tenants/{id}/authorize`
- `GET /api/v1/admin/tenants/{id}/audit-logs`
- `GET /api/v1/users/{id}/tenants`
- `GET /api/v1/events/{id}/evidence`
- `GET /api/v1/hotspots`
- `GET /api/v1/hotspots/{id}`
- `GET /api/v1/reports/daily?date=YYYY-MM-DD`
- `GET /api/v1/users/{id}/reports/daily?date=YYYY-MM-DD&keywords=OpenAI,model`
- `GET /api/v1/tenants/{id}/reports/daily?date=YYYY-MM-DD`
- `POST /api/v1/refresh-queue`
- `GET /api/v1/admin/refresh-queue`
- `GET /api/v1/admin/redis/health`
- `POST /api/v1/keywords/follow`
- `POST /api/v1/keywords/block`
- `POST /api/v1/keywords/additional`
- `GET /api/v1/keywords/preferences?userId=...`

当前关键词能力先使用进程内仓储锁定 API 行为和 OpenAPI 契约；PostgreSQL schema、pgvector 和 Redis 持久化会在后续 P0 数据与队列任务中接入。

当前来源能力先使用进程内来源注册表锁定合规字段、启停与限流契约；默认内置 `arxiv-ai` 作为国外事实源、`github-trending-ai` 作为传播源，不包含绕过授权、抓取真实 token 或规避平台限制的采集逻辑。

当前内容能力先使用进程内 SourceItem 仓储锁定标准化与去重契约；会保留原始 URL、来源、发布时间、抓取时间、内容 hash 和原始元数据，并按 canonical URL、内容 hash、标题时间窗口去重。

当前相似聚合能力先使用进程内事件簇仓储锁定 pgvector 契约；向量可用时使用余弦相似度归簇，向量不可用时退回 hash/标题规则聚合，并在响应中展示 `matchMethod` 与 `similarity`。

当前可信度能力先使用进程内证据链仓储锁定事实证据、传播证据和 AI 引用契约；低可信传播源只贡献热度，不能生成事实分，AI 总结必须携带来源引用。

当前热点能力先使用进程内热点仓储锁定列表与详情契约；列表支持关键词、地区、语言、最低可信度和 `heat` / `trust` / `relevance` 排序，详情返回关联内容、证据摘要、相似度和风险标签。

当前日报能力先使用进程内日报生成器锁定平台日报和用户关注日报契约；日报条目必须回链事件簇和证据 ID，默认 `date` 为昨日。

当前 Redis 基础能力先使用进程内实现锁定任务锁、手动刷新限流、刷新队列、短期去重和降级读契约；后续可替换为真实 Redis 客户端。

当前管理员 API 契约覆盖关键词和来源启停、任务运行与失败记录查询、管理员手动触发日报生成；任务记录先使用进程内实现锁定管理端契约，后续可替换为 PostgreSQL 任务运行表。

当前多租户能力先使用进程内租户、成员关系和租户隔离字段锁定平台化契约；用户可属于多个租户，关键词、来源和日报具备租户级隔离入口，后续与 PostgreSQL schema、RBAC 和审计日志合并演进。

当前 RBAC 与审计能力先使用进程内角色绑定、权限判定和审计事件锁定平台化契约；租户内 `owner`、`admin`、`viewer` 具备不同管理边界，关键角色和配置变更需要写入租户审计日志。

当前租户级管理员 API 扩展允许平台管理员列出租户，租户管理员通过租户路径管理本租户关键词、来源和日报入口；跨租户治理仍通过平台管理接口和 RBAC/审计契约约束。

OpenAPI 已声明 `BearerAuth` 鉴权方案和统一结构化错误响应；小程序端应从 `/openapi.json` 生成客户端，不手写后端 API 类型。
