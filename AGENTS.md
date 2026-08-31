# HotKey 单体仓库规范

本文件是整个仓库唯一的 `AGENTS.md`，适用于根目录、`backend/`、`agent/`、`frontend/` 与 `docs/`。子目录不得再创建同名规则文件；目录归属通过本文件的分区规则表达。

## 目录与单一事实源

- `backend/`：Go 后端、完整 PostgreSQL Schema、运行时 OpenAPI、任务系统、后端 Dockerfile 与应用环境示例。
- `agent/`：Python 数据分析服务、版本化分析契约、Python 测试、Agent Dockerfile 与服务环境示例；不得承载业务 API 或业务事实。
- `frontend/`：Next.js Web 工作台、页面组件、生成的 OpenAPI 客户端、前端 Dockerfile 与应用环境示例。
- `docs/`：唯一正式文档树，统一使用 `design/`、`prd/`、`plans/`、`acceptance/`、`operations/` 与 `openapi/` 分类。
- `.github/`、`.codex/`、`.gitignore`、`.gitattributes`、`.editorconfig`、`LICENSE`、`CONTRIBUTING.md` 与 `SECURITY.md`：只在根目录维护。
- `docker-compose.yml`：Go Core、Python Agent、前端和基础设施的公共编排基线；根目录的日常、生产覆盖文件只保存环境差异。
- `docs/openapi/swagger.json`：唯一发布 OpenAPI；`backend/openapi/docs.go` 是同一契约的运行时生成代码。
- `backend/db/schema.sql`：唯一数据库结构事实源。

仓库只能存在根目录的 `.git/`、`.github/`、`.codex/` 和 `AGENTS.md`。不得在子项目复制通用配置、正式文档、Compose 公共服务定义或仓库元数据。

## 通用工作规则

- Go 后端命令从 `backend/` 执行，Python Agent 命令从 `agent/` 执行，前端命令从 `frontend/` 执行，Compose 与跨项目命令从仓库根目录执行。
- 项目运行只通过根 `docker-compose.yml` 或 `docker-compose-prod.yml` 启动；不得把本机直接运行 API、Worker、Python Agent 或 Next.js 开发服务器写成项目启动方式。
- 日常格式、静态、单元与集成测试直接使用本机已安装并锁定的 Go、Python/uv、Node 工具链，以及已存在的可丢弃 PostgreSQL/Redis 测试实例；不得为每轮测试反复启动或停止整套 Compose。只有 Compose 配置、新鲜容器 E2E、发布、备份恢复或用户明确要求的隔离验收才创建独立容器栈，并在该门禁结束后清理。
- 修改前先阅读相关 Design、PRD、Plan、Acceptance、Operations 和现有测试；目标设计不得描述为当前已实现能力。
- 行为变更遵循测试先行：先保存可复现失败，再做最小实现，最后重构并运行相关回归。纯文档和机械迁移可说明为何不新增行为测试。
- 只在出现第二个真实实现或明确替换需求时提取抽象；避免空目录、占位层、重复 DTO、重复配置和第二套事实源。
- 跨端契约、目录、构建或部署入口变化时，同步更新根 README、CI、正式文档和测试。
- 不提交 `.env`、密钥、Token、个人数据、数据库 dump、Vault、对象存储内容、依赖目录、构建产物、日志或本地工具状态。

## 后端规则

### 架构边界

Go Core 是唯一业务后端和事实拥有者，`cmd/hotkey` 支持 `all`、`api` 与 `worker` 角色。Python Agent 是只返回结构化建议的内部分析服务，不改变 Go Core 的模块化单体依赖方向：

```text
transport/http -> application -> domain
infrastructure -> domain
bootstrap -> all adapters
```

