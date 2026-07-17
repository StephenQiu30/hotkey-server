# HotKey Server 项目规范

本文件是仓库级强制规范。代理或开发者必须先阅读本文件，再阅读任务涉及的 Design、PRD、Plan、Acceptance 和代码；不得把聊天记录或临时计划当作仓库事实。

## 项目边界与当前状态

HotKey Server 是本地优先的 AI 热点事件监控与 Obsidian 知识库治理后端，负责认证、监控、来源采集、内容标准化、相关性、事件、AI、报告、订阅、PostgreSQL、可靠任务和运行观测。不负责 Web 或 Miniapp 页面，前端只能消费后端 OpenAPI。

当前仓库处于绿色重建和持续交付阶段：

- 目标设计不等于当前实现；未完成任务不得描述为已上线或已验收。
- PRD/Plan/Acceptance 的权威索引分别位于 `docs/prd/README.md`、`docs/plans/README.md` 和 `docs/acceptance/README.md`；设计索引位于 `docs/design/README.md`，运行手册索引位于 `docs/operations/README.md`。
- 代码开工必须同时满足：PRD `status: accepted`、Plan `status: accepted` 且 `review_status: approved`、前置 Plan 已完成、当前 `execution_status: ready`，并且验收已有可执行红灯/绿灯或替代证据。
- 目标、范围、文件、步骤、依赖或验收变化后，Plan 必须重新审核；不得沿用旧批准状态。
- 实施任务必须同步代码、完整 `db/schema.sql`、记录模型、OpenAPI、测试和架构校验；纯设计不得伪装成已实现。

## 架构与目录

使用单仓库、单二进制的模块化单体。`cmd/hotkey` 支持 `all`、`api`、`worker` 角色；角色共享代码和 PostgreSQL，但不得依赖进程内共享业务状态。后台任务、检查点和审核状态写入 PostgreSQL。

技术栈：Go 1.26、Gin、GORM v2、Fx、Viper、PostgreSQL 16+、pgvector、pgx v5、River、MinIO、cron、Zap、OpenTelemetry、Prometheus、JWT/bcrypt、官方 LLM SDK 和可选 ONNX Runtime。

```text
cmd/hotkey/
internal/bootstrap/       # Fx 装配和生命周期
internal/platform/        # HTTP、DB、Queue、MinIO、Vault、邮件、观测
internal/shared/          # 错误、分页、事务、Clock、ID
internal/modules/{identity,monitor,source,ingestion,event,
  intelligence,knowledge,report,delivery,operations}/
db/schema.sql
test/                     # 全部测试源码、fixture、runner、门禁工具
docs/{design,prd,plans,acceptance,operations}/
```

每个业务模块按职责使用 `domain/`、`application/`、`infrastructure/`、`transport/http/`；没有职责不得创建空目录或抽象层。

依赖方向固定为：

```text
transport/http -> application -> domain
infrastructure -> domain
bootstrap -> all adapters
```

- Transport 只处理协议、参数、认证上下文和 Result；Application 负责用例、权限、事务和跨模块编排；Domain 只保存规则、实体、值对象和端口；Repository 只读写数据。
- 跨模块调用通过目标模块 Application 接口或只读查询端口；业务模块不得直接读取其他模块拥有的表。
- Domain 禁止导入 Gin、GORM、River、MinIO 或第三方 SDK；第三方类型不得穿透 Infrastructure；禁止包级业务单例和全局可变状态。
- Redis 只用于缓存、验证码、短期票据和限流，不是业务事实源或核心任务依赖。禁止重新引入 Kafka、微服务、内部事件总线、Elasticsearch、独立向量库、动态插件框架和通用规则/工作流引擎。

## HTTP 契约

所有 JSON 接口只返回 `code`、`message`、`data`；成功业务码为 `0`，无数据使用 `data: null`，分页信息放入 `data`。

- 业务 Transport 禁止直接调用 `c.JSON`、`AbortWithStatusJSON` 或 `String`；必须使用统一 Result 和全局错误处理器。
- HTTP 状态码保留协议语义；客户端依赖业务 `code`，不得依赖 `message` 文案。
- `X-Request-ID` 只放响应头和日志，不放业务 JSON；错误不得泄露堆栈、SQL、密钥或第三方原始错误。
- API、错误码、DTO、OpenAPI 和 Transport 测试必须一起更新。

