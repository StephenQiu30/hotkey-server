# HotKey Server 项目规范

本文件是 `hotkey-server` 的项目级架构与工程约束入口，适用于仓库根目录及全部子目录。代理开始工作前必须先阅读本文件，再读取任务涉及的设计文档和代码。

## 项目定位

`hotkey-server` 是 AI 热点事件监控平台的后端服务。平台允许内容创作者配置热点词，从合规的中英文新闻、社交和视频来源发现相关内容，并生成可追溯的热点事件卡片。

本仓库负责：

- 用户、认证、会话和权限
- 监控词、AI 扩展词和查询规划
- 数据来源、采集、标准化和去重
- 多语言匹配、事件聚类、热度和趋势
- AI 摘要、提醒、报告和管理接口
- PostgreSQL 数据、定时流水线和运行观测

本仓库不负责 Web 页面和 Miniapp 页面。前端只能消费后端发布的 OpenAPI 契约。

## 当前状态与目标状态

项目正在进行全面架构重设计。现有代码和旧设计文档只代表迁移前现实，不约束目标架构。

- 不得把目标设计描述成已经实现的能力
- 不得依据旧代码复制 `topic`、`event`、`hot_event` 等重复模型
- 未经批准的实施计划，不得批量重写现有代码
- 迁移任务必须同时更新代码、`db/schema.sql`、OpenAPI、测试和架构校验
- `docs/design/README.md` 是详细设计索引

本文件与 `docs/design/README.md` 共同构成目标架构基线。

## 基础技术栈

新架构使用以下技术：

- Go 1.26
- Gin
- GORM v2
- go.uber.org/fx
- Viper
- PostgreSQL 16+ 和 pgvector
- robfig/cron
- Zap
- OpenTelemetry 和 Prometheus
- validator/v10
- Swaggo 和 Gin Swagger
- JWT v5 和 bcrypt
- 官方 LLM SDK，封装在项目 Provider 接口之后
- ONNX Runtime，用于可选的本地多语言 Embedding

Redis 只能用于缓存、短期会话或限流，不得成为业务事实源。MVP 不引入 Kafka、微服务或独立向量数据库。

## 运行架构

项目使用单仓库、单二进制、单进程的模块化单体。`cmd/hotkey/main.go` 启动一个 Fx 应用，并统一管理：

- Gin HTTP Server
- Cron Scheduler
- 有界并发的 Pipeline Executor
- 数据源 Connector
- 数据库和可选 Redis 客户端
- 日志、指标、链路和优雅停机

不得拆分 API 和 Worker 运行角色。后台任务必须在同一进程内运行，并将状态和检查点写入 PostgreSQL。

## 目标目录

```text
cmd/hotkey/main.go
internal/
├── app/                     # Fx 应用组装和生命周期
├── config/                  # 配置加载与校验
├── platform/                # HTTP、DB、安全、日志、调度和 AI 基础能力
└── module/
    ├── identity/
    ├── monitor/
    ├── source/
    ├── queryplan/
    ├── content/
    ├── matching/
    ├── event/
    ├── ranking/
    ├── notification/
    ├── report/
    └── operations/
db/schema.sql
docs/design/
```

每个业务模块按实际需要包含 `entity.go`、`dto.go`、`repository.go`、`service.go`、`controller.go`、`route.go` 和 `module.go`。不要为没有对应职责的文件创建空壳。

当前代码尚未完成该目录迁移。只有架构基础任务可以移动目录，并且必须同步更新构建、测试和架构校验。

## 依赖边界

模块内部依赖方向固定为：

```text
Controller -> Service -> Repository -> PostgreSQL
```

必须遵守以下规则：

- Controller 只处理协议、参数、认证上下文和 Result 输出
- Service 保存业务规则、事务边界和用例编排
- Repository 只处理数据读写，不调用 Controller 或 Service
- 跨模块调用通过目标模块的 Service 接口
- `platform/` 不包含监控、事件、热度等业务规则
- 只在跨模块或外部依赖边界定义接口
- 禁止全局可变状态和包级业务单例
- 禁止业务模块直接读取其他模块拥有的表

## 统一 Result 响应

所有 JSON 接口只返回 `code`、`message`、`data`：

```go
type Result[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}
```

响应必须遵守以下规则：

- 成功业务码为 `0`
- 无数据时返回 `data: null`
- 分页信息封装在 `data` 中
- Controller 禁止直接调用 `c.JSON`
- JSON 只能通过统一 Result 工具输出
- HTTP 状态码保留标准协议语义
- 客户端依赖业务 `code`，不得依赖 `message` 文案
- `X-Request-ID` 放在响应头和日志中，不加入 Result
- 错误响应不得泄露堆栈、SQL、密钥或第三方原始错误

