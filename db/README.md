# HotKey 数据库

本目录保存 HotKey Server 的 PostgreSQL 完整数据库结构。

## 文件

- `schema.sql`：唯一建库 SQL，包含扩展、表、约束、索引与 pgvector 向量字段。

新环境直接执行该文件即可初始化数据库，不再维护分散的 migration 文件。

## 执行方式

确保 PostgreSQL 已安装 pgvector 扩展后执行：

```bash
psql -v ON_ERROR_STOP=1 -d <database> -f db/schema.sql
```

本地 Homebrew 环境：

```bash
brew install pgvector
```

Docker 生产编排（`docker-compose.prod.yml`）会在首次启动 Postgres 时自动挂载并执行 `db/schema.sql`。

## 业务域覆盖

`schema.sql` 覆盖当前服务端实现所需的核心表：

- 用户、会话、第三方授权
- 频道、订阅、用户关键词
- 来源、采集运行记录、X OAuth 凭证
- 原始内容、去重、质量过滤
- 内容向量、热点聚类、热点评分
- AI 摘要、日报、RSS、邮件投递
- 审计日志、后台清理任务、作业队列

## pgvector

首期 embedding 维度为 `1536`，与 `HOTKEY_EMBEDDING_DIMENSION` 及 DashScope `text-embedding-v2` 保持一致。
