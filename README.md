# hotkey-server

X 热点监控平台后端服务。提供 API、内容检索、主题聚类、趋势分析、告警通知与数据库访问。

## 项目架构

```
┌────────────────────────────────────────────────────────────┐
│                     cmd/hotkey (main)                      │
│                   go.uber.org/fx DI                        │
├────────────────────────────────────────────────────────────┤
│  ┌─ 应用组合 ───────────────────────────────────────────┐  │
│  │  internal/fxapp        Fx App 装配、HTTP Server、     │  │
│  │                        生命周期钩子                   │  │
│  │  internal/module       基础设施模块：Config, DB,      │  │
│  │                        Redis 的 Fx Provide            │  │
│  └───────────────────────────────────────────────────────┘  │
│  ┌─ 业务领域 (domain 接口 + 纯逻辑) ────────────────────┐  │
│  │  auth        账号注册/登录（bcrypt + JWT）             │  │
│  │  monitor     关键词监控 CRUD                          │  │
│  │  notify      通知服务（站内通知 + 邮件）              │  │
│  │  hotevent    热点事件查询、热度计算、趋势判定         │  │
│  │  content     内容标准化与命中评分                     │  │
│  │  topic       Jaccard 主题聚类                         │  │
│  │  trend       趋势快照构建与速度计算                   │  │
│  │  config      环境变量配置加载                         │  │
│  └───────────────────────────────────────────────────────┘  │
│  ┌─ 持久化层 ──────────────────────────────────────────┐  │
│  │  internal/repository/gormimpl  GORM 实体实现          │  │
│  │    ├── model.go         26 个 GORM 模型 ←→ 26 张表   │  │
│  │    ├── user.go          auth.Repository 实现          │  │
│  │    ├── monitor_repo.go  monitor.Repository 实现       │  │
│  │    ├── notify_repo.go   notify.Repository 实现        │  │
│  │    └── hot_event.go     hotevent.Repository 实现      │  │
│  │                                                       │  │
│  │  internal/database      GORM 查询服务 + 初始化        │  │
│  │    ├── database.go      Open（连接 + 自动建库）       │  │
│  │    ├── bootstrap.go     EnsureReady（初始化）          │  │
│  │    ├── contentquery.go  content.PostQueryService 实现 │  │
│  │    ├── topicquery.go    topic.TopicQueryService 实现  │  │
│  │    └── trendquery.go    trend.TrendQueryService 实现  │  │
│  └───────────────────────────────────────────────────────┘  │
│  ┌─ HTTP 平台层 ───────────────────────────────────────┐  │
│  │  internal/platform/http    Gin HTTP API               │  │
│  │    ├── router.go           路由注册                    │  │
│  │    ├── middleware.go       JWT 认证、请求 ID、         │  │
│  │    │                       Context Metadata            │  │
│  │    ├── response.go         统一响应格式                │  │
│  │    ├── auth.go             注册 / 登录                 │  │
│  │    ├── monitor.go          CRUD                       │  │
│  │    ├── notify.go           通知列表 / 已读             │  │
│  │    ├── content.go          内容查询                    │  │
│  │    ├── topic.go            主题查询                    │  │
│  │    ├── trend.go            趋势查询                    │  │
│  │    ├── trending.go         热点事件查询                │  │
│  │    ├── health.go           健康检查                    │  │
│  │    └── errors.go           错误码                      │  │
│  │                                                       │  │
│  │  internal/platform/logging  结构化日志                 │  │
│  │  internal/platform/runtime 运行时上下文                │  │
│  └───────────────────────────────────────────────────────┘  │
│  ┌─ 工具包 ─────────────────────────────────────────────┐  │
│  │  internal/pkg                                         │  │
│  │    ├── array.go     Int64Array（PostgreSQL bigint[]） │  │
│  │    └── jsonb.go     JSONB[T] 泛型扫描器              │  │
│  └───────────────────────────────────────────────────────┘  │
├────────────────────────────────────────────────────────────┤
│  db/                        数据库 DDL                     │
│    ├── schema.sql           DDL 事实源（26 张表）          │
│    └── migrations/          goose 迁移                    │
├────────────────────────────────────────────────────────────┤
│  tests/                     测试                           │
│    ├── unit/                单元测试                       │
│    └── integration/         集成测试                       │
└────────────────────────────────────────────────────────────┘
```

## 文件夹结构

