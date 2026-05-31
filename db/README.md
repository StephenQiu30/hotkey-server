# HotKey 数据库设计

本目录保存 HotKey Server 的 PostgreSQL 完整数据库结构。

## 文件说明

- `schema.sql`：完整建库 SQL，包含扩展、函数、表、约束、索引、pgvector 向量字段和中文数据字典注释。

当前阶段使用单文件 Schema，方便开源用户阅读、审查和初始化数据库。后续如果引入正式迁移工具，可以从 `schema.sql` 拆分生成版本化 migration，但事实来源仍以本文件定义的全局模型为准。

## 设计顺序

数据库设计必须发生在 API、Repository 和 Worker 实现之前：

1. 需求分析
2. 顶层业务域划分
3. 核心实体与关系建模
4. 数据生命周期设计
5. 数据库表、字段、约束、索引和向量策略设计
6. Repository、Service、API 和 Worker 实现

## 业务域覆盖

`schema.sql` 覆盖以下业务域：

- 租户、用户、身份绑定、成员关系
- 复杂 RBAC、API Key、审计
- 套餐、订阅、账单、计费事件、用量统计
- 热点词、别名、用户偏好、监控规则
- 事实源、传播源、来源账号、授权凭证、合规策略
- 采集任务、原始内容、内容资产、内容版本、传播指标
- 内容事实主张、传播路径、相似度规则
- pgvector 内容向量和事件向量
- 热点事件、事件证据链、事件关系、事实冲突仲裁
- 日报、日报生成记录、热点排名快照
- 队列消息、Worker 锁、实时频道、实时事件、通知和 Webhook

## 命名规范

- 表名统一使用复数 `snake_case`。
- 字段名统一使用 `snake_case`。
- 主键统一使用 PostgreSQL 生成的 UUID。
- 租户隔离表必须包含 `tenant_id`。
- 可变业务表必须包含 `created_at` 和 `updated_at`。
- 需要软删除的业务表使用 `deleted_at`。
- 结构化扩展字段使用 `jsonb`。
- 向量字段使用 pgvector，并与原始事实表分离。

## pgvector 策略

第一阶段向量维度统一为 `1536`。如果后续更换 embedding 模型并导致维度变化，应新增迁移或新表，不要在同一向量列中混合不同维度。

向量表分为两类：

- `content_embeddings`：原始内容语义向量，用于内容相似度召回。
- `event_embeddings`：热点事件语义向量，用于事件聚类、归并和相似事件发现。

## 执行方式

确保 PostgreSQL 已安装 pgvector 扩展后执行：

```bash
psql -v ON_ERROR_STOP=1 -d <database> -f db/schema.sql
```

本地 Homebrew 环境可使用：

```bash
brew install pgvector
```
