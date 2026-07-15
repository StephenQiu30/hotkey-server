# HotKey Server 全量重建实施计划

日期：2026-07-15

分支：`refactor/hotkey-greenfield-rebuild`

设计事实源：`docs/design/001`—`008` 与 `AGENTS.md`

## 1. 目标

删除与新设计冲突的旧运行代码、旧数据库结构、旧测试和旧生成文档，按模块化单体目标重新建立可测试、可迁移、可扩展的 HotKey Server。

本计划不保留 Kafka、旧 Topic/Event/HotEvent、旧 PlatformPost、旧知识回写和旧 Schema 的兼容运行路径。旧数据库数据不迁移；新数据库从空库通过 Goose Migration 建立。

## 2. 范围边界

本次只重建 `hotkey-server`。`hotkey-web` 在新 OpenAPI 稳定后单独迁移。

保留：

- `LICENSE`、Git配置和仓库元数据
- `AGENTS.md`
- `docs/design/001`—`008` 与设计索引
- 与新架构仍一致的GitHub/编码协作配置；发现失效引用时再清理

删除或替换：

- 旧 `internal/controller`、`service`、`repository`、`model`、`queue`、`worker`、`fxapp` 等运行代码
- 旧 `db/schema.sql`、`db/tables` 和旧Migration
- 旧测试、Fixture和旧Swagger生成文件
- Kafka、LangChain、Redis核心任务和旧Embedding依赖
- Dockerfile、Docker Compose和部署脚本；本期不设计部署
- 旧README、Makefile、环境变量样例和验证脚本中的失效入口

## 3. 质量纪律

1. 每个行为先写失败测试或可执行结构校验，再写实现。
2. 删除旧代码后立即建立最小可编译骨架，不保留长期红色主线。
3. 每个业务表有统一CRUD Repository；运行表使用受限Repository。
4. Transport、Application、Domain、Infrastructure依赖方向由脚本验证。
5. 每个阶段使用独立提交：`test:`、`impl:`/`feat:`、`refactor:`、`docs:`或`chore:`。
6. 当前机器没有可用Go命令；在实施开始时安装仓库外或`.tools/`内的便携Go工具链，`.tools/`不得提交。
7. 未实际运行的测试不得标记为通过。

## 4. 阶段和任务

### 阶段A：旧系统清退与新骨架

#### A1. 建立结构验收测试

创建：

- `scripts/validate_architecture.ps1`
- `scripts/validate_repository.ps1`
- `tests/architecture/layout_test.go`

验证：

- 禁止旧顶层包：`controller`、`service`、`repository`、`model`、`queue`、`worker`、`fxapp`
- 禁止Kafka、LangChain和业务Redis依赖
- 强制 `internal/bootstrap`、`platform`、`shared`、`modules`
- 强制业务模块四层目录
- 禁止Domain导入Gin、GORM、River、MinIO和外部SDK

提交：`test: define greenfield architecture gates`

#### A2. 删除旧代码和文档

删除：

- 旧 `internal/` 全部运行代码
- 旧 `tests/` 全部测试和Fixture
- `db/schema.sql`、`db/tables/`、旧 `db/migrations/`
- 旧 `scripts/*.sh`
- `docs/docs.go`
- `docs/acceptance/`
- `docs/obsidian/dataview-examples.md`
- `Dockerfile`、`docker-compose.yml`、`.dockerignore`

保留设计文档。同步重写：

- `README.md`
- `CONTRIBUTING.md`
- `Makefile`
- `.env.example`
- `.gitignore`

提交：`refactor: remove legacy server implementation`

#### A3. 最小可编译应用

创建：

- `cmd/hotkey/main.go`
- `internal/bootstrap/app.go`
- `internal/bootstrap/role.go`
- `internal/platform/config/config.go`
- `internal/platform/logging/logger.go`
- `internal/platform/http/server.go`
- `internal/platform/http/router.go`
- `internal/platform/http/result.go`
- `internal/shared/errors/error.go`
- `internal/shared/clock/clock.go`
- `internal/shared/id/id.go`

行为：

- 支持 `serve --role=all|api|worker`
- 支持 `/healthz` 和 `/readyz`
- 配置错误启动失败
- 统一Result只由HTTP适配层输出
- API和Worker启动装配可独立测试

提交顺序：

- `test: define bootstrap and health behavior`
- `impl: create modular monolith bootstrap`

### 阶段B：数据库基础和完整Schema

#### B1. Goose和数据库命令

创建：

- `internal/platform/database/pool.go`
- `internal/platform/database/gorm.go`
- `internal/platform/database/migrate.go`
- `internal/platform/database/transaction.go`
- `cmd/hotkey/db_command.go`

行为：

- `hotkey db status`
- `hotkey db migrate`
- 启动时只检查Schema兼容，不自动迁移
- GORM和River通过受控数据库基础设施装配

#### B2. Migration拆分

创建：

