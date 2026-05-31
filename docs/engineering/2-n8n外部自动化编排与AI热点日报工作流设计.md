# n8n 外部自动化编排与 AI 热点日报工作流设计

## 1. 设计结论

HotKey Server 当前已经具备 Go API 契约、进程内样机和完整数据库 Schema，但还不是完整的 AI 热点事件监测平台。下一阶段需要补齐真实 PostgreSQL/pgvector/Redis 持久化、内部 API、n8n 工作流和 SMTP 日报闭环。

n8n 在本项目中定位为外部自动化编排层，不是核心事实系统：

```text
n8n = 定时触发、外部来源编排、AI 处理编排、邮件发送、失败通知
hotkey-server = 事实写入、租户隔离、幂等、去重、可信度、pgvector 聚类、日报保存、审计
PostgreSQL/pgvector = 事实账本与语义索引
Redis = 缓存、锁、限流、队列和短期去重
```

核心边界：

- n8n 不直接写 PostgreSQL。
- n8n 不绕过 hotkey-server 的租户、权限、去重和审计。
- n8n 通过 hotkey-server internal API 完成所有事实写入和状态回写。
- n8n 的 workflow JSON 可以进入仓库版本管理，但凭证只能保存在 n8n Credentials 或部署环境中。

## 2. 当前完成度判断

当前可视为“平台骨架 + 契约样机 + 数据库总模型”阶段。

已完成：

- Go 服务可以启动，`go test ./...` 通过。
- OpenAPI 契约已经覆盖热点、关键词、来源、事件、日报、RBAC、租户、Redis/队列等主要接口方向。
- `db/schema.sql` 已建立全局数据库模型，覆盖 59 张表，并包含 pgvector 向量字段和中文数据字典注释。

未完成：

- 真实 PostgreSQL repository 尚未替换进程内仓储。
- 真实 Redis 客户端、锁、限流、队列和短期去重尚未接入。
- pgvector 相似召回和事件聚类尚未进入真实数据链路。
- 外部来源采集、内容清洗、AI 摘要、日报发送还没有形成自动化闭环。
- n8n 目录、workflow 模板、internal API 和执行状态回写尚未实现。

因此，平台不能判定为完成态。后续目标是先实现“外部来源采集 -> 后端入库 -> 事件聚类 -> 日报生成 -> SMTP 邮件发送”的最小生产闭环。

## 3. n8n 工作流目录

建议新增仓库目录：

```text
n8n/
  README.md
  workflows/
    daily_ai_hotspot_email_digest.json
    fact_source_collector.json
    signal_source_collector.json
    event_review_notification.json
```

目录规则：

- `n8n/README.md` 说明导入方式、所需 Credentials、环境变量和安全边界。
- `n8n/workflows/*.json` 只保存 workflow 模板，不保存真实 API Key、SMTP 密码、OAuth Token。
- workflow 命名使用小写 snake_case，并与业务用途一致。
- workflow 的写入动作只能调用 hotkey-server internal API。

## 4. 第一阶段工作流

第一阶段优先实现：

```text
daily_ai_hotspot_email_digest
```

目标：每天定时收集 AI 热点内容，触发后端事件聚类，生成昨日 AI 热点日报，并通过 SMTP 发送邮件。

推荐链路：

```text
Schedule Trigger
  -> 获取外部 AI 热点源
  -> 标准化内容字段
  -> POST /api/v1/internal/ingest/contents
  -> POST /api/v1/internal/jobs/event-clustering
  -> GET /api/v1/internal/reports/daily-candidates
  -> AI 生成 Markdown/HTML 日报
  -> POST /api/v1/internal/reports/daily
  -> SMTP 发送日报邮件
  -> POST /api/v1/internal/workflows/n8n/executions
```

推荐数据源分层：

- 事实源：OpenAI Blog、Anthropic News、Google DeepMind Blog、Meta AI、Mistral、arXiv、GitHub Release、官方论文页。
- 传播源：Hacker News、Reddit、YouTube、Product Hunt、X/Twitter、Bilibili、技术媒体 RSS。

事实源用于确认事件是否真实发生，传播源用于计算热度和趋势。日报中必须明确区分事实源证据和传播源证据。

## 5. Internal API 设计

n8n 只通过 internal API 与 hotkey-server 通信。

### 5.1 内容写入

```text
POST /api/v1/internal/ingest/contents
```

用途：接收 n8n 标准化后的外部内容，由后端完成租户校验、来源校验、幂等、去重、入库和审计。

必要字段：

```json
{
  "workflowName": "daily_ai_hotspot_email_digest",
  "executionId": "n8n-execution-id",
  "tenantId": "tenant-id",
  "sourceCode": "openai_blog",
  "sourceType": "fact",
  "items": [
    {
      "externalId": "source-item-id",
      "url": "https://example.com/news",
      "title": "AI event title",
      "summary": "short summary",
      "contentText": "full or extracted text",
      "contentType": "article",
      "language": "en",
      "region": "global",
      "authorName": "source author",
      "publishedAt": "2026-05-26T00:00:00Z",
      "rawPayload": {}
    }
  ]
}
```

必要响应：

```json
{
  "accepted": 10,
  "created": 7,
  "duplicated": 2,
  "rejected": 1,
  "runId": "collector-run-id"
}
```

### 5.2 事件聚类任务

```text
POST /api/v1/internal/jobs/event-clustering
```

用途：触发后端对新内容执行关键词粗召回、pgvector 相似召回、事件聚类、证据链写入和热点排名刷新。

必要字段：

