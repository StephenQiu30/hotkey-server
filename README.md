# hotkey-server

X 热点监控平台后端服务：API、内容检索、主题聚合、趋势分析、告警通知与 OpenAPI 契约事实源。

## 架构

```
┌─────────────────────────────────────────────────────┐
│                   cmd/hotkey (main)                  │
│              Fx DI (go.uber.org/fx)                  │
├─────────────────────────────────────────────────────┤
│  internal/fxapp    — 应用组合（Fx App + HTTP Server）  │
│  internal/module   — 基础设施模块（Config/DB/Redis）   │
├─────────────────────────────────────────────────────┤
│  ┌─ 业务服务 ─────────────────────────────────────┐  │
│  │  auth     — 账号注册/登录 （bcrypt + JWT）      │  │
│  │  monitor  — 关键词监控 CRUD                    │  │
│  │  notify   — 通知服务（站内通知）                 │  │
│  │  hotevent — 热点事件查询                        │  │
│  └────────────────────────────────────────────────┘  │
│  ┌─ 读写层 ─────────────────────────────────────────┐ │
│  │  database  — GORM 查询服务（content/topic/trend） │ │
│  │  repository/gormimpl — GORM 实体实现              │ │
│  │    ├── model.go    (26 个 GORM 模型，1:1 映射 DB)  │ │
│  │    ├── user.go                                    │ │
│  │    ├── monitor_repo.go                            │ │
│  │    ├── notify_repo.go                             │ │
│  │    └── hot_event.go                               │ │
│  └──────────────────────────────────────────────────┘ │
│  ┌─ 领域模型 ─────────────────────────────────────────┐│
│  │  content — 内容标准化与命中评分                     ││
│  │  topic   — 主题聚类                               ││
│  │  trend   — 趋势计算                               ││
│  └──────────────────────────────────────────────────┘│
│  ┌─ 平台层 ──────────────────────────────────────────┐│
│  │  platform/http     — Gin HTTP API（唯一 HTTP 入口） ││
│  │  platform/logging  — 结构化日志                    ││
│  │  platform/runtime  — 运行时管理                    ││
│  └──────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────┤
│  db/schema.sql          — PostgreSQL DDL 事实源      │
│  db/migrations/         — goose 迁移                 │
└─────────────────────────────────────────────────────┘
```

### 包依赖原则

- **领域模型**（auth/monitor/notify/content/topic/trend/hotevent）：定义纯接口和数据，零外部依赖
- **持久化**（database/gormimpl）：实现领域接口，依赖 GORM + PostgreSQL
- **业务层**（fxapp）：通过 Fx DI 组合依赖，不直接依赖具体实现
- **平台层**（platform/*）：提供统一 HTTP 契约、中间件、日志、运行时上下文

## 数据层

- 26 个 GORM 模型，1:1 映射数据库表
- `pkg.Int64Array` — PostgreSQL `bigint[]` 序列化
- `pkg.JSONB[T]` — PostgreSQL `jsonb` 泛型扫描器

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.26 |
| HTTP 框架 | Gin |
| ORM | GORM |
| DI | go.uber.org/fx |
| 数据库 | PostgreSQL 16 |
| 缓存 | Redis |
| Schema 管理 | goose 迁移 |
| 认证 | bcrypt + JWT |

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
```

## 验证

```bash
# 完整验证（构建 + vet + 测试 + 架构边界 + schema + API smoke）
make ci

# 仅架构边界检查
bash scripts/validate-architecture-boundaries.sh

# 仅 Schema 验证
bash scripts/validate-schema.sh
```

## Agent 规范

- `CLAUDE.md` — Claude 协作规范
- `CLAUDE.local.md` — 本项目局部配置
- `WORKFLOW.md` — Symphony / Linear 调度契约
- `.claude/agents/` — 角色定义
- `.claude/skills/` — 可复用工作流