## 数据库与运行配置

- `db/schema.sql` 是唯一结构事实源；禁止 `db/migrations/`、分片 Schema、第二套快照、Goose 和 GORM `AutoMigrate`。
- 初始化使用 `go run ./cmd/hotkey db init --empty-only --confirm-empty`，`db verify` 只读；服务启动只检查 Schema 兼容性。
- 每张业务表必须有明确查询/生命周期需求和 Repository；事务、GORM、River 复用同一 PostgreSQL pool。每个进程只创建一个 `*pgxpool.Pool`。
- 时间统一 UTC `timestamptz`；业务分数为 `0–100` 并有 CHECK；Embedding 必须保存模型/版本，当前向量契约为 `halfvec(1024)`。
- 业务事实在 PostgreSQL，原始证据在 MinIO，人类可读知识投影在本地 Vault；核心历史、审计、运行记录不得被静默覆盖。
- 只使用两个配置文件：默认 `.env`，以及 `HOTKEY_ENV=production` 时覆盖读取的 `.env.prod`；进程环境变量优先级最高。
- JWT 和认证 HMAC secret 每个环境至少 32 字节，不得使用不安全默认值；运行服务使用 `HOTKEY_DATABASE_URL`，测试使用可丢弃的 `HOTKEY_TEST_DSN`，认证集成测试使用独立 `HOTKEY_TEST_REDIS_URL`。

来源只能使用官方 API、RSS、Atom 或授权 Feed；不得绕过登录、验证码、反爬或平台访问限制。外部调用必须有超时、限流、指数退避、熔断和可恢复错误处理。

## 测试、CI 与提交

- 所有 `*_test.go` 必须位于 `test/`；业务目录不得提交测试文件。`test/_suite/` 按业务包镜像保存测试，使用 `go run ./test/runner test <package>` 或 `make test` 执行；runner 创建的临时映射必须在退出时清理。
- `test/tools/` 只保留被 Makefile、测试或 CI 调用的门禁工具；无调用的临时脚本、fixture 和数据库必须删除。提交前不得残留符号链接、测试数据库、dump、SQLite 文件或根目录构建产物。
- 本地至少按风险运行 `make lint`、`make test`、`make build`、`make validate`、`git diff --check`；涉及 Schema、OpenAPI、依赖或 CI 时运行 `make ci`，完成后 `make clean`。
- GitHub Actions 在 `main` push、面向 `main` 的 Pull Request 和手动触发时运行唯一质量门禁 `make ci`，使用临时 PostgreSQL/pgvector 和 Redis。
- 行为变更先写失败测试，再做最小实现；架构/契约变化先更新 Design，再同步 PRD、Plan、Acceptance 和可执行事实源。
- 提交只包含当前任务文件，工作区必须干净；本项目按既定流程直接提交并推送 `main`，禁止私自修改其他仓库。

## 文档规范

- Design 只记录长期技术决策；PRD 记录稳定范围和依赖；Plan 记录明确文件、步骤、验证、回滚和提交边界；Acceptance 记录与准确提交关联的长期证据；Operations 记录可重复运行、发布、升级、回滚和故障流程。
- 正式文档必须有完整 frontmatter，状态只使用 `draft`、`review`、`accepted`、`archived`；PRD/Plan 的执行状态只使用项目索引规定的枚举。
- 除固定入口 `README.md` 外，所有正式文档使用 `序号-中文主题.md`，序号与 `doc_no` 一致。Operations 目录当前使用 `001–005` 自有编号，关联的 PLAN 编号写在标题和正文中；禁止英文泛化名、仅序号名或 `runbook.md`、`schema-upgrade.md` 等无语义名称。
- 已完成且有长期验收证据的 Design、PRD、Plan、Acceptance 移入各自目录的 `archive/`；未完成内容不得归档。所有路径变化必须同步索引、README、测试和 canonical path。
- 运行手册不得写入真实密钥、Token、DSN、个人绝对路径或一次性终端流水；Vault 工作流不得执行 Git 操作。

提交前检查：`git diff --check`、相关测试、架构/仓库校验、OpenAPI 漂移、工作区状态、临时数据库和生成物。优先明确业务代码，只有出现第二个真实实现或明确替换需求时才提取抽象。