Service 返回领域错误。全局错误处理器负责将领域错误、参数错误和 panic 转换为 Result。

## 数据库规则

`db/schema.sql` 是唯一数据库结构事实源。不得创建 `db/migrations/` 或第二套 schema。

- 普通 CRUD 使用 GORM
- 复杂聚合、批量写入和 pgvector 查询可以使用参数化原生 SQL
- 核心查询、关联和排序字段使用独立列
- 只有来源扩展字段、指标和评分解释可以使用 JSONB
- 不为每个来源创建独立内容表
- 时间统一使用 `timestamptz` 和 UTC
- 分数统一归一化为 `0-100` 并增加 CHECK 约束
- Embedding 必须保存模型和版本，不得混用向量空间
- 新增表必须对应明确的查询、约束或数据生命周期需求

使用软删除的数据包括用户、监控任务、监控词和用户通知。临时、可重建或超过保留期的数据使用硬删除。事件合并和内容失效使用状态字段，不使用通用软删除。

## 核心数据关系

目标模型只保留一个全局事件概念：

```text
Monitor -> MonitorEventMatch -> Event -> EventContent -> ContentItem
```

- `ContentItem` 保存新闻、帖子和视频的统一内容
- `Event` 保存跨来源、跨语言的热点事件
- `MonitorEventMatch` 保存用户上下文中的相关性和排序
- 同一个 Event 可以匹配多个 Monitor
- 同一事件的独立报道必须作为证据保留，不得误判为重复内容

## 来源与采集

MVP 只接入官方 API、RSS、Atom 或授权 Feed。不得绕过登录、验证码、反爬或平台访问限制。

每个来源实现小型 `Connector` 接口，负责采集、分页、游标、配额和统一 SourceItem 转换。不要实现动态插件加载、反射注册或通用采集 DSL。

查询计划按来源、语言、地区和词组生成稳定签名。多个 Monitor 必须复用相同 QueryPlan，避免重复消耗平台配额。

## 匹配、事件和 AI

相关性使用关键词、实体、多语言向量和用户偏好的混合评分。排除词和冲突实体必须抑制语义漂移。

- LLM 负责扩词、实体辅助、摘要和复杂边界判断
- LLM 不负责采集、基础去重、热度计算或唯一事实判定
- Embedding 或 LLM 失败时，关键词和实体匹配仍应工作
- 每个匹配结果必须保存命中解释和评分版本
- 每个 AI 摘要必须引用有效的原始内容
- 用户反馈只进入评测和词项建议，MVP 不做在线训练

## 定时流水线

Cron 每小时触发：

```text
查询规划 -> 来源采集 -> 内容标准化 -> 相关性匹配
-> 事件聚类 -> 热度计算 -> AI 摘要 -> 通知评估
```

流水线必须满足：

- 使用 `context` 传递取消和超时
- 使用有界并发，不创建无限 goroutine
- 使用 PostgreSQL advisory lock 防止同一任务重入
- 使用 `pipeline_runs` 和采集运行记录保存状态
- 所有写入支持幂等重试
- 单一来源失败不阻塞其他来源
- 进程重启后可以从数据库状态恢复
- 外部调用遵守限流、超时、指数退避和熔断规则

## 避免过度设计

禁止引入：

- 微服务和分布式事务
- Kafka 或内部事件总线
- 通用 `Repository[T]`
- 通用规则引擎和工作流引擎
- 动态插件框架
- 独立 Elasticsearch 或向量数据库
- 没有当前用例的抽象层、配置项和数据表

优先编写明确的业务代码。只有出现第二个真实实现或明确替换需求时才提取抽象。

## 工程质量

行为变更遵循测试先行。架构和契约变更先更新设计，再实施代码。

常用验证命令：

```bash
make lint
make test
make build
make validate
make openapi
make openapi-validate
make smoke
make ci
git diff --check
```

提交前必须：

- 运行与改动风险相称的测试
- 验证 `db/schema.sql` 与实体一致
- 验证架构边界和 OpenAPI
- 检查仓库根目录没有构建产物
- 保持 staging 仅包含当前任务文件
- 不修改 HotKey 的其他仓库，除非任务明确扩大范围

## 文档同步

架构、模块边界、数据库模型、API 契约或运行方式发生变化时，必须同步更新：

1. `AGENTS.md` 中的强制约束
2. `docs/design/` 中的详细设计
3. `docs/design/README.md` 中的权威索引
4. `db/schema.sql` 和 OpenAPI 等可执行事实源

聊天记录、临时计划和一次性排查过程不能替代仓库文档。