- Transport 只处理协议、参数、认证上下文和统一 Result；Application 负责用例、权限、事务与跨模块编排；Domain 保存规则、实体、值对象和端口；Repository 只读写数据。
- 跨模块调用使用目标模块的 Application 接口或只读查询端口，不直接读取其他模块拥有的表。
- Domain 不得导入 Gin、GORM、pgx、River、MinIO 或第三方 SDK；第三方类型不得穿透 Infrastructure。
- `internal/shared/` 只保存基础设施中立的稳定类型和契约，不导入 Platform 或具体适配器；通用数据库适配器位于 `internal/platform/database/repository`。
- Redis 只用于缓存、验证码、短期票据与限流，不是业务事实源。除根目录 `agent/` 的 Python 分析服务外，不得引入第二套业务后端、第二套 Schema、Kafka、其他微服务、内部事件总线、Elasticsearch、独立向量库或通用规则引擎。

### 分层对象约定

- Alibaba Java 开发手册只作为语言中立的分层与对象语义参考，不在 Go 中复制 `controller/service/mapper/pojo` 顶层目录。
- Domain 保存实体、值对象和端口；Application 不得导入任一模块的 Infrastructure 或 Transport；Transport 不得导入 Infrastructure。既有 Application 对 `internal/platform/database` 事务协调器的有限引用由架构测试冻结，不得扩散；新用例使用 Domain/Shared 端口。
- PostgreSQL 行结构属于 Infrastructure 私有 Record（Go 中使用未导出类型）；Application 的 Command、Query、Result、DTO 使用纯 Go 类型；HTTP 只暴露明确的 `RequestDTO` / `ResponseDTO`，不得把 GORM Model、数据库 Record 或第三方 SDK 对象向上透传。
- 兼容迁移可在适配器边界保留短期类型别名，但新生产调用方必须引用内层规范类型，并由架构测试阻止新增反向依赖。

## Python Agent 规则

- Agent 只负责相关性、聚类候选、摘要、实体/主题提取等数据分析与模型编排；业务授权、幂等、状态迁移、Evidence 白名单、最终写入和人工治理必须留在 Go Application/Domain。
- Agent 不得持有 PostgreSQL DSN、Redis URL、MinIO/Vault 凭据、来源 Token、浏览器 JWT 或任意用户会话；不得直接访问业务表、对象存储、Vault 或外部来源。
- Go Worker 通过内部版本化 HTTP 契约调用 Agent。请求只包含任务身份、契约版本、有界输入、允许 Evidence ID 与追踪信息；Agent 响应只包含经 Pydantic/JSON Schema 校验的建议、计量和稳定错误码。
- Python 包、应用和测试分别位于 `agent/src/hotkey_agent/` 与 `agent/tests/`；使用 `pyproject.toml` 管理依赖，生产依赖必须锁定并通过高危漏洞门禁。
- Agent 必须提供存活与就绪检查，限制请求体、并发、超时和输出大小；日志不得记录正文、Prompt、Token、密钥或完整模型原始响应。
- Agent 不得把数据分析接口暴露给浏览器或公网。Compose 中只供内部网络访问，生产环境必须使用独立的服务认证密钥并支持禁用；不可用时 Go 确定性链路继续并保留待补算状态。
- Agent 行为变更遵循 Python 测试先行，并至少运行格式、静态检查、类型检查、单元测试和依赖审计；跨语言契约必须同时由 Python 与 Go 契约测试保护。

### HTTP 与数据

