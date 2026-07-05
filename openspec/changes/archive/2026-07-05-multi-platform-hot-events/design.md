## Context

当前 hotkey-server 仅支持 X (Twitter) 单一数据源的热点采集，通过 `poll_monitor` job 定时轮询 X Search API v2，结果经 Jaccard 聚类形成 Topic。系统存在以下瓶颈：

1. **数据源单一**：X API 有额度限制（月度积分），且中文内容覆盖不足
2. **事件模型薄弱**：Topic 是中间产物，无独立的事件生命周期、跨平台合并能力
3. **项目结构混淆**：`PlatformConnector` 接口藏在 `jobs/` 包中、`worker_jobs.go` 是单点耦合、所有 job 挤在 `jobs/` 目录
4. **无数据过期机制**：采集数据持续累积，无清理策略

本设计针对上述问题提出面向多平台热点事件的完整架构方案。

## Goals / Non-Goals

**Goals:**
1. 多数据源采集架构：支持微博热榜、知乎热榜、百度热点，X 保持主力
2. HotEvent 独立实体：从 Topic 和榜单数据中聚合为独立的热点事件
3. 跨平台事件匹配：余弦相似度 + 关键词交集的匹配算法
4. 项目工程结构重构：connector 抽象层、job 按职责分包、wiring 注册器模式
5. 数据过期清理：按策略定期清理旧数据
6. HotEvent REST API：榜单查询、热点事件列表/详情

**Non-Goals:**
1. 不做前端界面（仅 REST API）
2. 不做实时 WebSocket 推送
3. 不改动现有 X poll_monitor 核心逻辑
4. 不改动现有 Jaccard 聚类逻辑（`internal/topic/`）
5. 不改动现有通知/Alert 模块
6. 不改动现有 LLM/摘要模块
7. 不引入第三方 SDK 依赖（HTTP 直调）

## Architecture Design

### 分层架构

```
┌──────────────────────────────────────────────────────────────────────┐
│                          HTTP Layer (Gin)                            │
│  /api/v1/trending          /api/v1/hot-events                        │
│  /api/v1/hot-events/:id    /api/v1/hot-events/:id/posts              │
└──────────────────────┬───────────────────────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────────────────────┐
│                        Wiring Layer (internal/app/)                  │
│  routes.go — 路由注册                                                   │
│  worker.go — Worker goroutine 管理                                     │
│  worker_jobs.go — 注册器模式: 每个 job 包提供 Register() 方法             │
└──────────────────────┬───────────────────────────────────────────────┘
                       │
┌──────────────────────┬─────────────────┬────────────────────────────┐
│  collector/          │  aggregator/     │  cleanup/                  │
│  (榜单采集 Job)       │  (事件聚合 Job)   │  (数据清理 Job)             │
│  TrendingCollector   │  EventMatcher    │  CleanupPolicy             │
│  → platform_posts    │  → hot_events    │  → DELETE old records      │
└─────────┬────────────┴────────┬─────────┴───────────────┬────────────┘
          │                     │                         │
          ▼                     ▼                         ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        Jobs Layer (internal/jobs/)                    │
│  runner.go — 调度内核    poll_monitor.go — X 采集（含接口定义迁移）       │
│  aggregate_topics.go — Jaccard 聚类                                    │
└──────────────────────┬───────────────────────────────────────────────┘
                       │
┌──────────────────────┬───────────────────────────────────────────────┐
│                    Connector Layer (internal/connector/)              │
│  search.go — Searcher 接口                                            │
│  trending.go — TrendingCollector 接口                                  │
│  types.go — 共用类型                                                   │
└───┬──────────┬──────────┬──────────┬─────────────────────────────────┘
    │          │          │          │
    ▼          ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐ ┌──────────┐
│ X      │ │ 微博    │ │ 知乎    │ │ 百度     │
│ client │ │ client │ │ client │ │ client   │
│ (现有)  │ │ (新增)  │ │ (新增)  │ │ (新增)   │
└────────┘ └────────┘ └────────┘ └──────────┘
    │          │          │          │
    ▼          ▼          ▼          ▼
┌───────────────────────────────────────────────────────┐
│                Domain Layer                            │
│  hotevent/ — HotEvent 实体 + service + repository      │
│  monitor/ — Monitor 关键词配置（现有）                    │
│  topic/ — Topic 聚类（现有）                              │
└───────────────────────────────────────────────────────┘
                       │
                       ▼
┌───────────────────────────────────────────────────────┐
│               Database Layer                            │
│  platform_posts（扩展现有）                               │
│  hot_events（新增）                                       │
│  hot_event_platforms（新增）                               │
└───────────────────────────────────────────────────────┘
```

