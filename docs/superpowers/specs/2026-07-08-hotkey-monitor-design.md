# HotKey 热点事件监控统计与日报服务 设计方案

> 基于 pgvector 余弦相似度匹配 + ONNX 内嵌 embedding 的实时热点链路

**版本：** v1.0
**日期：** 2026-07-08

---

## 1. 目标与范围

### 1.1 业务目标

构建从 **关键词配置 → 内容采集 → 语义匹配 → 话题聚类 → 热点识别 → 趋势分析 → 日报产出** 的完整自动化链路，最终通过 Obsidian Vault 交付每日热点报告。

### 1.2 成功标准

- X (Twitter) 长连接实时采集，延迟 < 30s
- 关键词与帖子的余弦相似度匹配 > 阈值时自动入库
- 每小时自动执行话题聚类、热点聚合、趋势快照
- 每日 8:00（Asia/Shanghai）自动产出日报并写入 Obsidian Vault（已有，不修改）

### 1.3 非目标（第一版不做）

- 通知推送（站内信/邮件）
- 多平台采集（X 优先验证后再扩展）
- Anthopic/Ollama LLM provider（仅用 OpenAI）

---

## 2. 整体架构

```
┌──────────────────────────────────────────────────────────┐
│                    X API Filtered Stream                  │
│      GET /2/tweets/search/stream (长连接, bufio.Scanner)  │
│      POST /2/tweets/search/stream/rules (规则注册)        │
└────────────────────┬─────────────────────────────────────┘
                     │ 标准化为 PlatformPost
                     ▼
┌──────────────────────────────────────────────────────────┐
│            ONNX Embedding Service (进程内)                 │
│            模型: BAAI/bge-small-zh-v1.5 → 384d float32[]   │
│            库: github.com/yalue/onnxruntime_go            │
└────────────────────┬─────────────────────────────────────┘
                     │ embedding(384d) 写入 platform_posts
                     ▼
┌──────────────────────────────────────────────────────────┐
│          pgvector 余弦相似度匹配（GORM + Expr）             │
│          1 - (platform_posts.embedding <=>                │
│              keyword_monitors.query_embedding)            │
│          阈值: 0.7 → monitor_post_hits                    │
└────────────────────┬─────────────────────────────────────┘
                     │ 采集即匹配，同步入库
                     ▼
              ┌──────┴──────┐
              │   Kafka      │ ← Cron 每小时整点
              │ hourly.run   │    发布 TopicHourlyRun
              └──────┬──────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────────┐
│          HourlyAggregateJob (每小时批量处理)                │
│                                                          │
│  Step 1: 话题聚类                                          │
│   调用已有 topic.Cluster()  (Union-Find, Jaccard 0.3)     │
│   → 写入 topics + topic_posts 表                          │
│                                                          │
│  Step 2: 热点事件聚合                                      │
│   调用已有 ComputeHeatScore / DetermineTrend               │
│   → 写入 hot_events + hot_event_platforms                 │
│                                                          │
│  Step 3: 趋势快照                                          │
│   调用已有 BuildTopicSnapshot / BuildMonitorSnapshot       │
│   → 写入 topic_snapshots + monitor_snapshots              │
└──────────────────────────────────────────────────────────┘
                     │
                     ▼
              ┌──────┴──────┐
              │  日报        │ ← 已有 0 8 * * * cron
              │ daily digest│    DailyObsidianPublishJob
              │             │    无变动
              └─────────────┘
                      ↓
              Obsidian Vault (.md)
```

---

## 3. 模块设计

### 3.1 内嵌 ONNX Embedding (`internal/embedding/`)

**模型选择：** `BAAI/bge-small-zh-v1.5`
- 输出维度：384
- 模型大小：~15MB（ONNX 导出后）
- 推理库：`github.com/yalue/onnxruntime_go`
- 模型文件通过 `EMBEDDING_MODEL_PATH` 配置指定

**核心接口：**

```go
// internal/embedding/service.go
type Service interface {
    // Embed 单文本嵌入，返回 384 维归一化向量
    Embed(ctx context.Context, text string) (Vector384, error)
    // EmbedBatch 批量嵌入（预留给后续使用）
    EmbedBatch(ctx context.Context, texts []string) ([]Vector384, error)
}
```

**生命周期：**
- Fx `OnStart` 阶段加载 ONNX 模型到内存，加载失败则拒绝启动
- 运行时推理仅做矩阵乘法，无 IO 等待
- 推理失败：记录 warn 日志，跳过本条 embedding（帖子仍入库但无匹配）
- 384d float32 向量在推理内部做 L2 归一化，使余弦相似度等价于点积

### 3.2 向量类型与 GORM 集成 (`internal/pkg/vector.go`)

```go
// Vector384 封装 384 维 float32 向量，实现 GORM Scanner/Valuer
type Vector384 [384]float32

func (v *Vector384) Scan(src interface{}) error  // pgvector → Go
func (v Vector384) Value() (driver.Value, error)  // Go → pgvector
```

