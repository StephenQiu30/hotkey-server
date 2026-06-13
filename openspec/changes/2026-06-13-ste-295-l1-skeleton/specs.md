---
layer: Specs
issue: STE-295
title: "L1 工程骨架：Cobra + Viper + Fx + Wire"
status: accepted
---

## 规范

### S1: Cobra 子命令

- `cmd/hotkey/main.go` SHALL 注册 `api` 和 `worker` 两个子命令
- `api` 子命令 SHALL 启动 HTTP 服务
- `worker` 子命令 SHALL 启动后台任务
- 入口文件 SHALL 少于 50 行

### S2: Viper 配置

- `internal/config/config.go` SHALL 使用 Viper 加载配置
- 必填字段：`DATABASE_URL`、`JWT_SECRET`
- 可选字段：`REDIS_ADDR`、`HTTP_ADDR`（默认 `:8080`）
- SHALL 支持 `.env` 文件和环境变量

### S3: Wire 依赖注入

- `internal/app/wire.go` SHALL 定义 `InitializeAPI` 和 `InitializeWorker` provider set
- `internal/app/wire_gen.go` SHALL 由 `wire` 工具生成
- Wire SHALL 负责「谁注入谁」，Fx SHALL 负责「何时启动/停止」

### S4: Fx 生命周期

- `internal/app/api.go` SHALL 使用 `fx.New` 创建 API 应用
- `internal/app/worker.go` SHALL 使用 `fx.New` 创建 Worker 应用
- Fx Module SHALL 注册 config、database、observability

### S5: 构建配置

- `Dockerfile` SHALL 构建 `./cmd/hotkey` 入口
- `docker-compose.yml` SHALL 使用 `./hotkey-server api` / `./hotkey-server worker`
- `Makefile` SHALL 有 `build` 目标构建 `hotkey-server` 二进制

### S6: 兼容性

- 现有 smoke test 路径（`SMOKE_TEST=1`）SHALL 保留
- 不修改 HTTP handler 实现
- 不修改数据库 schema