### 核心数据流

```
X 采集流：
  poll_monitor job → X API Search → XConnectorAdapter → UpsertPost → (Jaccard聚类) → Topic

榜单采集流：
  collector job → TrendingCollector(微博) → TrendingItem → UpsertPost
  collector job → TrendingCollector(知乎) → TrendingItem → UpsertPost
  collector job → TrendingCollector(百度) → TrendingItem → UpsertPost

事件聚合流：
  aggregator job → Topic + TrendingItem → EventMatcher(余弦+关键词) → HotEvent

数据流出：
  HotEvent API → JSON response
  Daily Digest → Obsidian 日报（扩展现有管道）
  Cleanup Job → 按策略删除过期数据
```

### 新包划分

> 注意：`internal/` 根目录下的子包均在 `hotkey-server/internal/` 中新建，不涉及 `openspec/specs/` 下的目录。

```
internal/
  connector/                 ← 新增：接口定义层
    search.go                ← Searcher: SearchPosts(ctx, query, cursor)
    trending.go              ← TrendingCollector: FetchTrending(ctx)
    types.go                 ← SearchResult, TrendingItem, PostResult

  platform/
    weibo/client.go          ← 新增：微博热榜采集
    zhihu/client.go          ← 新增：知乎热榜采集
    baidu/client.go          ← 新增：百度热点采集

  collector/                 ← 新增：榜单采集 Job
    job.go                   ← TrendingCollectorJob
    adapters.go              ← 各平台 → TrendingCollector 适配

  hotevent/                  ← 新增：热点事件领域
    model.go                 ← HotEvent 实体 + 状态枚举
    repository.go            ← 接口定义
    service.go               ← 热度分 + 聚合 + 生命周期

  aggregator/                ← 新增：跨平台聚合 Job
    job.go                   ← HotEventAggregatorJob
    matcher.go               ← 余弦相似度 + 关键词交集

  cleanup/                   ← 新增：数据清理 Job
    job.go                   ← CleanupJob + CleanupPolicy

  http/                      ← 扩展：新增 handler
    handler/trending.go      ← HotEvent/榜单 API handler

  app/                       ← 改造：注册器模式
    worker_jobs.go           ← 重构为注册器模式
    routes.go                ← 新增路由注册
```

## Decisions

### D1: Searcher 与 TrendingCollector 分离为两个接口
- **方案**：不合并为统一的 DataSource 接口，搜索和榜单是两种不同模式
- **理由**：X 的 `SearchPosts(query, cursor)` 需要查询参数和分页，榜单 `FetchTrending()` 不需要
- **替代方案**：统一接口 + `SupportsSearch() bool` —— 被否决，接口污染

### D2: HotEvent 独立实体，不嵌入 Topic
- **方案**：`Post → Topic → HotEvent` 三层模型，HotEvent 存储 topic_ids 和 post_ids 引用
- **理由**：Topic 只来自 X 聚类，HotEvent 跨平台聚合，生命周期不同
- **理由**：Topic 和 HotEvent 可能 1:1、N:1、1:N 关系，不能硬耦合