- JSON 接口只返回 `code`、`message`、`data`；成功业务码为 `0`，无数据使用 `data: null`。
- 业务 Transport 使用统一 Result 和全局错误处理，不直接调用 `c.JSON`、`AbortWithStatusJSON` 或 `String`。
- 客户端依赖业务 `code`，不依赖 `message` 文案；错误不得泄露堆栈、SQL、密钥或第三方原始错误。
- API、错误码、DTO、OpenAPI 与 Transport 测试必须一起更新。
- 禁止 `db/migrations/`、分片 Schema、第二套快照、Goose 与 GORM `AutoMigrate`。初始化使用 `go run ./cmd/hotkey db init --empty-only --confirm-empty`，服务启动只验证兼容性。
- PostgreSQL 是业务事实源，MinIO 保存原始证据，本地 Vault 保存人类可读投影；核心历史和审计记录不得静默覆盖。
- JWT 与认证 HMAC secret 每个环境至少 32 字节。运行服务使用 `HOTKEY_DATABASE_URL`，测试使用可丢弃的 `HOTKEY_TEST_DSN` 与独立 `HOTKEY_TEST_REDIS_URL`。
- 数据来源只能使用官方 API、RSS、Atom 或授权 Feed，不得绕过登录、验证码、反爬或平台访问限制。

### 后端测试与生成物

- 所有 `*_test.go` 与纯测试 fixture 位于 `backend/test/`；业务目录不得提交测试源码。
- `test/_suite/` 按业务包镜像保存测试，使用 `go run ./test/runner test <package>` 或 Makefile 入口执行。
- OpenAPI 只由 Swaggo 注解生成：`make openapi` 同步写入 `backend/openapi/docs.go` 与 `docs/openapi/swagger.json`，不得手工编辑生成物。
- 涉及 Schema、OpenAPI、依赖或 CI 时运行 `make ci`；完成后清理临时映射、测试数据库和根构建产物。

## 前端规则

### 技术与目录

- 使用 Next.js App Router、React、TypeScript、Tailwind CSS、现有 Radix/组合组件、lucide-react、Zustand、Axios、Recharts 与 GSAP。
- 页面位于 `src/app/`，业务组件位于 `src/components/`，布局位于 `src/layouts/`，状态位于 `src/stores/`，通用请求与工具位于 `src/lib/`。
- 所有 `*.test.ts`、`*.test.tsx` 与测试初始化位于 `frontend/test/`；不得在 `src/` 内创建测试文件。
- 单文件通常保持在 200–500 行；超出时按职责拆分，不创建无职责包装层。
- 前端按 React 语义实现 MVVC/MVC：生成的 OpenAPI Client 与契约类型是 Model，`src/app/` 页面负责 Route/ViewController 编排，`src/components/` 是 View，`src/stores/` 保存跨页面状态；复杂多步 API 工作流进入聚焦的 `src/lib/` 工作流模块，展示组件不得拥有该工作流。

### API 与界面

- 前端只消费 `docs/openapi/swagger.json` 生成的 `src/services/hotkey/hotkey-server/` 客户端；不得手写后端 DTO、接口路径或重复服务层。
- 修改后端契约后，先在后端生成 OpenAPI，再在前端运行 `npm run openapi:generate` 和 `npm run openapi:check`。
- 复用现有设计令牌和 UI 组合组件，保持键盘可操作、可见焦点、语义化标签、合理对比度与 `prefers-reduced-motion` 支持。
- 同时覆盖桌面与移动布局，以及正常、空、加载、错误和权限不足状态。
- `HOTKEY_API_ORIGIN` 只供 Next.js 服务端 rewrites 使用，不以 `NEXT_PUBLIC_*` 暴露后端密钥或内部地址。

## 文档规则

- `docs/` 不按前端/后端建立二级目录；frontmatter 的 `scope` 只能是 `backend`、`frontend` 或 `shared`。
- 正式文档的 `canonical_path` 必须以 `docs/` 开头并指向实际文件。同一分类当前目录与 `archive/` 的编号不得重复。
- 除固定入口 `README.md` 外，正式文档使用 `序号-中文主题.md`，序号与 `doc_no` 一致。
- Design 记录长期技术决策；PRD 记录稳定范围；Plan 记录文件、步骤、验证与回滚；Acceptance 保存长期证据；Operations 保存可重复运行流程。
- 状态只使用各分类索引规定的枚举；已完成且有长期验收证据的文档移入 `archive/`，未完成内容不得提前归档。
- 文档不得包含真实密钥、Token、DSN、个人绝对路径或一次性终端流水。路径变化必须同步 frontmatter、索引、链接、测试和 README。

