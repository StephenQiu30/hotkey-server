# HotKey Backend

Python 3.12、FastAPI、Uvicorn、Pydantic 2、SQLAlchemy 2、psycopg 3、PostgreSQL、Redis、Kafka、MinIO。

采用按业务领域分组的模块化单体。项目架构、目录、API 契约和数据库事实源统一执行根目录 [PROJECT.md](../PROJECT.md)；实现门禁和验证命令执行 [AGENTS.md](../AGENTS.md#fastapi-目录与命名必须执行)。

## 执行入口

复制 `.env.example` 为未跟踪的 `.env` 并填写数据库凭据。在 `backend/` 执行 `uv sync --locked`；从 `backend/app/` 执行以下独立入口：

```bash
uv run --locked uvicorn main:create_app --factory
uv run --locked python -m worker
uv run --locked python -m cli
```

API、Worker 和 CLI 分别启动。应用启动不会创建或修改数据库结构。

浏览器登录状态维护仅适用于数据库中已存在的 `browser_state` 连接；目前尚无真实平台连接创建入口。操作者在本机设置 `HOTKEY_BROWSER_STATE_DIR` 为已存在、权限 0700 的绝对目录，捕获文件必须位于 0700 目录、权限 0600，且不得是符号链接。CLI 不接收 Cookie 正文参数，不打印捕获内容或路径：

```bash
uv run --locked python -m cli connections rotate-browser-state --owner-id OWNER_UUID --connection-id CONNECTION_UUID --expected-version 1 --capture-file /absolute/private/storage-state.json
uv run --locked python -m cli connections disable-browser-state --owner-id OWNER_UUID --connection-id CONNECTION_UUID --expected-version 2
```

停用后需导入新的人工登录状态才能重新启用；未接入平台适配器前，这些命令不代表平台采集可用。

## 数据库结构

`database/schema.sql` 是唯一 DDL 事实源；SQLAlchemy Model 只负责运行时映射。每次数据结构变更必须在同一提交中更新 SQL、Model 与真实 PostgreSQL 验证。

该文件只允许写入新建空库，并自行包含完整事务边界：

```bash
psql -X --set ON_ERROR_STOP=on \
  --dbname 'postgresql://USER:PASSWORD@HOST:5432/DATABASE' \
  --file database/schema.sql
```

当前不支持对存量数据库自动就地升级。需要保留数据时，先完成备份与恢复演练，再新建数据库、应用完整 `schema.sql` 并导入经过校验的数据；禁止对现有旧库直接执行该文件。

仓库根 `compose.yaml` 是唯一编排。复制根 `.env.example` 为未跟踪的 `.env` 并设置 URL-safe 数据库密码后，从根目录执行：

```bash
docker compose config --quiet
docker compose build
docker compose up --detach --wait
```

Compose 仅在全新 PostgreSQL 数据卷中通过官方初始化目录运行 `schema.sql`；普通停止不删除持久卷。Worker 与 CLI 是按需 profile，分别使用 `docker compose run --rm worker` 和 `docker compose run --rm cli`。验证直接使用 Compose 与各依赖官方 CLI，不建立额外脚本。

已配置的运行环境可用以下有界命令执行一批到期扫描与 Redis/MinIO 在线副本清理；它不会创建或启动新的依赖服务：

```bash
docker compose run --rm cli lifecycle cleanup-once --limit 100
```

来源凭据只由维护者写入未跟踪的 `backend/.env`（Compose 使用根 `.env`）中的 `HOTKEY_SOURCE_CREDENTIALS` JSON 映射，键仅允许 `x`、`douyin`，值为对应获授权凭据，默认 `{}`。不要把真实值放入命令参数、聊天、Git 或日志。修改后只替换现有 API/Worker 进程，再从 `/sources` 确认配置或替换；页面不提供秘密输入/回读。数据库只存不可逆指纹引用，移除或替换环境值会使原连接需重新授权。停用不删除历史资料，重新启用产生新版本并重新验证；配置完成不等于获准采集或能力可用。

来源维护者完成一次显式探测后，可在现有环境登记稳定结果：

```bash
uv run --env-file .env python -m cli connections record-probe \
  --owner-id OWNER_UUID \
  --connection-id CONNECTION_UUID \
  --connection-version 1 \
  --operation-id OPERATION_UUID \
  --capability search \
  --entry-point manual \
  --outcome succeeded \
  --component-name approved-probe \
  --component-version 1
```

命令只登记 probe 事实，不读取连接秘密、不发起外部请求，也不会将能力标为可用。`--connection-version` 必须使用探测实际执行的版本，不可在登记时改成新版本；连接停用或版本已变更会拒绝新增证据，已提交的同一事实重放仍返回原记录。失败结果必须另传 `--stop-reason`；只有后续采集用例持久业务记录后才能登记 persisted read 成功。

维护者可在现有本机环境显式指定一个已存在的受控目录，生成 PostgreSQL custom-format 候选归档、MinIO 证据对象内容和引用清单：

```bash
PYTHONPATH=app uv run --env-file .env python -m cli backup create-candidate \
  --destination /absolute/protected/backup-root
```

命令不创建服务、不修改数据库，也不把凭据写入参数或候选包。可在同一 PostgreSQL 服务中，以单独维护库的连接环境变量运行实际恢复验证；MinIO 内容会写到随机临时对象名前缀、回读校验后清理：

```bash
PYTHONPATH=app uv run --env-file .env python -m cli backup verify-restore \
  --candidate /absolute/protected/backup-root/hotkey-backup-... \
  --isolation-url-env HOTKEY_TEST_DATABASE_URL
```

隔离连接不能指向业务库；命令创建并清理唯一临时数据库，核对逐表行数和受控读写，MinIO 对象在同一 bucket 的随机前缀验证内容后清理，输出完整验证耗时。`manifest.json` 保持 `restore_verified=false`；独立介质、删除重放及 B0 RPO/RTO 演练前不能称为完整已验证备份。

业务接口统一使用 `/api` 命名空间，例如存活检查 `/api/health`、就绪检查 `/api/ready`。采集任务使用 `POST /api/jobs` 持久受理，按响应 `Location` 读取 `GET /api/jobs/{job_id}`，并以 `POST /api/jobs/{job_id}/cancel` 登记取消；详情返回持久阶段、已发请求、已保存数量及取消截止。当前没有真实采集处理器，不得把受控 Worker 验证当作来源接入。接口文档入口为 Swagger UI `/docs`、Scalar `/scalar`，共用 `/openapi.json`。

## 首个使用者与恢复

首次初始化前，在未跟踪的根 `.env` 中设置唯一且不少于 32 字符的 `HOTKEY_BOOTSTRAP_TOKEN`。受控客户端调用 `POST /api/identity/initialize` 时同时发送该值、`X-HotKey-CSRF: 1` 以及 owner 用户名和不少于 12 字符的密码。初始化完成后从运行环境移除 bootstrap 值并重新创建 API 容器；系统没有默认用户或默认密码。

登录、当前会话和注销分别使用 `POST /api/identity/sessions`、`GET /api/identity/session`、`DELETE /api/identity/session`。浏览器只使用服务端设置的会话 Cookie；非安全方法由 Web 请求层附加 CSRF 请求头，不把 Cookie 值传给业务函数。

密码遗失时由具备部署维护权限的操作者执行交互式恢复；新密码不会放入命令参数，恢复成功会撤销全部旧会话：

```bash
docker compose run --rm cli identity reset-password
```

当前只支持全新空库初始化，不得为给旧库补身份表而直接执行 `schema.sql`。

## 状态

2026-09-22 现行更新：任务 API 已有单任务读取、持久进度、取消、分类失败、有限延期、到期 Outbox、消息防重、手动重试和详情页，但尚无列表/历史及真实来源处理器。`connections` 已有连接版本/能力证据三表、幂等的 probe/persisted read 登记机制、能力状态 API、连接配置/替换/启停 API 与页面，以及显式 probe 登记 CLI；停用或当前版本认证失效拒绝新任务与人工重试，历史同事实重放仍保留。002 已提供受控 X 适配器；047 S01 已提供默认关闭、无业务调用者的 Firecrawl 网页适配器基础。两者都没有真实授权/准入连接、有效探针或平台证据。下方长段为早期底座盘点，凡与本更新冲突的任务/连接 API 描述以本更新为准；技术契约通过不代表真实来源或产品验收通过。

当前底座包含应用工厂、数据库会话、唯一 `schema.sql`、结构化日志、健康检查、Swagger UI、Scalar、Kafka Worker、管理 CLI、身份会话领域、任务持久受理/状态读取与恢复服务、任务运行上下文/阶段尝试/互斥汇总、计划到期/分阶段时间链、免费组件/尝试计量账本与分层预算窗口/预留/结算契约、来源访问政策、字段最小化门禁、纯来源能力/作品/评论/分页契约、保留/删除控制面、不可变输入清单/比较参考集/方法指纹、Redis/MinIO 在线清理适配器、同快照候选备份及架构测试。Worker 已具备 `job.accepted.v2` outbox 发布、手动 offset、inbox、租约、检查点和安全关联日志装配；没有已登记业务 kind 处理器时安全退出。生命周期、计量、来源能力、运行观察、溯源清单、时间链与候选备份当前仍只有内部 Service/DTO/Protocol 或维护 CLI。没有已启用的真实来源处理器、真实评分/模型输出、最后成功/延期/缺口/陈旧传播、独立对象备份、隔离恢复或已批准的业务配额；受控/默认关闭适配器不代表真实业务对象已接入或产品验收已通过。其余业务领域只在对应切片完成 Design 登记后创建，不保留空的未来领域包。