- `db/migrations/20260715000100_extensions.sql`
- `20260715000200_identity_monitor_source.sql`
- `20260715000300_content_collection.sql`
- `20260715000400_event_entity_claim.sql`
- `20260715000500_intelligence_embeddings.sql`
- `20260715000600_knowledge_report_delivery.sql`
- `20260715000700_operations_indexes.sql`

覆盖 `003` 中31张业务表和17张运行表、外键、CHECK、部分唯一索引、GIN、BRIN和HNSW。

测试：

- 空库Up成功
- 关键约束拒绝非法状态、非法分数和多目标KnowledgeDocument
- 核心幂等唯一键生效
- 应用不包含AutoMigrate

提交顺序：

- `test: define database schema constraints`
- `impl: add versioned greenfield schema`

### 阶段C：共享领域契约与CRUD

创建：

- `internal/shared/pagination/cursor.go`
- `internal/shared/transaction/manager.go`
- `internal/shared/repository/crud.go`
- `internal/shared/repository/errors.go`
- `internal/platform/database/crud_helpers.go`

行为：

- `CRUDRepository[T,ID]`
- `ErrNotFound`、`ErrConflict`、`ErrUniqueViolation`
- `id + version`乐观锁
- 游标分页和排序允许列表
- 事务上下文显式传播

测试：泛型契约、错误映射、乐观锁、软删除、关系硬删除。

### 阶段D：Identity模块

目录：`internal/modules/identity/{domain,application,infrastructure,transport/http}`。

实现：

- users、user_preferences、auth_sessions
- 密码哈希、登录、刷新、退出、停用用户
- admin/editor/viewer授权
- 安全会话存储在PostgreSQL，不要求Redis

公共API：`/api/v1/auth/*`、`/api/v1/users/*`安全操作。

### 阶段E：Monitor和Source模块

实现：

- monitors、monitor_rules、monitor_sources、source_connections
- 监控主题统一CRUD和安全启停
- SourceConnection凭据引用
- Connector端口、健康检查、checkpoint和collection run
- RSS/Atom、Hacker News第一批Connector
- 运行时query_signature去重

测试：规则校验、正则限制、ETag/cursor、429退避、单来源失败隔离。

### 阶段F：Ingestion模块与MinIO

实现：

- source_authors、contents、content_assets、monitor_matches
- URL规范化、内容哈希、来源幂等和三层去重
- ObjectStore端口与MinIO适配器
- 确定性对象键、SHA-256和孤儿对账
- 内容指标快照

测试：规范化、重复检测、MinIO幂等、单条失败隔离、来源删除同步。

### 阶段G：Event和Intelligence模块

实现：

- events、event_contents、monitor_events
- entities、aliases、关系、claims和claim_evidences
- AI model profiles、ai_runs和证据引用
- 1024维Embedding表与pgvector查询
- 规则、语义和LLM边界复核
- 事件合并、拆分、人工锁和热度快照

测试：跨来源聚类、跨语言候选、主张立场、人工覆盖优先和事务回滚。

### 阶段H：Knowledge模块

实现：

- knowledge_documents、proposals、annotations、revisions、vault_sync_runs
- VaultStore端口和本地文件系统适配器
- 安全路径、Frontmatter、自动区域、哈希和路径锁
- 临时文件、Flush和原子Rename
- 人工编辑扫描、冲突、审核、归档和恢复

测试：路径逃逸、并发写、人工内容保护、崩溃残留和跨存储对账。

### 阶段I：Report和Delivery模块

实现：

- reports、report_items、subscriptions
- 日报、周报、范围、时区和发布快照
- report_deliveries、delivery_attempts
- MailSender端口、SMTP HTML/纯文本
- RSS和Atom、ETag、Last-Modified、私有Token轮换

测试：报告幂等、快照不漂移、邮件重试、永久错误、RSS 304和令牌失效。

### 阶段J：Operations、OpenAPI和总验收

实现：

- audit_logs、retention_policies、运行查询和批量清理
- MinIO/数据库/Vault对账
- OpenAPI和安全业务操作
- 新README、开发命令和本地配置说明
- 更新CI只执行真实存在的门禁

验证：

- `go test -race ./...`
- `go vet ./...`
- `go build ./cmd/hotkey`
- Architecture和Repository脚本
- Migration空库Up和约束测试
- RSS/HN -> Content -> Event -> Vault -> Report -> RSS/邮件端到端链路
- `git diff --check`

## 5. 完成定义

1. 仓库不存在旧运行包、旧表、Kafka和重复事件模型。
2. 最新设计文档中的需求都有代码所有者、表、用例和测试。
3. 所有业务表具有CRUD Repository，但公共API只提供安全业务操作。
4. 本地`all`角色可运行，`api`和`worker`角色可独立启动。
5. PostgreSQL、MinIO和Vault状态可对账，任务可幂等重试。
6. 日报周报可写入Vault并通过邮件及RSS/Atom交付。
7. 所有自动化门禁实际通过；缺少外部凭据的Connector使用契约测试和Fake验证。