### D3: 匹配算法 — 余弦相似度 + 关键词交集
- **方案**：`score = w1 * cosine_sim(titles) + w2 * keyword_overlap`
- **理由**：线性加权组合，简单可调；w1=0.6, w2=0.4 默认，阈值 0.5
- **替代方案**：纯 LLM 匹配 —— 被否决，成本高且不适合大批量

### D4: 平台采集使用公开发接口/HTML 解析
- **方案**：HTTP 直调，无 Go SDK 依赖
- **理由**：目标平台无官方 Go SDK，HTTP 直调更可控

### D5: Worker 注册器模式
- **方案**：每个 job 包暴露 `Register(r *jobs.Runner, db *gorm.DB)`
- **理由**：消除 `worker_jobs.go` 单点耦合，新增 job 只需新建包 + Register 调用
- **替代方案**：依赖注入框架 —— 被否决，过度设计

### D6: connector 接口迁移
- **方案**：`PlatformConnector` 接口从 `internal/jobs/poll_monitor.go` 迁移到 `internal/connector/search.go`
- **影响**：`jobs/poll_monitor.go` 删除接口定义并 import connector 包；`jobs/adapters.go` 引用新路径
- **理由**：新平台的 client 不需要 import `jobs/` 包才能引用接口

## Data Model

