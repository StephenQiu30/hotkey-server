# Backend Task Infrastructure Enhancement Design

> 基于 robfig/cron + Kafka 的后台任务基础设施改造设计

**状态:** Draft  
**日期:** 2026-07-08  
**关联项目:** hotkey-server  
**参考:** [algorithm-cloud RabbitMQ 死信队列模式](https://github.com/StephenQiu30/algorithm-cloud)

---

## 1. 背景与目标

### 1.1 现状

当前定时任务为单进程手写调度器——`time.Ticker` 每分钟轮询 + `ShouldRun()` 逻辑判断，仅有一个 Obsidian 日报任务。存在以下问题：

- 无 cron 表达式支持，每个新定时任务都需要手写轮询逻辑
- 无异步任务队列，耗时操作（数据采集、报告生成）直接阻塞调度循环
- 无失败重试和死信机制，任务失败仅有日志
- 无监控面板
- 每新增一个定时任务都需要修改 `fxapp/app.go` 中的 ticker 循环

### 1.2 目标

- 引入标准 cron 调度库，支持 cron 表达式配置
- 引入 Kafka 异步消息队列，实现生产-消费解耦
- 实现死信队列机制，失败消息可追溯
- 单实例部署，无需分布式协调
- 首批迁移 Obsidian 日报生成，保留后续扩展能力

### 1.3 非目标

- ❌ 不引入分布式 worker coordination（单实例）
- ❌ 不实现 CQRS / Event Sourcing
- ❌ 不实现复杂 DAG 工作流编排
- ❌ 不一次性迁移所有现有任务（采集、通知后续迭代）

---

## 2. 整体架构

```
┌─────────────────────────────────────────────┐
│              robfig/cron/v3                  │
│  daily-digest (0 8 * * *)   定时触发         │
│  后续: 采集、通知                           │
└────────┬────────────────────────────────────┘
         │ 每个 cron 触发 → 生产消息到 Kafka
         ▼
┌─────────────────────────────────────────────┐
│              Kafka Producer                  │
│  topic: hotkey.digest.run                    │
│  topic: hotkey.collect.run  (后续)           │
│  topic: hotkey.notify.run  (后续)            │
└────────┬────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────┐
│              Kafka Consumer                  │
│  ConsumerGroup: "hotkey-{domain}-worker"     │
│  offset 自动提交 (At-Least-Once)             │
│                                              │
│  ┌─────────────────────────────────────┐     │
│  │         Dispatcher                   │     │
│  │  按 msg.Type 路由到具体 Handler      │     │
│  │                                      │     │
│  │  "digest.run"  → DigestHandler        │     │
│  │  "collect.run" → CollectHandler (后续)│     │
│  │  "notify.run"  → NotifyHandler (后续) │     │
│  └─────────────────────────────────────┘     │
└─────────────────────────────────────────────┘
```

### 2.1 组件职责

| 组件 | 职责 |
|------|------|
| **cron (robfig/cron/v3)** | 只负责"到点了，生产一条消息"——不做具体业务 |
| **Kafka Producer** | 只负责"把消息写到 topic"——不决定何时触发 |
| **Kafka Consumer** | 只负责"收到消息→dispatcher→handler"——不管谁触发的 |
| **Dispatcher** | 按 `msg.Type` 路由到对应的 Handler 实现 |
| **Handler** | 执行业务逻辑（生成日报、采集数据等） |
| **Dead Letter** | Handler 多次重试失败后，消息投递到 DLQ topic |

---

## 3. 消息模型

### 3.1 统一消息结构

```go
// internal/queue/message.go
type Message struct {
    ID         string    `json:"id"`          // UUID v7，全局唯一
    Type       string    `json:"type"`        // "digest.run", "collect.run", ...
    Payload    []byte    `json:"payload"`     // 业务数据，由 Handler 反序列化
    CreatedAt  time.Time `json:"created_at"`
    RetryCount int       `json:"retry_count"` // 当前重试次数
}
```

### 3.2 Topic 命名规范

参考 algorithm-cloud 命名风格：`hotkey.{domain}.{action}`

| 领域 | Topic | DLQ Topic | 说明 |
|------|-------|-----------|------|
| digest | `hotkey.digest.run` | `hotkey.digest.run.dlq` | Obsidian 日报生成 |
| collect | `hotkey.collect.run` | `hotkey.collect.run.dlq` | 热点数据采集（后续） |
| notify | `hotkey.notify.run` | `hotkey.notify.run.dlq` | 通知推送（后续） |

### 3.3 Handler 契约

```go
// internal/queue/dispatcher.go
type Handler interface {
    Type() string
    Handle(ctx context.Context, msg Message) error
}
```

---

## 4. 死信队列机制（DLQ）

### 4.1 Kafka 端 DLQ 实现

RabbitMQ 有声明式 DLX，Kafka 没有内置 DLQ。采用**生产者模式**实现：

```
Consumer 收到消息
    │
    ├─ ✅ 成功处理 → commit offset
    │
    ├─ ❌ 可重试错误（业务异常、网络抖动）
    │   ├─ retry_count < maxRetry (默认3次)
    │   │   → 投递到原 topic (retry_count+1)
    │   │   → commit offset ✓ (避免阻塞 partition)
    │   │
    │   └─ retry_count ≥ maxRetry
    │       → 投递到 DLQ topic + commit offset ✓
    │
    ├─ ❌ 不可恢复错误（反序列化失败、bizType 不匹配）
    │   → 直接投递到 DLQ topic + commit offset ✓
    │
    └─ 💥 系统异常（panic）
        → 不 commit offset，重启后重新消费
```

### 4.2 重试策略参数

```go
type ConsumerConfig struct {
    MaxRetries    int           // 默认 3
    RetryInterval time.Duration // 默认 30s（投递到原 topic 前等待）
}
```

### 4.3 DLQ 消息记录表

在数据库中记录所有进入 DLQ 的消息，用于运维管理：

```sql
CREATE TABLE dead_letter_records (
    id              BIGSERIAL PRIMARY KEY,
    topic           VARCHAR(255) NOT NULL,
    message_id      VARCHAR(64)  NOT NULL,
    message_type    VARCHAR(64)  NOT NULL,
    payload         TEXT,
    error_message   TEXT,
    retry_count     INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
```

---

## 5. 去重机制

### 5.1 消息幂等性

利用已有 Redis，参考 algorithm-cloud `@RabbitMqDedupeLock` 模式：

```go
// internal/queue/dedupe.go
// 基于消息 ID 的去重锁
// TTL: 24 小时，同一条消息 ID 不重复消费
func (d *Dedupe) Seen(ctx context.Context, msgID string) (bool, error) {
    // SETNX msgID 1 EX 86400
    // 返回 true = 已见过, false = 首次见
}
```

去重只在 Handler 级别开启（非所有消息）。每个 Handler 通过 Option 声明是否需要去重：

```go
type Handler interface {
    Type() string
    Handle(ctx context.Context, msg Message) error
    DedupeEnabled() bool  // 是否需要消息去重
}
```

---

## 6. 代码模块结构

```
internal/
├── queue/                    ← 新增：Kafka 基础设施
│   ├── message.go           消息模型 + JSON 序列化
│   ├── producer.go          Kafka 生产者封装
│   ├── consumer.go          Kafka 消费者 + dispatcher 调度
│   ├── dispatcher.go        Handler 注册 + 按 Type 路由
│   ├── dedupe.go            Redis 消息去重
│   └── types.go             通用常量、错误定义
│
├── worker/                   ← 改造
│   ├── daily_obsidian_publish.go
│   │   └── 实现 queue.Handler (type="digest.run")
│   │
│   ├── daily_scheduler.go    ← 删除：整个被 cron 取代
│   │
│   ├── run_repository.go
│   │   └── 保留：knowledge_runs 幂等性
│   │
│   └── handler_registry.go  ← 整合 handler 注册
│
├── fxapp/app.go
│   └── ticker → cron.New() + Producer + Consumer
│
├── config/config.go
│   └── 新增: KafkaBrokers, Kafka config 组
```

---

## 7. Cron 调度配置

### 7.1 注册方式

```go
// internal/fxapp/app.go
c := cron.New(cron.WithLocation(loc))

// 日报：每天 08:00 触发
c.AddFunc("0 8 * * *", func() {
    queue.Publish(ctx, "hotkey.digest.run", Message{
        Type: "digest.run",
        Payload: json.RawMessage(`{"target_date":"2026-07-08"}`),
    })
})

// 后续任务只需再加一行
// c.AddFunc("*/30 * * * *", func() { ... })

c.Start()
```

### 7.2 配置驱动（后续优化）

cron 表达式可逐步迁移到配置文件中：

```yaml
cron:
  digest: "0 8 * * *"
  collect: "*/30 * * * *"
  notify: "*/15 * * * *"
```

---

## 8. 基础设施

### 8.1 Docker Compose

新增 Kafka + Zookeeper 服务：

```yaml
zookeeper:
  image: confluentinc/cp-zookeeper:7.8.0
  container_name: hotkey-zookeeper
  environment:
    ZOOKEEPER_CLIENT_PORT: 2181
  ports: ["2181:2181"]

kafka:
  image: confluentinc/cp-kafka:7.8.0
  container_name: hotkey-kafka
  depends_on: [zookeeper]
  ports: ["9092:9092"]
  environment:
    KAFKA_BROKER_ID: 1
    KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
    KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092
    KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
    KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    KAFKA_LOG_RETENTION_HOURS: 168
  volumes:
    - kafka-data:/var/lib/kafka/data
```

### 8.2 配置切换

```go
// internal/config/config.go
type KafkaConfig struct {
    Brokers           []string      // 默认 ["localhost:9092"]
    ConsumerGroup     string        // 默认 "hotkey-workers"
    MaxRetries        int           // 默认 3
    RetryInterval     time.Duration // 默认 30s
    TopicPrefix       string        // 默认 "hotkey"
    AutoCreateTopics  bool          // 默认 true（开发环境）
}
```

**本地开发**用本机 Kafka（`localhost:9092`），**docker-compose** 时通过环境变量切换为 `kafka:9092`。

---

## 9. 首批改造范围

### 9.1 Change List

| 操作 | 文件 | 说明 |
|------|------|------|
| 🔴 删除 | `internal/worker/daily_scheduler.go` | 整个被 cron 取代 |
| 📝 新增 | `internal/queue/` (5 文件) | Kafka 基础设施 |
| 🔧 修改 | `internal/worker/daily_obsidian_publish.go` | 实现 Handler 接口 |
| 🔧 修改 | `internal/fxapp/app.go` | ticker → cron + producer + consumer |
| 🔧 修改 | `internal/config/config.go` | 加 Kafka 配置 |
| 📝 新增 | `db/migrations/000001_create_all_tables.up.sql` | 加 `dead_letter_records` 表 |
| 📝 新增 | `.env.example` | 加 Kafka 相关环境变量 |
| 🔧 修改 | `docker-compose.yml` | 加 Kafka/Zookeeper |

### 9.2 保留不变

- `internal/worker/run_repository.go` — knowledge_runs 幂等性继续使用
- `internal/obsidian/` — 渲染/路径/写入逻辑不动
- `internal/report/` — 报告服务不动
- `internal/monitor/` — 监控服务不动

### 9.3 依赖库新增

```
github.com/robfig/cron/v3          — cron 调度
github.com/segmentio/kafka-go      — Kafka 客户端（纯 Go，API 简洁）
```

---

## 10. 后续迭代（不做在首批）

| 迭代 | 内容 |
|------|------|
| v2 | 热点数据采集迁移到 cron + Kafka |
| v3 | 通知推送迁移到 cron + Kafka |
| v4 | Kafka 监控面板（Kafka UI / AKHQ） |
| v5 | 配置驱动的 cron 表达式（YAML → cron） |

---

## 11. 风险与回退

| 风险 | 影响 | 缓解 |
|------|------|------|
| Kafka 依赖新增增加运维成本 | 中等 | 提供 docker-compose 一键部署，本地用已安装的 Kafka |
| 消息丢失 | 高 | Producer 用 `acks=all` + Consumer 用自动 commit（At-Least-Once） |
| 死信消息积压无人处理 | 低 | 记录到 dead_letter_records 表，后续加管理通知 |
| 本机 Kafka 版本不一致 | 低 | docker-compose 固定镜像版本 7.8.0 |

回退方案：保留旧 ticker 代码（`daily_scheduler.go` 不立即删除，先注释）。新架构运行稳定后再清理。