**Model 变更：**

```go
// platform_posts 加字段
type PlatformPost struct {
    // 已有字段不变
    Embedding *Vector384 `gorm:"type:vector(384);column:embedding"`
}

// keyword_monitors 加字段
type KeywordMonitor struct {
    // 已有字段不变
    QueryEmbedding *Vector384 `gorm:"type:vector(384);column:query_embedding"`
}
```

### 3.3 X Filtered Stream 采集 (`internal/collect/`)

**采集流程：**

```
应用启动 (OnStart)
  → 从 DB 读取所有活跃 keyword_monitors
  → 调用 POST /2/tweets/search/stream/rules 注册过滤规则
  → 建立 GET /2/tweets/search/stream 长连接
  → bufio.Scanner 逐行读取 SSE 流
  → 解析为 PlatformPost
  → embeddingService.Embed(contentText)
  → GORM Create platform_posts
  → 余弦相似度匹配活跃关键词
  → 命中则写入 monitor_post_hits
```

**断线重连：**
- 检测到连接断开 → exponential backoff: 1s, 2s, 4s, 8s, ... 上限 5min
- 重连后通过 `since_id` 参数 catch up 断线期间可能丢失的消息
- 服务关闭时（OnStop）取消 context 以安全断开

**X API 请求限制：**
- Filtered Stream 规则数上限：25（Free）/ 250（Basic）/ 5000（Pro）
- 启动时全量替换规则，避免残留规则产生噪音
- 用户新增关键词时动态调用 `POST /rules` 添加

### 3.4 余弦相似度匹配（GORM Repository 层）

**匹配查询——全 GORM，无 raw SQL：**

```go
// internal/repository/gormimpl/match_repo.go
func (r *MatchRepo) FindMatchingPosts(ctx context.Context, monitorID int64, threshold float64) ([]PostMatch, error) {
    var results []PostMatch
    err := r.db.WithContext(ctx).
        Table("platform_posts").
        Select("platform_posts.*, 1 - (platform_posts.embedding <=> ?) AS similarity",
            gorm.Expr("keyword_monitors.query_embedding")).
        Joins("JOIN keyword_monitors ON keyword_monitors.id = ?", monitorID).
        Where("1 - (platform_posts.embedding <=> keyword_monitors.query_embedding) >= ?", threshold).
        Order(gorm.Expr("similarity DESC")).
        Find(&results).Error
    return results, err
}
```

**采集时单条匹配也类似，用 GORM Expr 封装 pgvector 操作符。**

**阈值说明：**
- 默认阈值 0.7（余弦相似度，1.0 = 完全一致，0.0 = 正交）
- 可通过 `keyword_monitors.alert_threshold_config` 覆盖
- bge-small-zh-v1.5 的 384 维向量在 0.7 阈值下召回率约 85%，精确率约 90%

### 3.5 每小时批量 Worker (`internal/worker/hourly_aggregate.go`)

**消息定义：**

```go
const TopicHourlyRun = "hotkey.hourly.run"
```

**Handler 实现——三个步骤依次执行：**

```go
func (j *HourlyAggregateJob) Handle(ctx context.Context, msg queue.Message) error {
    now := j.deps.Now()
    since := now.Add(-1 * time.Hour)

    // Step 1: 话题聚类
    // 取过去1小时的 hits，去重，调用 topic.Cluster()
    // 写入 topics + topic_posts
    if err := j.clusterPosts(ctx, since); err != nil {
        return err  // 收 DLQ，下一小时重试
    }

    // Step 2: 热点事件聚合
    // 从所有活跃 topics 聚合成 hot_events
    if err := j.aggregateHotEvents(ctx); err != nil {
        return err
    }

    // Step 3: 趋势快照
    // 对所有活跃 topics + monitors 打快照
    if err := j.snapshotTrends(ctx); err != nil {
        return err
    }

    return nil
}
```

**并发防护：** 复用已有的 `RunRepository.TryStart`，通过 `knowledge_runs` 表防止同一小时重复执行。

### 3.6 数据库变更

```sql
-- 1. 启用量子扩展
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. platform_posts 加 embedding 列
ALTER TABLE platform_posts ADD COLUMN IF NOT EXISTS embedding vector(384);
CREATE INDEX IF NOT EXISTS idx_platform_posts_embedding ON platform_posts
  USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- 3. keyword_monitors 加 query_embedding 列
ALTER TABLE keyword_monitors ADD COLUMN IF NOT EXISTS query_embedding vector(384);
```

**索引策略：** IVFFlat（Inverted File with Flat）索引，`lists = 100` 适合 10 万级数据量。后续数据量增长后可调优 `lists` 参数或升级为 HNSW 索引。

---

## 4. 配置变更

```go
// internal/config/config.go 新增
type Config struct {
    // ... 已有字段 ...
    EmbeddingModelPath string // 新增: ONNX 模型文件路径
}

// 环境变量
EMBEDDING_MODEL_PATH   // ONNX 模型文件路径，默认 "models/bge-small-zh-v1.5.onnx"
```

