# HotKey 系统端到端可运行设计

## 概述

将 HotKey 从"契约原型"推进到"端到端可运行"状态。后端接真实 PostgreSQL/Redis，Web 前端接真实 API，提供 Docker 部署能力。

## 当前状态

- 后端 17 个领域服务全部使用内存存储，重启丢失
- 前端（Web + 小程序）全部硬编码 mock 数据，未调用任何后端 API
- PostgreSQL Schema（1082 行、50+ 表）存在但未连接
- OpenAPI 规范完整（49 个端点），但 BearerAuth 认证未执行
- 44 个测试文件（~4000 行），全部基于内存实现

## 目标状态

1. 后端连接真实 PostgreSQL 和 Redis，核心数据持久化
2. Web 前端通过 `@umijs/openapi` 生成的客户端调用真实 API
3. 用户可以登录、查看热点、管理关键词、查看日报
4. `docker compose up` 一键拉起完整环境

## 项目结构

采用标准 Go 分层结构：

```
hotkey-server/
├── cmd/server/main.go
├── internal/
│   ├── config/                  # 环境配置
│   ├── middleware/              # 认证、CORS、日志
│   ├── model/                   # 领域实体
│   ├── handler/                 # HTTP 处理层
│   ├── service/                 # 业务逻辑层（按领域分子目录）
│   ├── repo/                    # 仓储接口定义
│   ├── store/
│   │   ├── postgres/            # PG 实现
│   │   └── memory/              # 内存实现（测试/开发）
│   └── infrastructure/
│       ├── postgres/pool.go
│       └── redis/client.go
├── db/
│   ├── schema.sql
│   └── migrations/
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

依赖方向：handler → service → repo(interface) ← store/postgres | store/memory

## 核心链路

### 后端

1. `main.go` 初始化 PG 连接池（pgx/v5）和 Redis 客户端（go-redis/v9）
2. 运行数据库迁移（golang-migrate）
3. 4 个核心服务实现 PG repository：keyword、source、content、hotspot
4. 其余 13 个服务保持内存实现
5. 认证中间件验证 JWT，CORS 中间件允许前端跨域
6. 请求日志使用 slog，优雅关闭监听 SIGINT/SIGTERM

### 前端

1. 使用 `@umijs/openapi` 从后端 OpenAPI 规范生成 TypeScript 客户端
2. axios 封装请求层，自动注入 token，401 拦截
3. zustand 管理登录态，swr 管理数据请求
4. 拆分为多页面路由：登录、热点榜单、热点详情、关键词管理、日报、设置
5. 使用 `/frontend-design` 技能设计灵动美观的 UI

### Docker

1. 后端多阶段构建（golang:1.25-alpine → alpine:3.20）
2. 前端多阶段构建（node:22-alpine）
3. docker-compose.yml 包含 pgvector/pgvector:pg16、redis:7-alpine、server、web
4. Makefile 提供 up/down/build/logs 快捷命令

## 不在范围

- P1-P3 非核心服务的 PG 对接（tenant、rbac、billing、eventgraph 等）
- 小程序接真实 API
- n8n 工作流实际运行
- AI/LLM 集成（DashScope 调用）
- pgvector 向量相似度召回
- 实际数据采集（RSS/web scraping）
- Worker 进程拆分

## 验收标准

- `go run ./cmd/server` 启动成功，`/healthz` 返回 200
- PG 和 Redis 连接正常，重启后数据不丢失
- 无 token 请求受保护接口返回 401
- Web 登录 → 热点列表 → 热点详情 → 关键词管理链路通
- `docker compose up` 全部服务健康
- `go test ./...` 全绿，`tsc --noEmit` 无报错
