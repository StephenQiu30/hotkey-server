# HotKey-Server 工程化改造设计规范

- **日期:** 2026-07-07
- **状态:** 待实施
- **架构:** Go + GORM + Fx + PostgreSQL + Redis

---

## 1. 目标

将 hotkey-server 从当前手动拼装、Raw SQL 混杂的架构升级为**工程化、可测试、模块化的企业级 Go 后端体系**。

### 1.1 非目标

- 不改变现有业务功能
- 不引入微服务拆分（保持单体架构）
- 不更换 HTTP 框架（保留 Gin）
- 不改变 API 契约（Swagger 路径/参数保持不变）

---

## 2. 技术栈

| 组件 | 选型 | 版本 | 用途 |
|------|------|------|------|
| Web 框架 | Gin | 当前 | HTTP 路由与中间件 |
| ORM | GORM | 当前 | 数据库操作 |
| DI 容器 | Uber Fx | 新增 | 依赖注入与生命周期 |
| 缓存 | go-redis | 新增 | 热点数据缓存/限流 |
| 迁移工具 | pressly/goose | 新增 | 数据库版本化迁移 |
| 数据库 | PostgreSQL | 当前 | 主存储 |
| Mock | gomock | 新增 | 测试 mock 生成 |

---

## 3. 目录结构设计

```
internal/
├── cmd/hotkey/main.go              ← 极简入口，fx.App().Run()
├── config/config.go                ← Viper 配置（保留优化）
├── pkg/                            ← 跨层共享工具
│   ├── array.go                    ← 从 database/array.go 迁入
│   ├── httputil/
│   └── pagination/
├── bootstrap/bootstrap.go          ← 数据库创建+schema初始化
├── model/                          ← 纯业务结构体（无 ORM tag）
│   ├── user.go
│   ├── monitor.go
│   ├── topic.go
│   ├── event.go
│   ├── hot_event.go
│   ├── content.go
│   └── ...
├── repository/                     ← Repository 接口定义
│   ├── user.go
│   ├── monitor.go
│   └── ...
├── repository/gormimpl/            ← GORM 实现
│   ├── model.go                    ← GORM 映射模型（含 Scanner/JSONB）
│   ├── user.go
│   ├── monitor.go
│   ├── hot_event.go                ← 已有好样板，保持
│   └── ...
├── service/                        ← 业务逻辑层
├── handler/                        ← HTTP handler（薄层）
├── router/router.go                ← 统一路由注册
├── middleware/                      ← Gin 中间件
├── worker/                         ← 后台任务
├── cache/                          ← Redis 缓存层
└── fxapp/app.go                   ← Fx 组装点
```

---

## 4. Fx 组装设计

### 4.1 Fx Module 分层

```go
// fxapp/app.go
func NewApp() *fx.App {
    return fx.New(
        module.Infra,       // config, DB, Redis
        module.AuthModule,
        module.MonitorModule,
        module.ContentModule,
        module.TopicModule,
        module.EventModule,
        module.HotEventModule,
        module.DigestModule,
        module.TrendModule,
        module.NotifyModule,
        module.SchedulerModule, // workers
    )
}
```

### 4.2 模块示例

```go
// internal/module/monitor.go
var MonitorModule = fx.Module("monitor",
    fx.Provide(gormimpl.NewMonitorRepo),
    fx.Provide(gormimpl.NewContentRepo),
    fx.Provide(monitor.NewService),
    fx.Provide(monitor.NewHandler),
    fx.Provide(cache.NewMonitorCache),
)
```

### 4.3 HTTP Server 生命周期

```go
// 通过 Fx Lifecycle 管理 Gin 启停，替代手动 signal.Notify
fx.Provide(fxapp.NewHTTPServer)
```

---

## 5. GORM 改造方案

### 5.1 废除所有 Raw SQL

将当前 `internal/database/` 中约 66 处 `db.Raw()` / `db.Exec()` 统一改为 GORM 链式调用。

### 5.2 UPSERT 统一

```go
db.Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "platform"}},
    UpdateAll: true,
}).Create(&record)
```

### 5.3 动态 UPDATE 统一