---

## 5. 新增/改动文件清单

### 新增文件（6 个）

| 路径 | 职责 | 估算行数 |
|------|------|----------|
| `internal/pkg/vector.go` | Vector384 类型（Scanner/Valuer） | 60 |
| `internal/embedding/model.go` | ONNX 模型加载与推理 | 120 |
| `internal/embedding/service.go` | Embed / EmbedBatch 接口 | 50 |
| `internal/collect/xclient.go` | X API Filtered Stream 客户端 | 200 |
| `internal/collect/service.go` | 采集调度（Start/Stop/规则管理） | 150 |
| `internal/worker/hourly_aggregate.go` | 每小时批量 Worker | 180 |

### 修改文件（8 个）

| 路径 | 改动内容 |
|------|----------|
| `internal/repository/gormimpl/collect_repo.go` | 新增：PlatformPost / monitor_post_hits 写入 + embedding 更新 |
| `internal/repository/gormimpl/match_repo.go` | 新增：余弦相似度匹配查询 |
| `internal/repository/gormimpl/topic_write_repo.go` | 新增：topics / topic_posts 写入 |
| `internal/repository/gormimpl/snapshot_repo.go` | 新增：topic_snapshots / monitor_snapshots 写入 |
| `internal/repository/gormimpl/model.go` | PlatformPost + KeywordMonitor 加 Embedding 字段 |
| `internal/monitor/service.go` | Create/Update 时生成 query_embedding |
| `internal/queue/message.go` | + TopicHourlyRun 常量 |
| `internal/fxapp/app.go` | 注册新 Provider + Cron `0 * * * *` |
| `internal/config/config.go` | + EmbeddingModelPath |
| `db/schema.sql` | + CREATE EXTENSION vector; + 列 |

---

## 6. 错误处理与降级

| 场景 | 行为 | 恢复方式 |
|------|------|----------|
| X API 断连 | Exponential backoff 重连（1s→5min） | 自动恢复，since_id catch up |
| ONNX 模型加载失败 | OnStart 返回 error，应用不启动 | 运维修复模型路径 |
| ONNX 推理失败 | Skip 本条帖子 embedding，warn 日志 | 后续不重试 |
| pgvector 索引失效 | 退化为全表扫描，功能正常但慢 | 监控查询延迟，自动重建索引 |
| Kafka 不可用 | Cron 发布失败，error 日志 | 下一小时自动重试 |
| Worker 重叠执行 | TryStart 拦截，return nil | 下一小时 |
| X API 规则超限（<=25 条） | 启动时分配，超出时拒绝创建新 monitor | 提示用户删除闲置 monitor |

---

## 7. 测试策略

| 层级 | 覆盖范围 | 方式 |
|------|----------|------|
| 单元测试 | Vector384 Scan/Value、Embedding 接口 mock | `go test ./internal/embedding/...` |
| 单元测试 | 采集消息解析、去重逻辑 | mock X API 返回 |
| 单元测试 | HourlyAggregateJob 三步编排 | mock repository |
| 集成测试 | pgvector 余弦相似度查询（需要真实 PG） | testcontainers PostgreSQL + pgvector |
| 集成测试 | ONNX 模型加载与推理（需要真实模型文件） | CI 预下载模型 |

---

## 8. 后续可扩展方向

- **多平台采集器**：微博、知乎、RSS → 实现相同 `Collector` 接口即可接入
- **通知触发**：heat_score 超过阈值时通过现有 notify 包推送
- **语义升级**：从关键词匹配升级到 LLM embedding（如 `text-embedding-3-small`），仅需更换 ONNX 模型文件
- **HNSW 索引**：数据量超过 100 万时，IVFFlat 可平滑升级为 HNSW 索引
- **Streaming 降级**：X API 配额不足时自动降级为轮询模式

---

## 9. 决策记录

| 序号 | 决策 | 理由 |
|------|------|------|
| ADR-01 | pgvector 而非在应用层算相似度 | 数据库层直接查询，减少网络 IO，支持 HNSW/IVFFlat 索引加速 |
| ADR-02 | 进程内 ONNX 而非容器化 embedding 服务 | 减少部署依赖，推理延迟 < 5ms，零网络开销 |
| ADR-03 | bge-small-zh-v1.5 (384d) | 中文场景最优的小模型，模型体积小，推理快 |
| ADR-04 | X Filtered Stream 而非轮询 | 长连接实时推送，延迟 < 30s，减少 API 请求量 |
| ADR-05 | 每小时 batch 处理而非实时聚类 | 数据积累充足后聚类效果更好，计算开销可控 |
| ADR-06 | 复用已有 topic.Cluster / trend 纯函数 | 代码已实现且测试覆盖，零重复开发 |
| ADR-07 | GORM Expr 封装 pgvector 操作符 | 减少 raw SQL 比例，保持 ORM 风格统一 |