```
.
├── .env.example                         # 环境变量模板
├── .github/workflows/ci.yml             # CI: build + vet + test + schema + smoke
├── Dockerfile
├── Makefile
├── docker-compose.yml
├── go.mod / go.sum
│
├── cmd/
│   └── hotkey/main.go                   # 唯一入口
│
├── internal/
│   ├── auth/                             # 账号领域
│   │   ├── model.go                     # User, RegisterInput, LoginInput
│   │   ├── repository.go                # Repository 接口
│   │   └── service.go                   # Register, Login 业务逻辑
│   ├── config/
│   │   └── config.go                   # Viper 配置加载
│   ├── content/
│   │   └── query.go                    # PostQueryService 接口
│   ├── database/
│   │   ├── bootstrap.go                # EnsureReady（自动建库）
│   │   ├── database.go                 # Open（GORM 连接）
│   │   ├── contentquery.go             # 内容查询（SQL builder）
│   │   ├── topicquery.go               # 主题查询
│   │   └── trendquery.go               # 趋势查询
│   ├── fxapp/
│   │   └── app.go                      # Fx App 组合
│   ├── hotevent/
│   │   ├── errors.go                   # ErrNotFound
│   │   ├── model.go                    # HotEvent, EventPlatform, constants
│   │   ├── queryservice.go             # 查询服务 + HotEventManager 接口
│   │   ├── repository.go               # Repository 接口 + ListFilter
│   │   └── service.go                  # ComputeHeatScore, DetermineTrend（纯函数）
│   ├── module/
│   │   └── infra.go                    # Fx Module: Config, DB, Redis
│   ├── monitor/
│   │   ├── model.go                    # Monitor, CreateMonitorInput
│   │   ├── repository.go               # Repository 接口
│   │   └── service.go                  # CRUD + 校验
│   ├── notify/
│   │   ├── mailer.go                   # 邮件发送
│   │   ├── model.go                    # Notification
│   │   ├── repository.go               # Repository 接口
│   │   └── service.go                  # ListUnread, MarkRead
│   ├── pkg/
│   │   ├── array.go                    # Int64Array（bigint[] 扫描器）
│   │   └── jsonb.go                    # JSONB[T] 泛型扫描器
│   ├── platform/
│   │   ├── http/                       # Gin HTTP API（路由 + handler）
│   │   ├── logging/                    # 结构化日志
│   │   └── runtime/                    # 运行时上下文
│   ├── repository/gormimpl/            # GORM 仓库实现
│   │   ├── model.go                    # 26 个 GORM 模型
│   │   ├── user.go
│   │   ├── monitor_repo.go
│   │   ├── notify_repo.go
│   │   └── hot_event.go
│   ├── topic/
│   │   ├── query.go                    # TopicQueryService 接口
│   │   └── service.go                  # Cluster, JaccardSimilarity, ExtractTokens
│   └── trend/
│       ├── query.go                    # TrendQueryService 接口
│       └── service.go                  # ComputeVelocity, BuildSnapshot（纯函数）
│
├── db/
│   ├── schema.sql                      # DDL 事实源（26 张表）
│   └── migrations/
│       └── 000001_create_all_tables/   # goose 迁移
│
├── scripts/
│   ├── smoke-api.sh                    # API smoke 测试
│   ├── validate-schema.sh              # Schema 完整性检查
│   ├── validate-architecture-boundaries.sh  # 分层边界检查
│   ├── validate-repository.sh
│   ├── dev.sh
│   ├── start-local.sh
│   ├── apply-schema.sh
│   └── load-env.sh
│
└── tests/
    ├── unit/                           # 单元测试
    │   ├── auth/
    │   ├── config/
    │   ├── database/
    │   ├── monitor/
    │   ├── notify/
    │   ├── platform/http/
    │   ├── platform/logging/
    │   ├── topic/
    │   └── trend/
    ├── integration/api/                # 集成测试（需真实 DB）
    └── testutil/                       # 测试辅助
        ├── db.go
        ├── env.go
        ├── router.go
        └── fake/                       # mock 实现
```

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.26 |
| HTTP 框架 | Gin |
| ORM | GORM v2 |
| DI | go.uber.org/fx |
| 数据库 | PostgreSQL 16 |
| 缓存 | Redis |
| 认证 | bcrypt + JWT |
| Schema 管理 | goose 迁移 |
| 配置 | Viper（.env + 环境变量） |

## 依赖注入（Fx）

```
module.Infra (Config → DB, Redis)
    │
    ├── gormimpl.NewUserRepo        → auth.Repository
    ├── gormimpl.NewMonitorRepo     → monitor.Repository
    ├── gormimpl.NewNotifyRepo      → notify.Repository
    ├── gormimpl.NewHotEventRepo    → hotevent.Repository
    │
    ├── database.NewContentQuerySvc → content.PostQueryService
    ├── database.NewTopicQuerySvc   → topic.TopicQueryService
    ├── database.NewTrendQuerySvc   → trend.TrendQueryService
    │
    ├── auth.NewService             → *auth.Service
    ├── monitor.NewService          → *monitor.Service
    ├── notify.NewService           → *notify.Service
    ├── hotevent.NewQueryService    → http.HotEventManager
    │
    └── fxapp.NewHTTPServer         → *http.Server
```

## 本地开发

```bash
# 安装依赖
go mod download

# 运行测试
make test

# 静态检查
make lint

# 构建
make build

# 本地开发（需本地 PostgreSQL）
make dev

# Docker 环境
make up

# 完整验证（构建 + vet + 测试 + 架构边界 + schema + API smoke）
make ci
```

## API 路由

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 健康检查 |
| POST | `/api/v1/auth/register` | 注册 |
| POST | `/api/v1/auth/login` | 登录 |
| GET | `/api/v1/monitors` | 监控列表 |
| POST | `/api/v1/monitors` | 创建监控 |
| GET | `/api/v1/monitors/:id` | 监控详情 |
| PATCH | `/api/v1/monitors/:id` | 更新监控 |
| GET | `/api/v1/monitors/:id/posts` | 监控内容 |
| GET | `/api/v1/monitors/:id/topics` | 监控主题 |
| GET | `/api/v1/monitors/:id/trends` | 监控趋势 |
| GET | `/api/v1/topics/:id/trends` | 主题趋势 |
| GET | `/api/v1/notifications` | 通知列表 |
| POST | `/api/v1/notifications/:id/read` | 标记已读 |
| GET | `/api/v1/trending` | 热门趋势 |
| GET | `/api/v1/hot-events` | 热点事件列表 |
| GET | `/api/v1/hot-events/:id` | 热点事件详情 |
| GET | `/api/v1/hot-events/:id/posts` | 热点事件内容 |
| GET | `/swagger/*any` | Swagger 文档 |

## Agent 规范

- `CLAUDE.md` — Claude 协作规范
- `CLAUDE.local.md` — 本项目局部配置
- `WORKFLOW.md` — Symphony / Linear 调度契约
- `.claude/agents/` — 角色定义
- `.claude/skills/` — 可复用工作流
