---
layer: Proposal
issue: STE-295
title: "L1 工程骨架：Cobra + Viper + Fx + Wire"
status: accepted
---

## 概述

将 `cmd/api/main.go` 的手工装配重构为 Cobra + Viper + Wire + Fx 工程骨架，实现 Layer 1 目标。

## 变更范围

### 涉及文件

- `cmd/hotkey/main.go` — Cobra 入口
- `cmd/hotkey/api.go` — api 子命令
- `cmd/hotkey/worker.go` — worker 子命令
- `internal/config/config.go` — Viper 重构
- `internal/platform/config/module.go` — Fx config 模块
- `internal/app/wire.go` — Wire provider set
- `internal/app/wire_gen.go` — Wire 生成
- `internal/app/api.go` — Fx API 应用
- `internal/app/worker.go` — Fx Worker 应用
- `Dockerfile` — 入口二进制名
- `Makefile` — 构建目标
- `docker-compose.yml` — 命令更新

### 不在范围内

- HTTP 路由实现（Layer 2）
- 数据库/sqlc 迁移（Layer 3）
- 业务逻辑变更（Layer 4）

## 非目标

1. 不替换现有 HTTP handler 实现
2. 不引入新业务依赖
3. 不修改数据库 schema 或查询

## 验证方式

```bash
make test && make validate && go build ./...
```