### HotEvent
```go
type HotEvent struct {
    ID          int64
    Name        string      // 事件标题
    HeatScore   float64     // 综合热度
    Platform    string      // 主流平台 / multi
    Trend       string      // rising / stable / declining
    FirstSeenAt time.Time
    LastSeenAt  time.Time
    TopicIDs    []int64     // JSON array
    PostIDs     []int64     // JSON array
    Summary     string
    Category    string
    Status      string      // active / archived
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 数据库新增表
- `hot_events` — 热点事件主表
- `hot_event_platforms` — 热点事件在各平台的映射（排名、标题、热度）

### 数据库扩展
- `platform_posts` 表无需变更，已有 `platform` 字段和 `(platform, platform_post_id)` 唯一约束

## Platform Client 设计

### 微博 (internal/platform/weibo/client.go)
```
接口: GET https://weibo.com/ajax/side/hotSearch
频率: 每 5 分钟一次
反爬: User-Agent + 请求间隔
提取: realtime[].word, rank, hot_num, url
```

### 知乎 (internal/platform/zhihu/client.go)
```
接口: GET https://www.zhihu.com/api/v3/feed/topstory/hot-lists/total?limit=50
频率: 每 5 分钟一次
反爬: User-Agent（不需要鉴权）
提取: data[].target.title, detail_text, metrics_area, id
```

### 百度 (internal/platform/baidu/client.go)
```
接口: GET https://top.baidu.com/board?tab=realtime
解析: HTML 中 script#sanData JSON 提取
频率: 每 10 分钟一次（百度风控较严）
反爬: User-Agent + 请求间隔 + 失败降级
提取: title, rank, heatScore, url, desc
```

## Job 调度设计

| Job 名 | 包 | 间隔 | 职责 |
|--------|------|------|------|
| `poll_monitor` | `jobs/` | 1min | X 搜索（现有，不改） |
| `collect_trending` | `collector/` | 5min | 轮询微博/知乎/百度，写入 platform_posts |
| `aggregate_topics` | `jobs/` | 5min | X 帖子聚类（现有，不改） |
| `aggregate_events` | `aggregator/` | 5min | X Topic + 榜单 → HotEvent 匹配合并 |
| `cleanup_data` | `cleanup/` | 1h | 删除过期数据 |
| `build_snapshots` | `jobs/` | 10min | 趋势快照（现有，不改） |
| `publish_daily_topics` | `jobs/` | 1min | Obsidian 日报（扩展 HotEvent） |

## 热度分计算

```
HeatScore = w_platform * Σ(post_heat * decay_factor)
```

- `w_platform`：平台权重（X=1.0, weibo=1.0, zhihu=0.8, baidu=0.7）
- `post_heat`：平台原始热度值归一化到 [0-100]
- `decay_factor`：基于 last_seen 的时间衰减（参照现有 scoring.Service）

## API 设计

### GET /api/v1/trending
返回各平台当前榜单汇总
```
?platform=weibo|zhihu|baidu  (可筛选)
&limit=20                    (默认20)
```

### GET /api/v1/hot-events
返回跨平台热点事件列表
```
?status=active               (默认 active)
&platform=multi|x|weibo      (按来源筛选)
&sort=heat_score|last_seen   (排序)
&limit=20
```

### GET /api/v1/hot-events/:id
返回单个热点事件详情（含关联帖子列表）

### GET /api/v1/hot-events/:id/posts
返回事件关联的各平台帖子

## 项目重构步骤

1. 新建 `internal/connector/` 包，从 `jobs/poll_monitor.go` 迁移接口定义
2. 更新 `jobs/poll_monitor.go` 和 `jobs/adapters.go` 引用新接口路径
3. 新建 `internal/hotevent/` 领域包（model + repository + service）
4. 新建数据库迁移（hot_events + hot_event_platforms 表）
5. 新建三个平台 client（weibo/zhihu/baidu）
6. 新建 `internal/collector/`（榜单采集 Job）
7. 新建 `internal/aggregator/`（事件聚合 Job + 匹配算法）
8. 新建 `internal/cleanup/`（数据清理 Job）
9. 重构 `app/worker_jobs.go` 为注册器模式
10. 扩展 HTTP handler + routes
11. 扩展每日日报覆盖 HotEvent

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| 微博接口可能变更或加反爬 | 配置化 URL + 采集失败时跳过该平台，不影响其他平台 |
| 百度无公开 JSON 接口，HTML 解析脆弱 | HTML 解析配合正则/JSON 提取双重保障，监控采集成功率 |
| 跨平台匹配准确率不够 | 阈值可通过配置调整；保留人工覆写机制 |
| 榜单数据量大导致 PostgreSQL 膨胀 | Cleanup Job 定期清理，配置保留期 |
| 国内平台从本地 Mac 采集可能被限流 | 间隔 5-10 分钟 + User-Agent + 失败退避 |
| X 作为主力数据源，月度额度用完 | X API 402 时自动降级为纯榜单模式 |

## Open Questions

1. 国内平台榜单数据是否需要存储完整帖子内容，还是仅存榜单标题 + URL？
2. 热度分归一化算法：各平台热度值量级不同（微博百万、知乎万级别），是否需要自适应归一化范围？
3. 余弦相似度分词器：中文分词是否需要引入 jieba 级别工具，还是基于 unigram 的 TF 向量即可？

## 关键文件变动清单

```
新增:
  internal/connector/search.go          — Searcher 接口
  internal/connector/trending.go        — TrendingCollector 接口
  internal/connector/types.go           — 共用类型
  internal/platform/weibo/client.go     — 微博热榜 client
  internal/platform/zhihu/client.go     — 知乎热榜 client
  internal/platform/baidu/client.go     — 百度热点 client
  internal/hotevent/model.go            — HotEvent 实体
  internal/hotevent/repository.go       — HotEvent repository 接口
  internal/hotevent/service.go          — HotEvent service
  internal/database/repositories/hot_event.go  — HotEvent repo 实现
  internal/collector/job.go             — TrendingCollectorJob
  internal/collector/adapters.go        — 平台→Collector 适配
  internal/aggregator/job.go            — HotEventAggregatorJob
  internal/aggregator/matcher.go        — EventMatcher
  internal/cleanup/job.go               — CleanupJob
  internal/http/handler/trending.go     — 榜单/热点 API handler

修改:
  internal/jobs/poll_monitor.go         — 移除接口定义（import connector）
  internal/jobs/adapters.go             — 引用新接口路径
  internal/app/worker_jobs.go           — 重构为注册器模式
  internal/app/run.go                   — 注册新路由
  internal/database/models.go           — 新增 GORM 模型
  internal/jobs/publish_daily_topics.go — 扩展 HotEvent 覆盖
```
