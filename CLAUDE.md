# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概览

X 热点监控平台后端服务。自动采集多平台热点内容、聚类主题、分析趋势，通过邮件与 Obsidian 推送日报。

- **语言**：Go 1.26
- **HTTP**：Gin
- **ORM**：GORM v2
- **DI**：go.uber.org/fx
- **数据库**：PostgreSQL 16（pgvector）+ Redis
- **消息队列**：Kafka
- **LLM**：langchaingo
- **定时**：robfig/cron/v3

## 架构分层（Fx DI）

```
cmd/hotkey/main.go            入口，启动 Fx App
internal/
├── fxapp/                     Fx 应用组装（NewApp + 生命周期）
├── module/                    Fx 基础设施 Module（Config / DB / Redis）
├── config/                    Viper 配置加载（.env + 环境变量）
├── controller/                Gin HTTP 处理器（路由注册 + handler + 中间件）
├── service/                   业务逻辑层（接口定义 + 实现）
│   ├── auth.go, monitor.go, notify.go, ...
│   └── report.go, topic.go, trend.go, collect.go, llm.go, hotevent.go
├── repository/                数据访问层（GORM 实现 service 层接口）
├── content/                   跨平台内容检索查询
├── queue/                     Kafka 消息队列（Producer / Consumer / Dispatcher / Dedupe / DLQ）
├── worker/                    后台定时任务（日报发布、小时聚合）
├── module/                    基础设施 Module（DB / Redis 连接）
├── pkg/                       共享工具类型
│   ├── jsonb.go               PostgreSQL JSONB 泛型
│   └── vector.go              pgvector 384维向量类型
└── platform/                  基础设施
        ├── http/              Gin 中间件（accesslog / auth / cors / recover）
        ├── database/          GORM 初始化和日志
        └── logging/           Zap 日志
```

### 依赖注入（Fx）

`module.Infra` → 提供 Config / DB / Redis → repository 实现 as 接口注入 service → service → controller → fxapp 组装 HTTP Server + Worker + Kafka + Cron。

controller 层完全通过 `internal/controller/route.go` 的 `Config` 结构体接收依赖，不在 controller 包内直接访问全局变量。

### 数据模型

- `model/entity/` — GORM 表模型（`KeywordMonitor`, `PlatformPost`, `Report`, `Topic`, `Alert`, `Event`, `User`...）
- `model/dto/` — 请求/响应 DTO
- `model/vo/` — 统一响应格式（`ResponseBody`, `PageBody`）

## 测试结构

```
tests/
├── testutil/                  测试辅助（DB 连接 / Router 组装 / Fake 仓库）
│   ├── fake/auth/, fake/monitor/, fake/notify/
│   ├── db.go, kafka.go, router.go, env.go
├── integration/api/           集成测试（端到端 auth→register→login→API）
├── unit/                      单元测试（按领域组织）
│   ├── auth/, config/, collect/, database/, embedding/
│   ├── llm/, monitor/, notify/, obsidian/
│   ├── pkg/, topic/, trend/
│   ├── report/, worker/, queue/
│   └── platform/http/
```

集成测试需要真实 PostgreSQL（`TEST_DATABASE_URL` 或 `DATABASE_URL` 环境变量）。

## 常用命令

```bash
make test              # 全部测试（go test ./... -v -count=1）
make lint              # 静态检查（go vet ./...）
make build             # 构建二进制
make dev               # 本地开发启动（scripts/dev.sh）
make up                # Docker Compose 全栈启动
make down              # Docker Compose 停止
make schema            # 应用数据库 schema
make validate          # Schema + 架构边界校验
make validate-schema   # Schema 与实体一致性校验
make validate-arch     # 架构层边界校验（禁止跨层引用）
make smoke             # 运行时 API 冒烟测试
make ci                # 完整 CI（lint + build + test + validate + smoke）

# 单包测试
go test ./internal/service/... -v -count=1
go test ./tests/unit/monitor/... -v -count=1
```

## 约束与规范

1. **版本控制** — 使用 Fx DI，不引入全局变量。controller 层依赖通过 `Config` 结构体传入。
2. **实体循环依赖** — `model/entity/` 不引用 `model/dto/` 或其他业务包。entity 可使用 `internal/pkg`（JSONB / Vector 类型）。
3. **分层引用方向**：controller → service → repository，反向引用通过接口 + Fx DI 解耦。
4. **pgvector** — 所有 embedding 向量使用 `pkg.Vector384` 类型，数据库 schema 通过 `CREATE EXTENSION vector` + `ALTER TABLE ... ADD COLUMN ... vector(384)` 管理。
5. **消息队列** — 使用 Kafka，消息路由通过 `queue.Dispatcher` + `Queue.Register(job)` 模式，失败消息写入 DLQ 表。
6. **PR 准备** — 提交前跑 `make ci`（lint + build + test + validate + smoke），确保 CI 通过。
7. **CLAUDE.local.md** 不重复 CLAUDE.md 内容，只存本地环境特有配置。