## Docker 与环境配置

- 根 Compose 是开发、验收与生产的唯一项目运行入口；本机工具链仅用于构建、生成和测试，不直接承载项目服务进程。
- Dockerfile 与 `.dockerignore` 保留在 `backend/`、`agent/` 与 `frontend/`，因为构建上下文不同；不得为了文件名相同而合并不同应用镜像。
- `docker-compose.yml` 与 `docker-compose-prod.yml` 都保存完整服务、环境变量、依赖、健康检查、初始化命令和卷；prod 文件只将默认环境值替换为生产环境值、生产凭据和 prod 镜像标签。
- 日常 Compose 内置明确的非生产默认值，不要求创建 `.env` 或 `backend/.env`；根 `.env` 只在端口冲突或同机多实例时覆盖编排参数。生产环境读取根 `.env.prod`，基础设施只接收所需凭据且不发布宿主机端口。
- 上游镜像使用 `latest`；`pgvector/pgvector` 使用官方浮动 `pg16` 以保持 PostgreSQL 16 数据卷兼容，Python Agent 使用官方 `3.12-slim` 以匹配锁定运行时。应用镜像使用 `env` / `prod` 标签。
- 子项目不得新增 Compose 文件、生产环境模板、启动脚本或重复部署入口。

```bash
# 日常环境
docker compose -f docker-compose.yml up --build -d

# 生产环境
docker compose --env-file .env.prod \
  -f docker-compose-prod.yml up --build -d
```

## Git、评审与交付

- 提交只包含当前任务文件；提交前检查工作区、生成物、冲突标记和敏感信息。
- Git 提交标题统一使用 Conventional Commits：`<type>(<scope>): <subject>`。`scope` 必填，使用稳定的小写英文模块名（如 `backend`、`frontend`、`docs`、`ci`、`repo`）；`subject` 必须使用简体中文动宾短语，冒号后保留一个空格，标题不超过 72 个字符。
- 允许的 `type` 为 `feat`、`fix`、`test`、`refactor`、`docs`、`chore`、`perf`、`build`、`ci` 与 `revert`；禁止使用 `impl`、无 scope 前缀、英文主题或 `feat():xxx` 这类空 scope/缺少空格的变体。示例：`feat(frontend): 新增监控空状态`。
- 提交正文和脚注统一使用简体中文；正文按“变更摘要”“变更原因”“验证”三个段落记录实际内容、原因与已通过命令，禁止使用无信息量的“更新代码”“修复问题”。
- 不兼容变更使用 `<type>(<scope>)!:`，并在脚注以 `BREAKING CHANGE: <中文迁移说明>` 记录影响和迁移方式。
- 一个提交只完成一个可独立验收的任务；生成物必须与源文件同提交，纯格式化、顺手重构和无关文档不得混入功能提交。
- 推荐格式：`feat(operations): 完成配额保留与审计治理`；修复示例：`fix(frontend): 恢复弹窗关闭后的触发器焦点`；文档示例：`docs(repo): 统一中文提交信息规范`。
- 行为变更的提交顺序保持测试、最小实现、重构/文档可审查；不得通过放宽断言掩盖失败。
- Pull Request 说明用户影响、实现边界、测试命令与结果、Schema/OpenAPI/配置/部署影响和残余风险。
- 未经用户明确要求，不创建提交、不推送、不创建或合并 Pull Request。

## 验证入口

```bash
cd backend
make ci

cd ../frontend
npm ci
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ../agent
uv sync --all-extras --locked
uv run ruff format --check .
uv run ruff check .
uv run mypy src
uv run pytest
uv run pip-audit

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod \
  -f docker-compose-prod.yml config --quiet
git diff --check
```

按变更风险运行必要检查，并在交付时说明实际结果与未覆盖风险。