```json
{
  "workflowName": "daily_ai_hotspot_email_digest",
  "executionId": "n8n-execution-id",
  "tenantId": "tenant-id",
  "windowStart": "2026-05-26T00:00:00Z",
  "windowEnd": "2026-05-27T00:00:00Z",
  "sourceScope": ["fact", "propagation"]
}
```

### 5.3 日报候选查询

```text
GET /api/v1/internal/reports/daily-candidates
```

查询参数：

```text
tenantId
reportDate
timezone
limit
minTrustScore
includeConflicts
```

用途：由后端返回结构化热点事件候选，n8n 或 AI 节点只负责表达整理，不负责决定事实归属。

### 5.4 日报保存

```text
POST /api/v1/internal/reports/daily
```

用途：保存最终日报 Markdown、HTML、结构化 JSON、选入事件和生成状态。

### 5.5 workflow 执行状态

```text
POST /api/v1/internal/workflows/n8n/executions
POST /api/v1/internal/workflows/n8n/errors
```

用途：记录 n8n 每次执行的开始、结束、成功、失败、错误信息、重试次数和关联任务。

必要字段：

```json
{
  "workflowName": "daily_ai_hotspot_email_digest",
  "executionId": "n8n-execution-id",
  "tenantId": "tenant-id",
  "status": "succeeded",
  "runStartedAt": "2026-05-27T00:00:00Z",
  "runFinishedAt": "2026-05-27T00:03:00Z",
  "errorMessage": "",
  "metadata": {}
}
```

## 6. 认证、幂等与安全

internal API 必须支持：

```text
X-HotKey-Internal-Key
X-HotKey-Tenant-ID
Idempotency-Key
```

要求：

- `X-HotKey-Internal-Key` 用于 n8n 调用鉴权。
- `X-HotKey-Tenant-ID` 明确租户上下文。
- `Idempotency-Key` 用于防止 n8n 重试导致重复写入。
- 后端必须校验来源是否存在、是否启用、是否允许该租户使用。
- 所有写入都要生成审计日志。
- n8n workflow JSON 中禁止出现真实密钥、邮箱密码和 Token。

## 7. SMTP 日报邮件

第一阶段邮件发送方式采用 SMTP。

n8n Credentials 中维护：

```text
SMTP_HOST
SMTP_PORT
SMTP_USER
SMTP_PASSWORD
SMTP_FROM
DAILY_REPORT_RECIPIENTS
```

仓库中只保存变量占位，不保存真实值。

邮件主题：

```text
[HotKey] AI 热点日报 - {{$json.reportDate}}
```

邮件内容：

- HTML 正文，用于直接阅读。
- Markdown fallback，用于归档和复制。
- Top AI 热点事件。
- 官方事实源证据。
- 社区传播热度。
- 冲突或待审核事件。
- 每个事件的来源链接。

邮件发送成功后，n8n 需要调用执行状态 API 回写结果。邮件发送失败时，n8n 需要进入错误分支并调用错误回写 API。

## 8. 错误处理与可观测性

n8n 负责自动重试、错误分支和通知；hotkey-server 负责最终状态存储。

建议状态落点：

- `collector_runs`：采集运行结果。
- `queue_messages`：后端异步任务状态。
- `report_generation_runs`：日报生成运行记录。
- `audit_logs`：internal API 调用审计。
- `outbox_events`：后续可靠事件发布。

常见失败策略：

- 外部来源失败：记录来源、错误码、错误信息，不阻塞其他来源。
- AI 摘要失败：保留后端结构化候选事件，日报标记为生成失败或使用规则模板降级。
- SMTP 失败：日报仍保存到后端，邮件发送状态标记失败。
- 后端 API 失败：n8n 按幂等键重试，超过次数后调用错误回写 API。
- pgvector 不可用：后端降级为关键词和标题规则聚合，并记录低置信状态。

## 9. 分阶段实施

第一阶段：最小闭环

- 新增 `n8n/README.md` 和 `daily_ai_hotspot_email_digest.json` 模板。
- 新增 internal API 契约。
- 后端支持内容 ingest、日报候选、日报保存和 n8n 执行状态回写。
- SMTP 发送日报。

第二阶段：来源扩展

- 增加 `fact_source_collector.json`。
- 增加 `signal_source_collector.json`。
- 来源按事实源和传播源分层调度。
- 后端增强来源可信度和采集运行记录。

第三阶段：审核与治理

- 增加 `event_review_notification.json`。
- 对低可信、冲突、高传播事件进入人工审核流。
- 审核结果回写 `event_trust_assessments` 和 `event_conflicts`。

第四阶段：生产化

- n8n 使用 queue mode。
- n8n 主实例负责触发，worker 负责执行。
- Redis 承担 n8n 执行队列和 hotkey-server 短期任务能力。
- hotkey-server 通过 outbox 和 queue 机制提升可靠性。

## 10. 验收标准

完成第一阶段后，必须满足：

- n8n workflow 可导入并使用占位凭证配置。
- n8n 不直接访问 PostgreSQL。
- n8n 能调用 hotkey-server internal API 写入内容。
- 后端能基于写入内容生成或刷新热点事件候选。
- 后端能返回日报候选事件。
- n8n 能生成 HTML/Markdown 日报。
- n8n 能通过 SMTP 发送日报邮件。
- workflow 成功和失败状态都能回写 hotkey-server。
- `go test ./...` 通过。
- workflow JSON 中不包含真实密钥。

## 11. 参考资料

- n8n Webhook Trigger：https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.webhook/
- n8n HTTP Request：https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.httprequest/
- n8n Queue Mode：https://docs.n8n.io/hosting/scaling/queue-mode/
- n8n Public API：https://docs.n8n.io/api/
- n8n 日报类工作流参考：https://n8n.io/workflows/4654-daily-insight-email-from-structured-web-data-with-firecrawl/