```go
db.Model(&model.KeywordMonitor{}).
    Where("id = ? AND user_id = ?", id, userID).
    Updates(map[string]any{
        "name":       newName,
        "updated_at": time.Now(),
    })
```

### 5.4 JSONB Scanner

```go
type JSONB[T any] struct { Data T }
// 实现 sql.Scanner / driver.Valuer
```

### 5.5 复杂 JOIN

优先用 GORM SubQuery + Joins。`LEFT JOIN LATERAL` 等特殊查询可保留参数化 Raw SQL。

---

## 6. 数据库迁移

### 6.1 方案

采用 `pressly/goose`：

```
db/migrations/
├── 000001_create_users.up.sql
├── 000001_create_users.down.sql
├── 000002_create_keyword_monitors.up.sql
...
```

### 6.2 迁移入口

```go
// bootstrap/bootstrap.go
func Migrate(sqlDB *sql.DB) error {
    return goose.Up(sqlDB, "db/migrations")
}
```

---

## 7. Redis 缓存

### 7.1 缓存策略

- **Cache-Aside**：读时查缓存→miss 则回源 DB→写入缓存
- **TTL**：热点事件 5min，话题 3min，排行榜 2min
- **失效**：Worker 写入数据后主动删除对应缓存 key

### 7.2 缓存穿透保护

- 空值缓存（1min TTL）
- bloom filter（可选，后续引入）

---

## 8. 测试策略

### 8.1 分层测试

| 层级 | 技术 | 运行时间 |
|------|------|---------|
| 单元测试 | gomock + go test | < 5s |
| 集成测试 | testcontainers-go | < 30s |
| E2E | 手动 + Swagger | 按需 |

### 8.2 Repository 层测试

用 testcontainers 启动真实 PostgreSQL，不对 GORM 做 mock。

### 8.3 Service 层测试

Mock Repository 接口，只测试业务逻辑。

### 8.4 Makefile 命令

```makefile
test-unit:        # 纯单元测试
test-integration: # 需要 Docker
test-all:         # 全量
```

---

## 9. 分阶段实施计划

### Phase 1：基础设施搭建（1-2天）
- 添加 Fx / go-redis / goose 依赖
- 创建新目录结构骨架
- 初始化迁移目录
- 搭建 fxapp/app.go 空壳

### Phase 2：Model + Repository 改造（3-5天）
- 定义纯 model 结构体
- 定义 Repository 接口
- 逐文件迁移 gormimpl（hot_event → auth → event → monitor → 复杂 JOIN）
- 每迁移一个 repo 即通过编译和现有测试

### Phase 3：Fx 串联（1天）
- 将所有 Provide/Invoke 接入 Fx
- 废弃 run.go 手动拼装
- 验证启动流程正确

### Phase 4：Redis 缓存（1-2天）
- 实现 Cache[T] 泛型
- 为热点查询添加缓存
- Worker 写入时失效缓存

### Phase 5：测试体系（1-2天）
- 生成 mock 代码
- Service 层单元测试
- testcontainers 集成测试
- Makefile 更新

---

## 10. 文件迁移清单

| 源文件 | 目标 | 改动类型 |
|--------|------|---------|
| cmd/hotkey/main.go | 保持，调用 fx.NewApp().Run() | 微调 |
| internal/app/run.go | 删除，功能移入 fxapp/ | 删除 |
| internal/database/models.go | 拆为 model/*.go + gormimpl/model.go | 重建 |
| internal/database/*repo.go | repository/gormimpl/*.go + 改 GORM builder | 迁移+重构 |
| internal/database/*query.go | 合并到对应 repository/gormimpl/ | 迁移+重构 |
| internal/database/array.go | pkg/array.go | 迁移 |
| internal/database/database.go | 保留，供 Fx Provide | 保留 |

---

## 11. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| GORM 链式调用暗坑 | 🟡 中 | 以 HotEventRepo 已验证 pattern 为准 |
| 大面积改动引入 regression | 🔴 高 | 每次迁移后运行已存在的测试 |
| Fx 学习成本低 | 🟢 低 | 核心 3 个概念，一天上手 |
| 新旧代码并行 | 🟢 低 | Phase 内新旧不共存，一次切换 |
