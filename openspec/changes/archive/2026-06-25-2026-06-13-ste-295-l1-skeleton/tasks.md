---
layer: Tasks
issue: STE-295
title: "L1 工程骨架：Cobra + Viper + Fx + Wire"
status: accepted
---

## 任务

### T1: 添加依赖

```bash
go get github.com/spf13/cobra
go get github.com/spf13/viper
go get github.com/google/wire
go get go.uber.org/fx
```

### T2: 重构 internal/config/config.go 为 Viper

- 使用 Viper 加载 `.env` 和环境变量
- 保留 `Load()` 函数签名
- 添加 `HTTP_ADDR` 默认值 `:8080`

### T3: 创建 internal/platform/config/module.go

- Fx Module 提供 `config.Config`
- 注册到 Fx 容器

### T4: 创建 internal/app/wire.go

- 定义 `APIProviderSet` 和 `WorkerProviderSet`
- 包含 config、database、auth、monitor、notify、server 等 provider

### T5: 创建 internal/app/api.go

- 使用 `fx.New` 创建 API 应用
- 注册 Wire provider set
- 注册 Fx lifecycle hooks

### T6: 创建 internal/app/worker.go

- 使用 `fx.New` 创建 Worker 应用
- 注册 Wire provider set
- 注册 Fx lifecycle hooks

### T7: 创建 cmd/hotkey/main.go

- Cobra root command
- 注册 api 和 worker 子命令

### T8: 创建 cmd/hotkey/api.go

- api 子命令调用 `internal/app/api.go`

### T9: 创建 cmd/hotkey/worker.go

- worker 子命令调用 `internal/app/worker.go`

### T10: 更新构建配置

- `Dockerfile` 构建 `./cmd/hotkey`
- `docker-compose.yml` 使用 `./hotkey-server api` / `./hotkey-server worker`
- `Makefile` 更新 build 目标

### T11: 验证

```bash
make test
make validate
go build ./...
```

### T12: 提交

- `test:` red test or documented exception
- `impl:` Cobra + Viper + Wire + Fx skeleton
- `chore:` dependency and build config updates
