# hotkey-server

X 热点监控平台后端服务。提供 API、内容检索、主题聚类、趋势分析、告警通知与数据库访问。

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
│   │   ├── app.go                      # Fx App 组合
│   │   └── doc.go                      # 包文档
│   ├── hotevent/
│   │   ├── errors.go                   # ErrNotFound
│   │   ├── model.go                    # HotEvent, EventPlatform, constants
│   │   ├── queryservice.go             # 查询服务 + HotEventManager 接口
│   │   ├── repository.go               # Repository 接口 + ListFilter
│   │   └── service.go                  # ComputeHeatScore, DetermineTrend（纯函数）
│   ├── llm/
│   │   ├── provider.go                 # Provider 接口 + Option 函数选项
│   │   ├── errors.go                   # ErrProviderError / ErrEmptyResponse
│   │   ├── adapter.go                  # langchaingo → Provider 适配层
│   │   ├── factory.go                  # NewProvider 工厂
│   │   ├── service.go                  # Summarize / LabelTopics / GenerateDigest
│   │   └── chain.go                    # Pipeline 编排 + 非阻塞容错
│   ├── module/
│   │   ├── doc.go                      # 包文档
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
│   │   ├── doc.go                      # 包文档
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
├── .superpowers/                   # SDD 进展记录与审查归档
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
    │   ├── llm/                           # LLM 单元测试
    │   └── trend/
    ├── integration/api/                # 集成测试
    └── testutil/                       # 测试辅助
        ├── db.go
        ├── env.go
        ├── router.go
        └── fake/
            ├── auth/repo.go
            ├── monitor/repo.go
            └── notify/repo.go
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
| LLM 框架 | langchaingo（Strategy + Factory + Adapter + Pipeline） |
| 配置 | Viper（.env + 环境变量） |

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
