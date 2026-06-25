---
layer: Design
issue: STE-295
title: "L1 工程骨架：Cobra + Viper + Fx + Wire"
status: accepted
---

## 设计

### 目录结构

```
cmd/hotkey/
  main.go          # Cobra root command
  api.go           # api subcommand
  worker.go        # worker subcommand
internal/
  app/
    wire.go        # Wire provider sets
    wire_gen.go    # Wire generated
    api.go         # Fx API app
    worker.go      # Fx Worker app
  platform/
    config/
      module.go    # Fx config module
  config/
    config.go      # Viper config loader
```

### 数据流

```
cmd/hotkey/main.go
  └─ Cobra root
       ├─ api subcommand → internal/app/api.go → fx.New → Wire InitializeAPI
       └─ worker subcommand → internal/app/worker.go → fx.New → Wire InitializeWorker

Wire InitializeAPI:
  config.Load → database.Open → auth.NewService → monitor.NewService → server.NewRouter

Fx 生命周期:
  OnStart → http.ListenAndServe / jobs.Runner.Run
  OnStop → server.Shutdown / db.Close
```

### 关键决策

1. **Cobra 替换 os.Args 手写** — 子命令增多后可维护性更好
2. **Viper 替换 os.Getenv** — 统一校验与文档化字段表
3. **Wire 负责静态图** — 编译期发现缺失依赖
4. **Fx 负责生命周期** — 优雅启停、依赖排序

### 回滚策略

- 保留 `cmd/api/main.go` 直到验证通过
- 新旧入口并行期：`cmd/api` 和 `cmd/hotkey` 均可构建
- 验证通过后删除 `cmd/api/`
