# Engineering Upgrade — Specs

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                          Fx DI Container                       │
│  ┌─────────┐  ┌──────────┐  ┌────────┐  ┌────────────────┐   │
│  │ Config  │  │ GORM DB  │  │  Redis  │  │   Logger/OTel   │   │
│  └────┬────┘  └─────┬────┘  └────┬───┘  └────────┬───────┘   │
│       │              │            │                │           │
│  ┌────▼──────────────▼────────────▼────────────────▼───────┐  │
│  │                   Repository Layer (接口 + GORM实现)       │  │
│  │  UserRepo  MonitorRepo  TopicRepo  HotEventRepo  ...     │  │
│  └───────────────────────────┬──────────────────────────────┘  │
│                              │                                 │
│  ┌───────────────────────────▼──────────────────────────────┐  │
│  │                   Service Layer (业务逻辑)                 │  │
│  │  Auth  Monitor  Topic  Event  Digest  Trend  Notify      │  │
│  └───────────────────────────┬──────────────────────────────┘  │
│                              │                                 │
│  ┌───────────────────────────▼──────────────────────────────┐  │
│  │        Handler Layer + Router (Gin)                     │  │
│  │  Auth  Monitor  Topic  HotEvent  Digest  Trend  Notify  │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                 │
│  ┌───────────────────────────▼──────────────────────────────┐  │
│  │                Worker Layer (后台任务)                     │  │
│  │  Poll  Digest  Snapshot  Cleanup  Trending               │  │
│  └──────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
internal/
├── cmd/hotkey/main.go              ← fx.App().Run()
├── config/config.go                ← Viper 配置
├── pkg/                            ← 跨层工具
│   ├── array.go                    ← PostgreSQL bigint[]
│   └── jsonb.go                    ← JSONB Scanner/Valuer
├── bootstrap/bootstrap.go          ← 数据库创建+goose迁移
├── model/                          ← 纯业务结构体
│   ├── user.go
│   ├── monitor.go
│   ├── topic.go
│   ├── event.go
│   ├── hot_event.go
│   └── ...
├── repository/                     ← 接口定义
│   ├── user.go
│   ├── monitor.go
│   ├── ...
├── repository/gormimpl/            ← GORM 实现
│   ├── model.go                    ← GORM 映射（含 JSONB Scanner）
│   ├── user.go
│   ├── monitor.go
│   ├── hot_event.go
│   └── ...
├── service/
├── handler/
├── router/router.go
├── middleware/                      ← Gin 中间件
│   ├── auth.go
│   ├── logger.go
│   ├── recovery.go
│   └── ratelimit.go
├── worker/                         ← Fx Lifecycle 管理
├── cache/                          ← Redis Cache-Aside
│   ├── cache.go                    ← Cache[T] 泛型
│   ├── hot_event_cache.go
│   └── topic_cache.go
├── module/                         ← Fx Module 分拆
│   ├── infra.go
│   ├── auth.go
│   ├── monitor.go
│   └── ...
└── fxapp/app.go                    ← Fx 装配点
```

## Layer Rules

1. **model/**: 纯 struct，无 ORM tag，无业务方法
2. **repository/**: 接口定义，参数必须包含 `ctx context.Context`
3. **repository/gormimpl/**: 全部 GORM builder，禁止 Raw/Exec
4. **service/**: 只依赖 repository 接口，不依赖 gormimpl
5. **handler/**: 只调用 service，不直接访问 repository
6. **cache/**: 只处理缓存逻辑，不写数据库
7. **worker/**: 通过 Fx Lifecycle 管理启停
