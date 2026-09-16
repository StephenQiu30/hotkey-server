---
layer: Operations
scope: shared
doc_no: "006"
title: Python运行与验证
status: active
version: v1.0
---

# Python 运行与验证

本手册只描述已实现的基础工作台，不代表完整 006 产品验收。SQLAlchemy 和 RabbitMQ 固定，当前依赖精确版本以 `backend/uv.lock`、`frontend/package-lock.json` 为准。

## 运行角色与入口

唯一根 Compose 包含 postgres、rabbitmq、migrate、backend、worker、scheduler、web。migrate 成功后才启动应用；Worker 使用 prefork 子进程池，数据库连接在 fork 后建立。API 的 lifespan 负责数据库资源关闭。没有双栈 profile、旧接口代理或旧库读取。

```sh
docker compose up -d --build
docker compose exec backend python -m cli owner-init learner
```

交互输入 12–128 位密码，不通过命令参数传入。owner 只能初始化一次，数据库唯一约束阻止并发创建第二账号。打开 http://localhost:8010；当前有监控草稿、只读监控收件箱和诊断任务。B站搜索、正文、一页根评论和一页回复的持久链已实现但默认不准入；真实来源用途与现有MinIO完成验收前不会采集。多页展开、分析和报告尚未接入。

本地默认地址：Web 8010、API 8867、PostgreSQL 15435、RabbitMQ AMQP 15673，全部绑定回环地址。默认数据服务凭据只用于本机学习；不生成默认应用账号。修改 Web 端口时必须同步 `HOTKEY_ALLOWED_ORIGINS`，它是精确 HTTP Origin 的 JSON 数组，不接受通配符或 URL 路径。

API `/health/live` 只报告进程存活；`/health/ready` 检查数据库可达和迁移版本，返回 scope=database_schema，不代表消息消费者或来源健康。实际队列路径使用以下诊断验证：

```sh
docker compose exec -T backend python -m cli enqueue my-pipeline-check
docker compose exec -T backend python -m cli show <job_id>
```

重复同一幂等键返回同一个任务。任务结果在 PostgreSQL，Celery 不使用 result backend。应用角色设定 45 秒停机宽限，覆盖 20 秒任务硬期限及收尾。正常停止使用 `docker compose stop`；不要对用户持久栈执行 `down --volumes`。

## 生产覆盖实验

要求 Compose v2.24.4+（支持 `!reset`）。复制 `.env.prod.example` 为私有 `.env.prod`，替换每个密码和域名；URL 内密码需要百分号编码，必须与数据库/队列凭据一致。已有卷内账号密码不会因环境变量修改自动变化，需要单独轮换。

```sh
docker compose --env-file .env.prod -f docker-compose.yml -f docker-compose-prod.yml config --quiet
docker compose --env-file .env.prod -f docker-compose.yml -f docker-compose-prod.yml up -d --build
docker compose --env-file .env.prod -f docker-compose.yml -f docker-compose-prod.yml exec backend python -m cli owner-init learner
```

生产覆盖不发布数据库、队列和 API 端口；Web 仍只绑定本机，由外部 HTTPS 反向代理访问。生产配置强制 Secure Cookie、HTTPS 精确 Origin。TLS 终止、备份恢复、公网网关和长期容量不属于本轮本地验证结果。

## 契约、结构和依赖检查

所有命令从仓库根目录执行：

```sh
uv sync --project backend --locked
uv run --project backend ruff format --check backend
uv run --project backend ruff check backend
uv run --directory backend mypy
uv run --directory backend/src python -m tools.export_openapi --check
uv run --project backend pip-audit
npm ci --prefix frontend
npm run check:contract --prefix frontend
npm run check:boundaries --prefix frontend
npm run test:boundaries --prefix frontend
npm run format:check --prefix frontend
npm run build --prefix frontend
npm audit --prefix frontend --omit=dev --audit-level=high
```

修改 API 后执行 `uv run --directory backend/src python -m tools.export_openapi`，然后 `npm run generate --prefix frontend`，同时审查自动导出的 JSON 与生成客户端；不要手工编辑两类生成文件。FastAPI 运行时自动提供 `/docs`、`/redoc` 和 `/openapi.json`；Cookie 认证方案、CSRF Header、输入输出和稳定错误格式均包含在契约中。业务代码只调用 `frontend/src/api/` 的端点函数，Axios 仅在 `frontend/src/request.ts` 封装。

迁移位于 `backend/src/migrations/versions/`，与生成模板一起复制进镜像。运行目录为 backend/src；升级使用 `python -m cli migrate`。生成候选迁移可在 `uv run --directory backend/src python` 中调用 `alembic.command.revision(migration_config(url), autogenerate=True, ...)`，必须审查 DDL、索引和数据转换后使用。迁移拒绝非空且无 Alembic 账本的数据库；不提供破坏性降级。

## 真实数据库与消息测试

测试会清空业务表，只能在可丢弃测试库上串行执行；固定允许库名和 RabbitMQ vhost 均为 `hotkey_test`。下面命令只在首次建立独立测试资源时使用：

```sh
docker compose exec -T postgres createdb -U hotkey hotkey_test
docker compose exec -T rabbitmq rabbitmqctl add_vhost hotkey_test
docker compose exec -T rabbitmq rabbitmqctl set_permissions -p hotkey_test hotkey '.*' '.*' '.*'
export HOTKEY_TEST_DATABASE_URL='postgresql+psycopg://hotkey:learning@127.0.0.1:15435/hotkey_test'
export HOTKEY_TEST_BROKER_URL='amqp://hotkey:learning@127.0.0.1:15673/hotkey_test'
uv run --project backend pytest backend/tests -q
```

未配置环境变量时，相关集成测试明确 skip，不能据此报告完整通过。测试覆盖事务回滚、并发幂等、重复消息、过期租约与 fencing、取消、未领取消息重排、尝试上限、发布失败、废弃 Outbox、迁移与模型一致性，以及认证、CSRF、限流、过期/撤销、乐观锁、分页、请求尺寸和错误脱敏。

## 浏览器与 prefork 回归

使用独立的可丢弃 Compose project 和合成 owner，不能对真实工作台运行会写草稿的浏览器测试。设置 `COMPOSE_PROJECT_NAME` 后启动整套栈，并通过 owner-init 初始化合成账号。

```sh
uv run --project backend python backend/scripts/verify_compose.py
uv run --project backend python backend/scripts/verify_proxy_replacement.py
cd frontend
npx playwright install chromium
# 在当前进程环境中设置 HOTKEY_E2E_USERNAME 与 HOTKEY_E2E_PASSWORD，勿提交凭据。
npm run test:e2e
cd ..
uv run --project backend python backend/scripts/verify_shutdown.py
```

可选 `HOTKEY_WEB_URL` 改变测试 Web 地址，默认 localhost:8010；修改API端口时同时设置 `HOTKEY_API_URL`。Playwright 验证登录、新建/编辑草稿、活动Job与CollectionRun自动终态回显、隐藏页停轮询、终态停轮询、页面刷新保留状态、390px布局、Swagger和退出后401。截图与受控状态序列只含合成数据。CI使用一次性项目，结束时只清理其自身资源。

## 初次替换验证记录（历史）

使用独立 `hotkey-replacement` Compose project、新建测试数据库及 vhost、合成账号执行，未访问真实社交数据或旧用户数据库。

- 后端 27 项 pytest 全部通过，无跳过；真实 PostgreSQL / RabbitMQ 集成包含在内。测试依赖仍产生两条上游弃用警告（Starlette 对 httpx、AnyIO 别名），不隐去、不当作项目错误。
- Ruff 格式与 lint、严格 mypy 通过；27 项测试检查来源文件和运行行为，未以旧测试通过代替新架构验收。
- wheel/sdist 构建成功；真实镜像中的 `hotkey-backend 0.2.0` 从 site-packages 找到迁移资源，迁移服务退出码 0。空库重放及从 0002 到 0003 的升级已执行，模型/迁移比较无差异。
- OpenAPI 重新生成检查、运行时 OpenAPI 与发布 JSON 深度比较均通过；客户端生成漂移检查、Prettier、TypeScript 与 Vite 构建通过。
- 真实 scheduler → RabbitMQ → Celery prefork → 数据库诊断成功，重复键返回同一 ID，结果显示 attempts=1。
- Playwright Chromium 主流程通过，无页面脚本错误；桌面及 390px 移动布局已查看，无横向溢出。登录页 axe WCAG 2 A/AA 检查为 0 violation；这不等同于全站人工无障碍验收。
- `npm audit` 为 0 漏洞；`pip-audit` 未发现已知依赖漏洞，本地 hotkey-backend 自身不在 PyPI 审计范围内。
- 停机复测：Worker 和 scheduler 退出码 0，API 完成 lifespan 关闭后退出码 143（SIGTERM），无 SIGKILL/OOM。此回归已加入 CI；镜像为非 root 且注册了对应系统用户。
- 生产 Compose 覆盖解析通过，确认 postgres/rabbitmq/backend 不暴露宿主端口；生产 HTTPS/Secure 配置负向测试通过。
- 工作树中的 Go 源码、go.mod、go.sum 数量为 0，旧运行入口与契约已清理；正式 Markdown 的本地链接检查通过。常见私钥/访问令牌模式检查无发现，不替代完整安全审计。

页面证据：[桌面](evidence/006-workspace-desktop.png)、[移动端](evidence/006-workspace-mobile.png)。

验证结束后停止本轮测试服务，保留数据卷。尚未运行远端 CI、生产 TLS、备份恢复或七日观察；不标记完整 006 Acceptance 通过。

## 后端目录规范化复验（历史，后续已取消包布局）

工程、镜像和入口统一为 `backend/`、`hotkey-backend:local`、`main:create_app`，Compose 服务为 backend；Web 代理已指向 backend。旧 server 工程/别名、旧 api 容器入口和 Workspace 聚合服务不再使用。规则见根 AGENTS，目录职责见 [backend README](../../backend/README.md)。

本次在已有可丢弃数据库/vhost上复验，保留既有数据。38 项 pytest 全部通过、无跳过，其中 13 项为架构检查。架构检查包含分层依赖、违规样例、包初始化约束、绝对导入、文件命名、无依赖环、唯一 FastAPI 工厂和部署入口。Ruff 和严格 mypy 同时检查应用与独立验证脚本；迁移文件和发布 OpenAPI 与重构前逐字节相同。

真实 prefork 诊断完成且 attempts=1；Playwright Chromium 主流程通过，运行时 OpenAPI 与发布契约一致。镜像安装的 hotkey-backend 包及迁移资源可用。停止测试返回 worker=0、scheduler=0、backend=143（完成正常 SIGTERM 收尾），没有 SIGKILL。

生产覆盖配置解析通过。以上是本地重构验证，远端 CI 和真实来源能力仍未验收。本次修改尚未提交。

## 当前扁平目录复验

当前目录规范以用户最新澄清为准：应用源码位于 backend/src，Worker 与 CLI 分别为 src/worker、src/cli，不增加 hotkey 包装层。运行入口为 main:create_app、worker.app:app、python -m cli；容器工作目录为 /app/src。此前直接放在 backend 根目录的结构已撤销，历史验证记录不能代替本轮复验。

本轮 src 目录调整复验（2026-09-08）：40 项 pytest 全部通过，无跳过，包含真实 PostgreSQL/RabbitMQ；Ruff、严格 mypy（54 个源文件）、OpenAPI 漂移检查、前端契约检查与 TypeScript/Vite 构建通过。Compose prefork 诊断任务 succeeded，attempts=1；Chromium 登录、监控编辑、诊断和退出流程通过。容器工作目录 /app/src，worker/app.py、cli/__main__.py 与迁移目录均存在；原迁移文件和发布 OpenAPI 的 SHA-256 未变，HTTP 运行契约也与发布 JSON 相同。

正常停机验证通过：worker/scheduler 退出码 0，backend 接收 SIGTERM 后退出码 143，均无 SIGKILL 或 OOM。已停止本轮测试容器并保留持久卷。测试有两条上游弃用提示，不影响结果；本轮没有提交或推送。


## S01 来源探测验证（2026-09-08）

先提交替换基线到本地 main：92569a49；未推送。随后来源切片新增 16 项来源单元测试和 1 项认证查询预览集成测试，另增加 2 项来源依赖边界检查，总计 59 项 pytest 通过（含真实 PostgreSQL/RabbitMQ，无跳过）。Ruff、严格 mypy 60 个文件、生成 OpenAPI、前端契约与构建通过。两个上游弃用提示保持可见。

真实探测摘要见 [source-probe-summary.json](evidence/source-probe-summary.json)。search_posts 请求为 science、UTC 最近两天、limit=2，返回 HTTP 403/access_denied，CLI 退出 1。thread 从官方 bsky.app 公开作者流的首个帖子取得 URI，以 depth=1/max_nodes=20 调用，HTTP 200；返回体 289638 字节，输出 20 个节点，partial/node_limit，CLI 退出 0。coverage 始终 unknown；退出 0 不表示全量完成。记录只保留元数据，没有提交来源正文、作者标识或会话信息。

使用方式见 backend/README.md。搜索失败未做自动重试或登录绕过；此切片不需要业务数据库/broker 凭据。没有新增迁移或将结果写入任务账本；国内来源、持续搜索准入、原始证据持久化和任务执行隔离仍待下一片验证。

来源切片容器回归：实际 RabbitMQ/Celery prefork 诊断 succeeded、attempts=1；Chromium 登录、监控编辑、诊断与会话撤销通过。正常停机 worker/scheduler=0、backend=143，无 SIGKILL/OOM。测试容器已停止，持久卷保留。现有 HTTP 路径契约不变，仅新增 query-preview，迁移文件无改动。来源切片尚未提交，92569a49 基线仍仅在本地 main。

## 007 S00 目录、Swagger与生成客户端验收（2026-09-15）

S00 在先登记文件归属和技术选择后执行。来源 I/O 移到 `backend/src/sources/adapters/`；前端最终收敛为 `app/features/api` 加根级 `request.ts`，不设置shared层。后端架构检查递归枚举整个 `backend/src`，前端脚本拒绝未知层级、反向依赖、越界导入和旧文件副本；对应违规样例先失败后转绿。

FastAPI 从路由和 Pydantic 模型运行时生成 `/openapi.json`、`/docs` 与 `/redoc`，14 个操作使用稳定 operationId。`tools.export_openapi` 只做确定性快照，运行时 JSON 与 `docs/openapi/openapi.json` 深度相等。`@umijs/openapi` 1.14.1 按 tag 生成 `frontend/src/api/` 的端点函数和DTO；业务组件只引用这些方法。根级 `request.ts` 使用 Axios 1.20.0 处理同源Cookie、XSRF、重复查询参数和错误体，不维护业务URL或DTO。临时重新生成与仓库文件逐字节比较通过。

本地静态检查结果：Ruff format/lint、严格 mypy（61个源文件）、OpenAPI漂移通过；一次性 `hotkey_test` PostgreSQL/RabbitMQ环境中64项pytest全部通过，保留两条上游弃用提示。前端生成、契约漂移、边界检查及负向样例、Prettier、TypeScript/Vite构建通过；运行时依赖 `npm audit --omit=dev --audit-level=high` 为0。

隔离项目 `hotkey-s00` 完成镜像构建、迁移、真实 scheduler → RabbitMQ → Celery prefork → PostgreSQL 诊断，结果succeeded/attempts=1。Chromium两项通过：Swagger UI实际渲染并读到稳定operationId；owner登录、新建/编辑草稿、Axios提交诊断、刷新恢复、390px无横向溢出、退出和会话撤销通过。人工查看1440px与390px截图未发现目录迁移造成的视觉回归。停机结果worker=0、scheduler=0、backend=143，无SIGKILL/OOM；随后只删除本次隔离项目及其卷。

开发工具依赖仍有2个high审计条目，均源于用户指定的 `@umijs/openapi` 间接依赖 `mockjs` 的同一原型污染公告，当前无可用修复。生成器不进入Nginx生产镜像运行阶段，只读取仓库内由FastAPI自动导出的可信契约；CI继续严格审计运行时依赖，并保留升级/移除该间接依赖的跟踪。以上完成AC-007-016与S00，不代表来源采集、MinIO证据链或完整007验收通过。

## 007 S02-T02A 页事务与收件箱基础验证（2026-09-15）

本片在MinIO连接参数和来源用途准入尚未具备时，只实现可独立验证的事务内核。`collection` 负责运行、fencing和页检查点，`evidence` 负责确定性gzip、对象协议及raw page元数据，`contents` 负责规范身份、不可变正文版本、观察值和只读收件箱，`monitors` 负责版本级内容命中。API增加认证只读 `GET /api/contents`；FastAPI自动生成Swagger/OpenAPI，UmiOpenAPI在 `frontend/src/api/contents.ts` 生成调用，页面仍只经根级Axios `request.ts` 发请求。

真实PostgreSQL 16和RabbitMQ测试环境中76项pytest通过，无跳过，保留两条上游弃用提示；迁移/模型比较为空。额外的一次性数据库从 `0004_monitor_versions` 升至 `0005_collection_page`，已有monitor version保留且5张新增核心表存在。隔离 `hotkey-s02a` Compose从迁移服务到0005，真实scheduler、RabbitMQ、Celery prefork和数据库诊断成功，attempts=1。运行时OpenAPI与发布JSON相等。生成客户端漂移、前端边界/负向样例、Prettier、TypeScript/Vite和开发/生产Compose解析通过。

首次最终镜像重建时，Web未被Compose替换而backend获得新IP，静态解析的Nginx upstream继续连接旧IP并使登录返回502。`frontend/nginx.conf` 改为通过Docker内置DNS在请求期解析backend；Nginx配置检查通过。`verify_proxy_replacement.py` 已接入CI：它保持Web容器ID不变、强制替换backend，在后端ready后要求经Web访问认证端点返回401。随后Chromium共2项通过：Swagger UI渲染自动契约；合成owner登录后经生成接口读取合法空收件箱，随后完成监控草稿和真实诊断流程。1440px和390px截图人工检查未发现横向溢出或布局遮挡。本片结构化证据见 [collection-page-foundation-poc.json](evidence/collection-page-foundation-poc.json)。测试内 `MemoryStore` 只验证对象协议、hash和上传先于数据库提交的边界；没有连接MinIO，没有访问外部平台，也没有把合成内容作为真实采集结果，因此不构成EV-007-003或S02-T02完成证据。

## 007 S02-T02B MinIO适配器与隔离协议验证（2026-09-15）

本片锁定官方 `minio-py 7.2.20`，将SDK限制在 `evidence/adapters/minio.py`。应用配置新增 `HOTKEY_S3_ENDPOINT/ACCESS_KEY/SECRET_KEY/BUCKET/SECURE`：四项连接值必须同时存在，密钥使用 `SecretStr`，endpoint明确使用 `host[:port]` 及独立TLS开关。根Compose只透传外部配置，没有增加MinIO服务或持久卷。对象已存在时只有大小和SHA-256都相同才视为幂等；冲突拒绝覆盖。新上传通过stat和完整读回验收，不使用ETag代替内容hash。

独立 `verify_minio.py` 只允许使用预先存在的bucket，写入一条合成JSON的确定性gzip对象。一次性 `minio/minio:RELEASE.2025-09-07T16-13-09Z` 容器中，首次上传与第二次幂等重投均成功，65字节对象SHA-256为 `7bedd8fc37c8cfb0b6c1457723d062fd393c2419ec8e9ff62afc7f709c5edbae`；读回校验通过，精确删除后bucket对象数为0，随后删除容器。25项配置/适配器/架构聚焦测试和真实PostgreSQL 16、RabbitMQ 4.1下82项全量后端测试通过，保留2条已知上游弃用提示；Ruff和严格mypy（81个源文件）通过。`pip-audit` 无已知漏洞；OpenAPI与UmiOpenAPI客户端无漂移，前端边界、格式、TypeScript/Vite和生产依赖审计通过。隔离Compose成功安装MinIO SDK并完成真实prefork诊断，attempts=1；backend原地替换后代理返回预期401，Chromium的Swagger UI与owner工作台2项通过。正常停机worker=0、backend=143、scheduler=0，无SIGKILL或OOM；隔离Compose及数据卷随后删除。结构化证据见 [minio-adapter-poc.json](evidence/minio-adapter-poc.json)。

该容器仅验证客户端和对象协议，不是用户选择复用的现有MinIO。尚未取得现有实例的非敏感endpoint、TLS路径、bucket、最小权限凭据注入和服务端版本，因此未测试真实网络、权限、生命周期或孤儿对账，也不构成EV-007-003、真实采集或S02-T02完成证据。

## 007 S02-T02C 采集任务与消费者基础验证（2026-09-15）

本片将采集运行接入既有Job/Outbox/RabbitMQ/Celery账本。任务消息仍只含job_id、epoch和契约版本，白名单kind增加`collect_page`；Worker由租约kind选择诊断或单页采集执行器，采集结果只写collection_run及页事务，不借用短字符串JobResult。run、Job和Outbox同事务创建，执行前再次检查来源准入；来源撤权测试没有调用来源适配器。任务硬超时为60秒，租约必须严格大于硬超时且默认90秒，防止正常任务在提交前自行过租。

新增`0006_collection_jobs`迁移。一次性数据库先升级至0005并写入一条queued和一条completed历史运行，再升级至0006：queued运行以`migration_boundary`终止并关联failed collect_page Job；completed/ok运行保持结果并关联succeeded Job；两者保留期均回填7天。全量迁移与SQLAlchemy模型比较无差异，readiness revision与Alembic唯一head增加静态一致性门禁。

FastAPI自动发布`POST /api/monitors/{id}/runs`和`GET /api/collection-runs/{id}`。创建响应为HTTP 201，因为run与Job在响应前已持久化；异步进度由资源状态表达。此前为生成器改写202响应的hook已删除，`@umijs/openapi`直接生成`frontend/src/api/collection.ts`，业务请求仍只经根级Axios `request.ts`。运行时OpenAPI与发布快照相等。

真实PostgreSQL 16和RabbitMQ 4.1环境中92项pytest通过，保留2条上游弃用提示；其中覆盖页提交后Worker崩溃的重领收口，确认不会二次抓取；也覆盖幂等重放在后续来源撤权和对象存储配置缺失时仍返回既有运行，以及8个并发同键请求只生成一组run/Job/Outbox。Ruff、严格mypy（82个源文件）、OpenAPI/UmiOpenAPI漂移、前端边界与负向样例、Prettier和TypeScript/Vite构建通过。隔离`hotkey-s02c` Compose完成迁移和真实scheduler → RabbitMQ → Celery prefork → PostgreSQL诊断，attempts=1；Web保持运行、替换backend后代理返回预期401。Chromium 2项通过，Swagger UI实际加载并核对两项collection operationId，owner工作台完成登录、草稿、查询预览、诊断、刷新、390px及退出流程。正常停机为worker=0、backend=143、scheduler=0，无SIGKILL/OOM；隔离资源随后删除。结构化证据见 [collection-job-foundation-poc.json](evidence/collection-job-foundation-poc.json)。

本片使用注入的合成原始页和内存EvidenceStore验证消费者编排，没有访问外部来源，也没有连接用户现有MinIO。所有来源仍为not_connected且不具备产品采集准入；预算预留、按slot调度、分页、详情展开和评论任务仍待后续切片。因此本记录不构成EV-007-003、真实收件箱或TASK-007-S02-T02完成证据。

## 007 S02-T02D 日预算、周期槽与运行列表验证（2026-09-15）

本片增加 `0007_collection_budget`：每个单页运行在创建Job前，按monitor version与UTC日期原子预留1次请求；run、Job、Outbox和预算占用同事务提交。预留不会因失败自动退还，避免失败重试绕过上限。8个不同幂等键并发争抢1次预算时，1个成功，7个稳定返回`request_budget_exhausted`，数据库最终各有1条run、Job和Outbox。

scheduler只读取monitor模块提供的当前active配置DTO，按UTC周期边界生成最近一个已结束窗口。同槽重扫不重复创建，暂停后不派下一槽；来源准入失败或 `HOTKEY_S3_*` 不完整时保持零写入，不尝试联网，也不追补停机期间无限历史。运行列表采用 `(created_at,id)` 不透明游标，GET只读取状态。前端只调用UmiOpenAPI生成的 `listCollectionRuns`，展示触发类型、预算日、时间窗口、页/内容/字节和停止原因；保留天数进入不可变monitor schedule快照，不保留前端默认值兼容分支。

一次性数据库先迁移到0006并写入同一UTC日的两条历史运行，再升级到0007。两条运行均回填manual、空schedule slot、每条1次预留及UTC预算日，旧schedule结构回填7天保留期。旧预算快照为1但历史已有2次请求时，新账本以既有事实为下限记录limit=2、reserved=2，没有删除运行或制造负额度。全量迁移和SQLAlchemy模型比较无差异。

真实PostgreSQL 16与RabbitMQ 4.1环境中97项pytest通过，保留2条上游弃用提示；Ruff和严格mypy（84个源文件）通过。`pip-audit`无已知漏洞，npm生产依赖审计为0。运行时OpenAPI与仓库快照相等；UmiOpenAPI漂移、前端目录/负向边界、Prettier和TypeScript/Vite构建通过，生产Compose覆盖解析成功。

隔离 `hotkey-s02d` Compose完成0007迁移、真实scheduler → RabbitMQ → Celery prefork → PostgreSQL诊断，attempts=1。未配置S3且无已准入来源时，collection_runs数量保持0。Web容器不替换、只重建backend后，经Web访问认证端点得到预期401。Chromium 2项通过：Swagger UI实际显示 `listCollectionRuns`；合成owner工作台在1440px和390px显示运行区、保留天数和合法空态，无横向溢出或页面脚本错误。正常停机worker=0、scheduler=0、backend=143，无SIGKILL/OOM；隔离Compose、迁移数据库及测试容器均已删除。结构化证据见 [collection-schedule-budget-poc.json](evidence/collection-schedule-budget-poc.json)。

本片没有连接用户现有MinIO，没有向任何平台发送请求，也没有产生真实帖子、评论或收件箱内容。生产来源仍全部not_connected；因此不构成EV-007-003、TASK-007-S02-T02或完整MVP验收。

## 007 S02-T02E 精确来源操作准入验证（2026-09-15）

来源准入从平台级状态收敛为精确 `source.operation`。FastAPI能力卡逐操作返回技术support、用途rights、pipeline和eligible，平台对象不再保留可造成误放行的汇总字段。只有三项门禁同时满足才eligible；monitor启用明确检查search_posts。API在lifespan构造共享SourceService，scheduler与Worker从同一Settings构造。生产Compose为所有应用角色透传完整MinIO配置及 `HOTKEY_SOURCE_RIGHTS_ALLOWED`、`HOTKEY_SOURCE_PIPELINES_CONNECTED`，默认均为空。查询预览从同一能力卡计算pipeline_connected。

当前持久消费者只实现 `bilibili.search_posts`。使用合成配置把尚未实现的 `bilibili.list_comments` 标为connected时，最终镜像在API启动阶段以退出码3失败并给出稳定领域错误；未知键也有启动测试覆盖。默认Compose实际返回6个平台、24个操作、0项eligible，未创建真实采集运行。该验证没有连接现有MinIO、没有使用平台账号或授权会话，也没有访问外部平台。

真实PostgreSQL 16与RabbitMQ 4.1环境中102项pytest通过，保留2条上游弃用提示；Ruff和严格mypy（84个源文件）通过。`pip-audit`无已知漏洞，npm生产依赖审计为0。运行时OpenAPI与仓库快照一致；UmiOpenAPI仅生成 `frontend/src/api`，前端契约漂移、目录/负向边界、Prettier和TypeScript/Vite构建通过；生产Compose覆盖确认migrate/backend/worker/scheduler均收到来源与MinIO配置。

隔离 `hotkey-s02e` Compose完成0007迁移、真实scheduler → RabbitMQ → Celery prefork → PostgreSQL诊断，结果succeeded/attempts=1。Web不替换、只重建backend后代理返回预期401。Chromium 2项通过：Swagger UI渲染自动契约，owner工作台在默认零准入状态展示逐操作“权限待核对 · 未连接”，并完成查询预览、草稿、诊断、390px检查和会话撤销。正常停机worker=0、backend=143、scheduler=0，无SIGKILL/OOM；全部一次性Compose和测试容器及卷已删除。结构化证据见 [source-operation-admission-poc.json](evidence/source-operation-admission-poc.json)。

这一步关闭的是配置表达、跨进程一致性和误配置失败边界。它不证明用户现有MinIO可用，也不证明B站或其他平台已获实际用途授权；因此不构成EV-007-003、真实收件箱、评论链或TASK-007-S02-T02完成证据。

## 007 S02-T02F 搜索引用到正文验证（2026-09-15）

B站搜索页现在只负责发现引用，最多选择一个合法bvid；搜索页数据库提交会在同一事务内为详情预留一次UTC日预算，并创建 `fetch_post` 子run、Job和Outbox。搜索和详情各自保存原始页，详情规范化为post后再执行关键词匹配与收件箱写入。父子关系、允许操作和同一父任务下的目标唯一性由0008数据库约束保证；周期唯一索引只覆盖parent为空的根搜索，因此同周期不同关键词发现同一帖子时不会让第二个父页回滚。同页重投先命中checkpoint，不重复创建详情任务。

页边界重新检查当前monitor version是否active和fetch_post是否仍eligible。预算不足、暂停或详情撤权分别留下 `detail_budget_exhausted`、`monitor_inactive` 或 `detail_not_eligible`，不会继续调用详情。B站search能力声明依赖fetch_post，所以只配置搜索操作也不能启用monitor。新监控默认日预算为48，可覆盖每小时一个关键词的搜索加一个详情上限；历史快照不改写。

从 `0007_collection_budget` 到 `0008_reference_expansion` 的独立升级POC先写入 `query_variant=AI-migration-value` 的历史search运行，再升级并确认 `request_value` 原值保留、parent为空、旧列已删除。真实PostgreSQL 16和RabbitMQ 4.1下107项pytest通过；Ruff与严格mypy（85个源文件）、依赖审计、FastAPI OpenAPI快照、UmiOpenAPI漂移、前端目录/负向边界、Prettier和TypeScript/Vite构建均通过。

隔离 `hotkey-s02f` Compose迁移到0008并完成真实scheduler/RabbitMQ/Celery prefork诊断，attempts=1；运行时OpenAPI等于发布快照。后端容器替换时Web保持运行，代理认证端点返回401。Chromium 2项在文档端口通过，覆盖Swagger自动契约、默认零准入工作台和48次预算默认值。默认运行结果为6个平台、24个操作、0项eligible和0个collection run。正常停机worker=0、backend=143、scheduler=0，无SIGKILL/OOM；全部一次性资源已删除。结构化证据见 [search-reference-expansion-poc.json](evidence/search-reference-expansion-poc.json)。

父子内容测试使用合成平台响应和内存EvidenceStore，没有访问外部平台或连接用户现有MinIO，也没有取得真实用途确认。它证明任务、预算、持久化与生成契约的工程闭环，不构成EV-007-003、真实平台收件箱、评论链或TASK-007-S02-T02完成证据。

## 007 S03-T01A 正文到首屏根评论验证（2026-09-15）

B站详情在平台明确返回评论数大于0时生成一个 `aid:<id>` 引用；详情页提交在同一数据库事务内预留一次预算并创建 `list_comments` 子run、Job和Outbox。评论消费者固定请求cursor=0、limit=20，只提交这一页原始JSON证据；响应仍有下一页时以partial/page_limit结束，不自动遍历。0评论正文不创建评论任务，非法aid在网络前拒绝。

来源依赖现在是search_posts → fetch_post → list_comments的传递链，三项必须各自满足support、rights和pipeline。查询预览与启用预算按每个关键词每周期最多3次请求估算，新监控默认每小时单关键词为72次/日；历史monitor version快照不改写。页边界继续检查当前monitor version、精确操作准入和UTC日预算，停止原因区分详情与评论阶段。

根评论以同来源post的外部身份解析关系。评论正文即使不含关键词，只要根帖已命中当前monitor version，就以 `root_context` 继承上下文并进入收件箱；跨来源或缺失根帖不会继承。运行列表明确区分搜索、正文和根评论。API路径未增加，FastAPI自动契约将运行操作扩为三种，`@umijs/openapi` 只更新 `frontend/src/api/typings.d.ts`，业务请求仍只经根级Axios `request.ts`。

Red阶段在0008基线上把 `bilibili.list_comments` 声明为connected，服务以“persistent source operation is not implemented”失败。Green阶段在真实PostgreSQL 16和RabbitMQ 4.1下111项pytest全部通过，保留2条上游弃用提示；Ruff与严格mypy（86个源文件）、OpenAPI/UmiOpenAPI漂移、前端目录与负向边界、Prettier、TypeScript/Vite、生产Compose解析均通过。`pip-audit`未发现已知漏洞，npm生产依赖审计为0。

独立迁移POC先把数据库停在0008，写入search_posts根运行及fetch_post子运行，再升级到 `0009_root_comments`；两条运行的operation、request_value和父子关系原样保留，Alembic与SQLAlchemy模型比较无差异。隔离 `hotkey-s03a` Compose迁移到0009，真实scheduler/RabbitMQ/Celery prefork诊断succeeded/attempts=1；运行时OpenAPI与发布快照完全相同。Chromium 2项通过，覆盖Swagger自动契约、默认零准入工作台和72次预算默认值。Web不替换、backend原地替换后代理返回401。正常停机worker=0、backend=143、scheduler=0，无SIGKILL/OOM；一次性容器、卷与迁移数据库均已删除。结构化证据见 [root-comment-page-poc.json](evidence/root-comment-page-poc.json)。

父子评论链使用合成平台响应与内存EvidenceStore，没有访问外部平台、连接用户现有MinIO或取得真实用途确认。默认生产配置仍是0项eligible和0条采集运行；该结果只证明一页根评论的工程链路，不构成EV-007-003、真实平台收件箱、完整评论树或TASK-007-S03-T01完成证据。

## 007 S03-T01B 根评论到首屏回复验证（2026-09-15）

根评论页只选择一个平台明确标记存在回复的根评论，并生成精确 `aid:<正整数>/root:<正整数>` 引用。`list_replies` 消费者固定请求第1页、limit=20；响应仍有后页时以partial/page_limit结束，不遍历第二页或第二个根评论。非法、Unicode数字或缺少根评论ID的引用均在网络请求前拒绝。根评论页提交与回复子run、Job、Outbox和一次UTC日预算预留保持同一事务。

来源依赖扩展为search_posts → fetch_post → list_comments → list_replies，四项操作必须分别通过技术support、用途rights和pipeline门禁。查询预览、启用校验和调度预算按每个关键词每周期最多4次请求计算；新监控每小时单关键词默认96次/日，历史快照不改写。回复只有在同来源根评论关系唯一解析且根帖已命中当前monitor version时才以 `root_context` 继承上下文；缺失父评论或关系不唯一时保持unresolved且不写入匹配收件箱。

Red阶段把 `bilibili.list_replies` 配置为connected，持久来源操作门禁以“persistent source operation is not implemented”失败。Green阶段使用合成四段平台响应和内存EvidenceStore验证搜索、正文、根评论、回复各自保存原始页，且任务、预算、父子关系和上下文继承完整。真实PostgreSQL 16与RabbitMQ 4.1环境中114项pytest通过，保留2条上游弃用提示；Ruff和严格mypy（91个应用与验证源文件）、FastAPI OpenAPI快照、UmiOpenAPI漂移、前端目录与负向边界、Prettier、TypeScript/Vite、生产Compose解析均通过。`pip-audit`未发现已知漏洞，npm生产依赖审计为0。

独立迁移POC先把数据库停在0009并写入search_posts、fetch_post和list_comments三层历史运行，再升级到 `0010_reply_page`；三条运行的operation、request_value和父子关系均保留，Alembic与SQLAlchemy模型比较无差异。隔离 `hotkey-s03b-stack` Compose迁移到0010，真实scheduler → RabbitMQ → Celery prefork → PostgreSQL诊断succeeded/attempts=1，空白业务库保持0条collection run；运行时OpenAPI与发布快照完全相同。Chromium 2项通过，覆盖Swagger自动契约、合成owner工作台和96次预算默认值。Web不替换、backend原地替换后代理返回401。正常停机worker=0、backend=143、scheduler=0，无SIGKILL或OOM。结构化证据见 [reply-page-poc.json](evidence/reply-page-poc.json)。

本片没有访问外部平台、连接用户现有MinIO或取得真实用途确认。默认生产配置仍是0项eligible；只展开一个根评论的一页回复，后续分页及其他平台仍未实现。它证明首屏回复工程链路，不构成EV-007-003、真实平台收件箱、完整评论树、事件归档或TASK-007-S03-T01完成证据。

## 007 S04-T01A 人工事件档案验证（2026-09-15）

本片新增events当前档案、event_members当前归属与event_revisions不可变快照。创建事件写入revision 1；加入和移出成员分别递增revision并保存变化后的成员ID清单。每条内容当前最多属于一个事件：服务锁定内容行后检查归属，数据库对content_id设置唯一约束；同事件重复加入幂等，跨事件加入返回content_already_assigned且不改写修订。事件接口均要求现有owner会话与CSRF。

FastAPI自动生成创建/列表/详情、加入/移出成员和修订读取6个端点，`@umijs/openapi` 只在 `frontend/src/api` 生成events.ts、index.ts和类型，Axios仍只位于根级request.ts。工作台事件区可创建档案并查看修订/成员数，收件箱使用生成端点选择事件，成员可移出；没有手写业务URL或DTO。

Red阶段因events模块不存在而在测试收集阶段失败。Green阶段真实PostgreSQL 16与RabbitMQ 4.1下116项pytest通过，保留2条上游弃用提示；Ruff、严格mypy（97个应用与验证源文件）、OpenAPI/UmiOpenAPI漂移、前端目录与负向边界、Prettier及TypeScript/Vite构建通过。独立迁移POC从0010升级到 `0011_event_dossiers`，保留历史内容 `video:migration`，新增三表且SQLAlchemy模型比较无差异。

隔离 `hotkey-s04a-stack` Compose首次拉取基础镜像token遇到一次EOF，未运行项目代码；同一锁定镜像重试后完成构建与0011迁移。真实scheduler/RabbitMQ/Celery prefork诊断succeeded/attempts=1。Chromium 2项通过，Swagger读取到事件端点，合成owner在工作台创建事件、刷新恢复，并完成既有监控与诊断流程。运行时OpenAPI与发布快照相等；Web保持运行而backend替换后代理返回401。正常停机worker=0、backend=143、scheduler=0，无SIGKILL/OOM。结构化证据见 [event-dossier-poc.json](evidence/event-dossier-poc.json)。

本片使用合成账号、合成事件和测试内容，没有连接外部平台或现有MinIO。它只完成事件人工整理基础；合并、拆分、分析过期、趋势和提醒仍未实现，因此不构成EV-007-006或TASK-007-S04-T01完成证据。

## 007 S04-T01B 事件合并拆分验证（2026-09-16）

合并请求同时携带目标和源事件期望修订，服务按UUID顺序锁定两行后再校验状态与修订；所有源成员移动到目标，源事件归档，双方各增加一条关联修订。拆分请求携带源事件期望修订和唯一成员ID清单，只允许当前成员的非空真子集；选中成员移动到同事务创建的新事件，原事件写split_out，新事件首条修订为split_in。event_members的content_id唯一约束继续保证当前单一归属，总成员数不因合并拆分改变。

Red阶段因EventMergeInput/EventSplitInput不存在而在测试收集失败。Green聚焦测试在真实PostgreSQL上验证三成员合并再拆分、双边related_event_id、源归档、快照、旧修订409和零部分写入。0011→ `0012_event_merge_split` 迁移保留既有create修订，related_event_id回填为空，Alembic与SQLAlchemy模型比较无差异。FastAPI自动增加mergeEvent和splitEvent，UmiOpenAPI更新生成客户端；工作台只调用生成函数。

隔离 `hotkey-s04b-stack` Compose完成0012迁移与真实scheduler/RabbitMQ/Celery prefork诊断，succeeded/attempts=1。Chromium 2项通过：Swagger读取合并拆分端点，合成owner创建两个事件并在UI执行合并，数据库为1个active、1个archived，修订为2条create及各1条merge_in/merge_out。运行时OpenAPI等于发布快照；Web不替换、backend替换后代理返回401；关停worker=0、backend=143、scheduler=0，无SIGKILL/OOM。结构化证据见 [event-merge-split-poc.json](evidence/event-merge-split-poc.json)。

拆分的浏览器交互因隔离栈没有合成收件箱内容，只由真实PostgreSQL服务测试覆盖。远程CI已复跑完整提交候选：117项后端测试、2项Playwright、Compose、代理替换和无SIGKILL停机检查通过，见 [GitHub Actions #34991217129](https://github.com/StephenQiu30/hotkey-server/actions/runs/34991217129)。趋势、提醒和分析失效仍未实现，因此本片推进AC-007-008但不构成完整EV-007-006或TASK-007-S04-T01完成证据。

## 007 S04-T01C 事件变化站内提醒验证（2026-09-16）

本片将事件加入/移出成员、合并和拆分的每条非create修订映射为站内提醒。修订与提醒使用同一数据库事务；`rule_version + event_id + change_id`唯一约束让同一变化重放不产生重复提醒。列表使用URL安全的不透明游标并返回全局未读数，标记已读以行锁和首次变化审计保持幂等。

Red阶段因notifications模块不存在而在测试收集失败。Green阶段真实PostgreSQL 16与RabbitMQ 4.1全量127项pytest通过；Ruff、严格mypy（105个应用与迁移源文件）、FastAPI OpenAPI快照、UmiOpenAPI漂移、前端边界、Prettier及TypeScript/Vite构建通过。独立迁移POC从0012升级到`0013_event_notifications`，保留既有事件，新增空notifications表，Alembic与SQLAlchemy模型比较无差异。

隔离`hotkey-s04c-stack`完成0013迁移和真实scheduler/RabbitMQ/Celery prefork诊断，succeeded/attempts=1。Chromium 2项通过：Swagger读取提醒端点；合成owner合并两个事件后看到2条未读提醒，标记1条后刷新仍为1条未读。运行时OpenAPI与发布快照相同；Web保持运行而backend替换后代理返回401；关停worker=0、backend=143、scheduler=0，无SIGKILL或OOM。首次浏览器运行使用未列入允许来源的测试端口，Origin门禁按设计拒绝登录；改用已登记的8010后通过。远端CI在锁定依赖与全新容器中复现完整门禁，见 [GitHub Actions #34995710640](https://github.com/StephenQiu30/hotkey-server/actions/runs/34995710640)。结构化证据见 [event-notification-poc.json](evidence/event-notification-poc.json)。

本片只验证合成事件变化，没有访问外部平台或连接现有MinIO。提醒由明确的人工事件变化触发，不代表趋势检测；趋势时间桶、来源覆盖中断、回填抑制、突发规则和分析失效仍未实现，因此不单独满足AC-007-010或完整TASK-007-S04-T01。

## 007 S03-T02B 用户取消与页提交屏障验证（2026-09-16）

`collect_page`现在通过组合根注入的领域处理器取消。处理器按CollectionRun→Job顺序加锁，在同一PostgreSQL事务把仍在排队或运行的两者结算为cancelled，并记录`partial/user_cancelled`；诊断任务继续只走通用Job取消。网络请求返回后和对象上传后均重新检查运行状态与fencing，取消或陈旧租约不能创建RawPage、Content、Checkpoint或后续run。已上传但被第二道屏障拒绝的精确对象立即删除；删除失败只登记一条`failed` RawPage，既有有界证据清理器可重试。

Red阶段3项用例因采集取消处理器不存在失败，旧fencing用例确认对象会遗留；新增HTTP用例第一次还因测试预算不足在准入阶段失败，修正fixture后不把它作为产品故障。Green/Refactor后5项聚焦测试和170项完整后端测试通过，保留2条上游弃用提示；Ruff覆盖158文件，严格mypy覆盖131个源/脚本文件。`pip-audit`无已知漏洞，npm生产依赖审计为0；FastAPI OpenAPI快照与UmiOpenAPI生成客户端无漂移，前端边界、负向样例、Prettier与TypeScript/Vite构建通过。

隔离`hotkey-s03d` Compose完成0019迁移，真实scheduler/RabbitMQ/Celery prefork诊断为succeeded/attempts=1；Web不替换而backend替换后代理返回401。正确初始化合成owner后Chromium 3项全部通过；首次浏览器运行因验证命令在frontend目录调用不存在的`python`而没有创建owner，Swagger通过而两项登录流程失败，改用仓库根目录的`python3`与显式Compose项目后原样重跑通过。PostgreSQL 16.15 custom dump为90426字节，恢复后删除清单重放两次保持幂等。正常停机worker=0、backend=143、scheduler=0且无SIGKILL，全部容器与卷已删除。功能提交`ff514541ce3204eac8091eff1dc92806590313c6`的[远端CI](https://github.com/StephenQiu30/hotkey-server/actions/runs/35044168135)在3分05秒内复现170项后端、Chromium 3项、prefork/代理/恢复/停机门禁并成功，远端0019 dump为90413字节。结构化证据见[collection-cancellation-poc.json](evidence/collection-cancellation-poc.json)。

验证使用合成来源响应和内存EvidenceStore，没有访问外部平台或用户现有MinIO。它证明应用取消、数据库账本和对象补偿协议，不证明真实平台在途请求中断、现有MinIO权限/TLS/版本行为、多页评论或七日稳定性；EV-007-005与TASK-007-S03-T02继续保持部分完成。

## 007 S03-T02C 页面与Job终态原子结算验证（2026-09-16）

`collect_page` 现在以完整Lease领取CollectionRun，在写入RawPage、内容、Checkpoint和run终态的同一PostgreSQL事务结算Job与Attempt。Worker不再在事务外第二次complete；提交后重投的同epoch消息无法重新领取终态Job，同一Lease的重复页只返回已有Checkpoint。

Red阶段3项聚焦断言失败：成功页提交后Job仍为running，暂停监控后执行器仍返回可完成，重投无法按终态收口。Green/Refactor后30项采集聚焦测试和170项全量后端测试通过，保留2条上游弃用提示；Ruff覆盖158文件，严格mypy覆盖131个源/验证文件。`pip-audit`无已知漏洞，npm生产依赖审计为0；FastAPI OpenAPI快照、UmiOpenAPI客户端和HTTP契约未变更且无漂移。

全新隔离`hotkey-s03f` Compose迁移到0019，PostgreSQL 16.15、RabbitMQ 4.1.8的scheduler → RabbitMQ → Celery prefork诊断为succeeded/attempts=1；Web不替换而backend替换后代理返回401。合成owner的3项Chromium测试全部通过，覆盖Swagger自动文档、知识不可用态和主工作台；首次复跑复用了上一次合成事件数据，空态断言按设计失败，清理一次性卷后原样通过。关停worker=0、backend=143、scheduler=0且无SIGKILL，所有`hotkey-s03f`容器与卷已删除。功能提交`134d8facc5cf8e1018cdeb70f785a9d0a900c498`的[远程CI](https://github.com/StephenQiu30/hotkey-server/actions/runs/35047820313)在4分09秒内复现全部门禁并成功，远端0019 dump为90351字节。结构化证据见[collection-atomic-settlement-poc.json](evidence/collection-atomic-settlement-poc.json)。

本片使用合成来源响应和内存EvidenceStore，没有访问外部平台或用户现有MinIO，也没有改动HTTP/OpenAPI契约。它关闭了单页结算的两个事务窗口，但多页/多根评论、现有MinIO、真实来源和七日观察仍未验收；EV-007-005与TASK-007-S03-T02仍为部分完成。

## 007 S02-T03D 主题匹配审核与收件箱筛选验证（2026-09-16）

收件箱现在返回每条 `monitor_match` 的主题ID、不可变版本、命中原因、相关性状态和审核状态。默认查询排除ignored关系；主题、来源、发现时间下界和审核状态由FastAPI查询参数表达。`PATCH /api/monitor-matches/{id}` 只更新指定关系，并在真实变化时同事务写审计；相同状态重放不新增审计。FastAPI运行时契约与发布快照一致，共40组路径；UmiOpenAPI只在`frontend/src/api`生成端点与类型，Axios仍只位于根级`request.ts`。

Red阶段稳定操作ID测试因审核端点不存在而失败。Green/Refactor后，真实PostgreSQL测试让同一内容命中两个主题：忽略主题甲后默认列表仍保留主题乙；主题甲默认筛选为空，显式ignored筛选可见并能恢复；主题乙跟进状态持久化，重复跟进不新增审计。完整181项后端测试通过并保留2条上游弃用提示；Ruff覆盖161个文件，严格mypy覆盖133个源/验证文件，Python与前端生产依赖审计均无已知漏洞。OpenAPI/UmiOpenAPI、前端边界/负向样例、Prettier和TypeScript/Vite构建通过。

一次性`hotkey-inbox-review` Compose在PostgreSQL 16.15、RabbitMQ 4.1.8与0020 schema上完成真实prefork诊断，结果为succeeded/attempts=1；backend替换后代理返回401。Chromium 7项通过，受控收件箱用例验证三次PATCH、主题隔离、恢复、跟进以及主题/来源/24小时筛选参数；工作台空态和390px主流程继续通过。PostgreSQL custom dump为100117字节，恢复后撤权清单重放两次幂等；固定旧应用只读快照回退通过。停机worker=0、backend=143、scheduler=0且无SIGKILL，隔离容器和卷已删除。功能提交`952d501e5551122c4f51ffcafa4188386d447cef`的[远端CI #35063725148](https://github.com/StephenQiu30/hotkey-server/actions/runs/35063725148)在3分31秒内复现全部门禁，远端dump为99152字节。结构化证据见[inbox-match-review-poc.json](evidence/inbox-match-review-poc.json)。

浏览器内容和匹配为明确合成数据，负责验证生成客户端接线；真实PostgreSQL测试负责状态、筛选和审计语义。没有访问外部平台或现有MinIO，也没有证明真实收件箱、真实评论、生产负载或七日观察，因此EV-007-003与完整TASK-007-S02-T03仍未完成。

## 007 S02-T03A 可见页面活动状态轮询验证（2026-09-16）

工作台从UmiOpenAPI生成的`listJobs`和`listCollectionRuns`读取第一页状态；只有页面可见且任一资源处于queued/running时建立2秒轮询链。每条链同时最多各有一个请求，页面隐藏、组件卸载或退出登录时清除计时并取消在途请求；恢复可见后继续同一有界链。Job和CollectionRun都进入终态后执行一次完整工作台刷新，使新收件箱、提醒和事件状态从数据库回显，随后停止轮询。没有增加请求库、WebSocket、SSE、手写URL或OpenAPI兼容分支。

Red浏览器断言在20秒后仍看到排队中的诊断行。Green用真实登录和真实诊断POST配合受控列表状态序列：隐藏4.5秒内Job与CollectionRun读取次数均不增长；恢复后Job先变为succeeded而CollectionRun仍running，轮询继续；CollectionRun完成后两个连续4.5秒窗口请求次数保持不变。该确定性序列避免将prefork完成速度误当成前端轮询证据；同一隔离栈另行证明真实scheduler→RabbitMQ→Celery prefork任务succeeded且attempts=1。

本地完整门禁为170项后端测试通过并保留2条上游弃用提示，Ruff覆盖158文件，严格mypy覆盖131个源/验证文件；`pip-audit`无已知漏洞，npm生产依赖审计为0；FastAPI OpenAPI、UmiOpenAPI、前端边界/负向样例、Prettier与TypeScript/Vite构建通过。全新`hotkey-s02poll` Compose保持0019，backend替换后代理返回401，Chromium 3项通过，PostgreSQL 16.15备份恢复本地dump为90427字节，停机worker=0、backend=143、scheduler=0且无SIGKILL。功能提交`9ebd91e87108cd5a632a3eefcd6ea7f7e5d7f254`的[远端CI](https://github.com/StephenQiu30/hotkey-server/actions/runs/35049798922)在3分19秒内复现全部门禁并成功，远端dump为90351字节。结构化证据见[activity-polling-poc.json](evidence/activity-polling-poc.json)。

验证没有访问外部平台、现有MinIO或真实用户内容，也没有修改HTTP契约、生成客户端、请求预算或来源准入。它证明活动状态自动回显和停轮询边界，不证明真实收件箱、评论多页、七日运行或完整TASK-007-S02-T03。

## 007 S00-T04 / S02-T03B 无数字版本API与采集取消入口验证（2026-09-16）

业务API前缀现在只在`api/router.py`声明一次：非健康操作位于`/api/*`，健康检查仍位于`/health/*`。资源路由不再各自声明全局前缀，也没有旧路径重定向或别名。架构测试要求全部业务OpenAPI路径位于唯一根路径且拒绝数字版本段；单元测试另行证明旧格式路径直接404且没有Location头。FastAPI重新导出Swagger/OpenAPI，`@umijs/openapi`重新生成`frontend/src/api`，业务代码没有手写URL或DTO。

采集运行卡片只在`queued|running`时显示“取消采集”。点击后`App`调用生成的`cancelJob({ identity: run.job_id })`，成功后执行完整重读；界面不自行修改CollectionRun状态。受控Chromium用例证明单次POST路径、`cancelled`回显与终态按钮消失；既有S03-T02B真实PostgreSQL测试继续负责Job/CollectionRun原子取消、页提交屏障和对象补偿语义。

Red阶段的架构测试因Swagger仍包含数字版本路径失败；取消入口浏览器用例在正确Origin下精确失败于按钮不存在。Green/Refactor后171项后端测试通过并保留2条上游弃用提示；Ruff覆盖158个文件，严格mypy覆盖131个源/验证文件。`pip-audit`无已知漏洞，npm生产依赖审计为0；OpenAPI/UmiOpenAPI漂移、前端边界/负向样例、Prettier与TypeScript/Vite构建通过。

全新`hotkey-s02b` Compose在0019 schema上完成scheduler→RabbitMQ→Celery prefork诊断，结果为succeeded/attempts=1；backend替换后代理返回401。清理仅该一次性项目的复用卷后，Chromium 4项通过，覆盖新API根路径的Swagger、采集取消、知识不可用态和完整工作台。PostgreSQL 16.15 custom dump为90457字节，删除清单重放两次幂等；停机worker=0、backend=143、scheduler=0且无SIGKILL。功能提交`586b77610341802c9b8efb7b043772ac5eed618b`的[远端CI](https://github.com/StephenQiu30/hotkey-server/actions/runs/35051558868)在3分钟内复现171项后端测试、Chromium 4项与全部运行门禁，远端dump同为90457字节。结构化证据见[api-root-poc.json](evidence/api-root-poc.json)与[collection-run-cancel-ui-poc.json](evidence/collection-run-cancel-ui-poc.json)。

验证使用合成owner、合成CollectionRun和可丢弃基础设施，没有访问外部平台或现有MinIO。它证明公开HTTP契约迁移、生成客户端和用户取消接线，不证明真实来源中断、多页评论、现有MinIO或七日稳定性。

## 007 S03-T01C 根评与回复两页续采验证（2026-09-16）

根评与回复运行现在最多各采集2页。第1页保存RawPage、内容与Checkpoint后，同一PostgreSQL事务预留下一请求预算、结束当前Attempt、增加Job epoch并写入立即Outbox。第2个epoch领取同一CollectionRun和Job，从数据库最新Checkpoint恢复数字游标；RabbitMQ消息仍只有job_id、epoch和契约版本。旧epoch无法重新领取。

根评运行的全部已提交页合计只选择1个有回复的根评，不会因第2页再派生新的回复运行。第2页仍有游标时以`partial/page_limit`结束，不写epoch 3；第1页后预算用尽时保留已提交证据和Checkpoint，并将运行与Job原子结算为`partial/page_budget_exhausted`。

Red阶段的内部页契约用例因cursor被拒绝而失败。Green/Refactor后，177项后端测试在真实PostgreSQL/RabbitMQ下通过，保留2条上游弃用提示；Ruff检查158个文件，严格mypy检查131个源/验证文件。`pip-audit`无已知漏洞，npm生产依赖审计为0；FastAPI OpenAPI、UmiOpenAPI生成客户端、前端边界/负向样例、Prettier和TypeScript/Vite构建均通过，HTTP契约与生成文件无变化。

全新隔离`hotkey-s03g` Compose保持0019 schema，scheduler→RabbitMQ→Celery prefork诊断为`succeeded/attempts=1`；运行时OpenAPI等于发布快照，backend替换后代理返回401。Chromium 5项通过。PostgreSQL 16.15 custom dump为90570字节，恢复后删除清单重放两次幂等；停机worker=0、backend=143、scheduler=0且无SIGKILL。功能提交`84effd002d41965accc3fe1359eb3d228d3c5353`的[远端CI](https://github.com/StephenQiu30/hotkey-server/actions/runs/35055542112)在2分29秒内复现全部门禁并成功：177项后端、Chromium 5项、prefork attempts=1、代理401、备份恢复dump 90556字节与0/143/0停机通过。结构化证据见[collection-two-page-poc.json](evidence/collection-two-page-poc.json)。

本片使用合成页响应和内存EvidenceStore，没有访问外部平台、连接用户现有MinIO或改变来源准入。它证明有界两页续采的事务、恢复和预算语义，不证明真实平台的页面口径、更广多根评覆盖或七日稳定性。

## 007 S06-T01A 旧应用隔离快照回滚演练（2026-09-16）

旧应用基线固定为 `13545f223ea9257464c6d4f8c91c16c2543eb8d0` / `0019_collection_retry_budget`，当前库为 `0020_trend_alerts`。Red验证将旧应用直连当前库，就绪端点稳定返回503/`schema_mismatch`；这证明不能用放宽revision检查的方式伪造回滚。

`verify_old_application_rollback.py` 在可丢弃Compose项目内导出该固定提交，新建隔离库并用旧迁移重放到0019，创建合成owner和会话后停止可写应用。随后用PostgreSQL `default_transaction_read_only=on`和仅SELECT授权的角色重启旧应用；就绪、既存会话和事件列表读取通过，30张旧库表的计数在HTTP读取前后不变，当前0020库也没有变化。脚本最后删除隔离库、只读角色和临时源码。结构化证据见 [old-application-rollback-poc.json](evidence/old-application-rollback-poc.json)。

这是一次隔离数据库的紧急只读回退演练：不增加旧接口、跳转、双路由、Schema降级或永久旧服务。它没有执行生产流量切换、异地备份或现有MinIO物理对象恢复，因此仍不能独立将EV-007-010标记完整通过。

## 007 S03-T01D 两根评独立回复链验证（2026-09-16）

每个重点帖仍只有一个根评论运行且最多2页；系统从这些已提交页按页序和平台返回顺序最多选择2条存在回复的根评论。每个父级分别创建CollectionRun、Job、Checkpoint和预算账本，并各自最多续采2页。搜索候选、正文和根评论的派生上限仍为1。来源请求预估与运行时派生共同读取按operation定义的上限，单关键词/单B站/每小时最大意图从96调整为120次/日；既有monitor version快照不回填。

Red阶段来源预估用例期望3个查询合计15次而旧实现只返回12次。Green用两页合成根评论返回3个有回复父级：只为前2个建立回复运行，第1个完成2页并以`partial/page_limit`结算，第2个独立完成1页并成功，第3个不建立run/Job且不占预算。最终对账为5个CollectionRun、5个Job、7个Outbox、7个RawPage、7个Checkpoint和7次预算；119次预算拒绝启用，120次通过。FastAPI自动OpenAPI只更新`BudgetSpec.daily_requests`默认值；`@umijs/openapi`重新生成后`frontend/src/api`无端点漂移，Axios仍只位于`frontend/src/request.ts`。

真实PostgreSQL 16.15和RabbitMQ 4.1.8下180项pytest通过并保留2条上游弃用提示；Ruff检查161个文件，严格mypy检查133个源/验证文件，Python与前端生产依赖审计为0已知漏洞。OpenAPI运行时与快照相同，共39组路径；UmiOpenAPI、前端边界/负向样例、Prettier和TypeScript/Vite构建通过。隔离`hotkey-s03-multi-root`栈完成真实prefork诊断`succeeded/attempts=1`、backend替换代理401、Chromium 6项、PostgreSQL备份恢复与删除清单两次幂等重放，dump为99114字节；固定0019旧应用在只读隔离快照完成回退读取。停机worker=0、backend=143、scheduler=0且无SIGKILL，随后删除本次容器、网络和卷。结构化证据见[collection-multi-root-replies-poc.json](evidence/collection-multi-root-replies-poc.json)。

首轮全量回归发现使用旧四请求估算的低预算测试与备份夹具无法启用监控；夹具统一到五请求链后全量通过。第一次Compose smoke与backend替换被并行执行，替换过程使smoke命令退出137；按CI顺序串行复跑后任务一次成功。功能提交`0433fa8d287b55e13eec2646d476547943de8f23`的[远端CI #35061534325](https://github.com/StephenQiu30/hotkey-server/actions/runs/35061534325)在3分48秒内复现180项后端、6项Chromium、prefork一次执行、代理替换、99015字节备份恢复、旧应用回退和无SIGKILL停机门禁并成功。验证只使用合成来源和内存EvidenceStore，没有请求外部平台或连接现有MinIO；真实评论链、超过两页/两父级的全量树和七日观察仍未验收。

## 007 S03-T01E 内容详情与评论上下文验证（2026-09-16）

FastAPI新增自动文档化的`GET /api/contents/{identity}`，读取选中内容、当前唯一可解析的根帖与父评论，以及同一根帖下已解析且可用的评论/回复。评论列表默认50、最大100，按`(first_seen_at,id)`稳定正向分页。读取会再次确认同来源根外部ID和父外部ID当前只有一个候选；即使历史关系曾经resolved，后续出现命名空间碰撞也不会把不同根帖的讨论混合。未解析关系保留选中内容并返回空上下文，已撤权和未知内容统一返回`content_not_found`。

工作台在用户点击“查看评论上下文”前不发详情请求；点击后只调用UmiOpenAPI生成的`getContentDetail`，展示根帖、父评论和等价列表，并可继续读取下一页。详情读取不创建Job、CollectionRun或来源请求。Red阶段稳定操作ID断言确认接口不存在；Green的真实PostgreSQL测试固定根帖、评论、回复、歧义双根和撤权内容，验证两页无重复、关系隔离以及读取前后任务账本计数不变。

本地完整门禁为184项后端测试通过并保留2条上游弃用提示，Ruff覆盖162个文件，严格mypy覆盖133个源/验证文件；`pip-audit`无已知漏洞，npm生产依赖审计为0。FastAPI运行时契约与41组路径的发布快照相同，UmiOpenAPI、前端边界/负向样例、Prettier和TypeScript/Vite构建通过。一次性`hotkey-detail-stack`在PostgreSQL 16.15和RabbitMQ 4.1.8上完成prefork诊断`succeeded/attempts=1`、backend替换代理401、Chromium 8项、99488字节备份恢复与删除重放、固定0019旧应用隔离只读回退；停机worker=0、backend=143、scheduler=0且无SIGKILL，全部隔离资源已删除。功能提交`79bb4af263e94d1bcf9a7dd0130b8130b5545a6f`的[远端CI #35066046632](https://github.com/StephenQiu30/hotkey-server/actions/runs/35066046632)在3分34秒内复现184项后端、8项Chromium、prefork一次执行、代理401、99342字节备份恢复、旧应用回退和无SIGKILL停机门禁并成功。结构化证据见[content-discussion-poc.json](evidence/content-discussion-poc.json)。

浏览器和数据库使用明确的合成内容，没有请求外部平台或连接用户现有MinIO。本片证明持久评论事实的只读产品入口、身份歧义屏障和生成客户端接线，不证明真实平台评论内容、超过既有两页/两父级口径的全量树、现有MinIO或七日稳定性；`TASK-007-S03-T01`和EV-007-004仍保持部分完成。

## 007 S04-T01D 事件档案评论上下文验证（2026-09-16）

事件档案中的每个成员现在复用内容领域的`ContentDiscussion`。初次渲染不读取详情；用户点击“查看评论上下文”后，组件只调用UmiOpenAPI生成的`getContentDetail`，展示已持久化的根帖、父评论和回复。事件界面只传递成员已有的内容ID、外部ID与类型，不复制关系解析、撤权或分页规则，也没有新增HTTP端点、手写请求、迁移或依赖。

Red浏览器用例精确失败于事件成员中不存在展开按钮。Green/Refactor后目标用例和空卷完整Chromium 9项通过；完整回归第一次复用了前一轮主工作台创建的事件，空态断言按设计失败，删除本次可丢弃卷后原样通过。首次远端CI还发现既有轮询用例会被更快的prefork抢先完成：测试现在让POST响应稳定进入受控排队态，之后仍只通过生成的Job/CollectionRun列表读取推进终态；真实诊断POST和Worker执行没有被替换。真实PostgreSQL/RabbitMQ下184项后端测试通过并保留2条上游弃用提示；Ruff覆盖162文件，严格mypy覆盖133个源/验证文件；Python与前端生产依赖审计无已知漏洞，FastAPI OpenAPI、UmiOpenAPI漂移、前端边界/负向样例、Prettier与构建通过。

一次性`hotkey-event-context` Compose按CI顺序完成prefork诊断`succeeded/attempts=1`、Web不替换而backend替换后的代理401、Chromium 9项、PostgreSQL 16.15 custom dump 99345字节及删除清单两次幂等重放、固定0019旧应用隔离只读快照回退；正常停机worker=0、backend=143、scheduler=0且无SIGKILL。修复提交`92d8a849d17a8ac120a33155968014f58de11b7e`的[远端CI #35068391771](https://github.com/StephenQiu30/hotkey-server/actions/runs/35068391771)在3分30秒内完成184项后端、9项Chromium、prefork/代理/恢复/回退/停机全套门禁，远端dump为99360字节。结构化证据见[event-content-context-poc.json](evidence/event-content-context-poc.json)。本片使用合成事件和受控浏览器响应，没有请求外部平台、连接现有MinIO或证明真实评论与七日稳定性，因此不独立完成EV-007-004/006。

## 007 S03-T01F 用户选择内容并启动评论追踪验证（2026-09-16）

收件箱的“追踪评论”现在调用FastAPI自动文档化、UmiOpenAPI生成的`startCommentTracking`。服务在一个PostgreSQL事务中锁定主题命中，确认其监控版本仍活动，解析唯一可用根帖，再从根帖可用RawPage反查同版本来源运行。已有详情证据时创建或重放`list_comments`，只有搜索证据时创建或重放`fetch_post`；Job、Outbox、预算与`following`状态一起提交。普通审核接口只接受`new|ignored`，不能单独写“评论追踪中”。

Red阶段后端操作ID测试精确失败于端点缺失，Chromium用例精确失败于收件箱没有“追踪评论”按钮。Green/Refactor后真实PostgreSQL集成测试覆盖已有一个抽样详情任务后为用户选择的第二帖子建立独立任务、重复调用不重复扣预算、详情页直接进入评论任务，以及未配置证据存储、预算耗尽、监控失活、根身份歧义和跨监控版本证据时命中/预算/队列零变化。完整后端189项通过并保留2条上游弃用提示；严格mypy检查133个源/验证文件，`pip-audit`与npm生产依赖审计无已知漏洞。FastAPI运行时契约与42组路径快照一致，生成客户端只位于`frontend/src/api`并继续使用根级Axios `request.ts`；边界、负向边界、Prettier与TypeScript/Vite构建通过。

全新`hotkey-comment-tracking`一次性Compose完成`scheduler → RabbitMQ → Celery prefork → PostgreSQL`诊断，结果为`succeeded/attempts=1`；Web不替换而backend替换后代理返回401。空库Chromium 10项通过，包括Swagger、新评论追踪调用与完整工作台。PostgreSQL 16.15 custom dump为99472字节，恢复后撤权清单重放两次幂等；固定0019应用从数据库强制只读的隔离旧Schema快照完成读取。正常停机worker=0、backend=143、scheduler=0且无SIGKILL，随后删除该项目容器、网络和卷。功能提交`4c1695b42c40ee95259eff8a0fa2956fe0c8aab7`的[远端CI #35079689852](https://github.com/StephenQiu30/hotkey-server/actions/runs/35079689852)在3分48秒内复现189项后端、10项Chromium、prefork一次执行、代理401、99480字节备份恢复、旧应用回退与无SIGKILL停机门禁并成功。结构化证据见[comment-tracking-poc.json](evidence/comment-tracking-poc.json)。

验证内容、来源页和浏览器响应均为明确合成数据，没有访问外部平台或用户现有MinIO。它证明用户动作与持久采集账本的原子接线，不证明真实平台评论、用途准入、现有MinIO权限、真实内容盲测或七日稳定运行；`TASK-007-S03-T01`和EV-007-004仍保持部分完成。

## 007 S02-T02G 现有 MinIO 实例隔离对象验证（2026-09-16）

验证直接复用用户已有项目的私有生产配置，只在子进程内将已有键映射为 `HOTKEY_S3_*`。命令和证据均未输出 endpoint、bucket、access key、secret key 或既有对象名；应用未创建bucket、未修改策略/生命周期，也未遍历非本次唯一键。

公开端点使用配置声明的TLS时，在bucket访问和证书链校验前就以`UNEXPECTED_EOF_WHILE_READING`结束握手，指向端口/反向代理的TLS协议配置，不能记录为证书无效。本项目没有关闭TLS或证书校验，也没有加入不安全兼容分支。同一份配置明确给出的主机回环HTTP端点可用：`verify_minio.py` 在1秒内将一条合成JSON的确定性gzip对象写入预先存在的bucket，第二次重投返回同一对象；65字节完整读回的SHA-256为 `d0ca6df230d1274f6dccd4b1e691367467aa12809b9ec75319789bb7176f3ad6`。随后按精确键删除所有版本/删除标记，带版本列举和`stat`均证明无余留。

聚焦配置/适配器的12项pytest通过，Ruff通过，仓库官方mypy目标检查133个源文件且无错误，证据JSON可解析。结构化证据见 [existing-minio-poc.json](evidence/existing-minio-poc.json)。这一结果证明现有实例、已有凭据和bucket可在本机开发路径承载当前对象协议。主机回环端点不能直接作为生产容器配置；公开TLS端点配置、可从生产网络到达的端点、对象锁/生命周期与真实来源页入库仍需后续验收，因此不独立完成EV-007-003。

## 007 S02-T02H Compose 容器到现有 MinIO 验证（2026-09-16）

验证复用当前 `backend/Dockerfile` 产出的Linux/arm64 `hotkey-backend:local`镜像，将`verify_minio.py`以只读方式挂载，使用默认Docker bridge与`host.docker.internal`/主机网关映射访问已有主机转发端口。容器去除全部Linux capabilities、启用`no-new-privileges`和只读根文件系统，只提供16MiB临时`/tmp`。私有值通过仓库外的0600临时env文件传入，验证结束后`finally`已删除，命令与输出均不含私有值。

首次启动在访问网络前因挂载脚本的Python模块路径缺失退出；显式使用镜像内`PYTHONPATH=/app/src`后原样重试通过，没有改应用代码、网络或安全策略。容器对一个65字节确定性gzip对象完成首次上传、幂等重投、完整读回SHA-256、全版本精确删除和`stat`不存在校验；容器退出0，对象hash为`63bdd909ee9377fa0e42701387cf1036c546381ceb33b76582508fbfa634a6cf`。

结构化证据见 [existing-minio-container-poc.json](evidence/existing-minio-container-poc.json)。这证明本机Compose部署可显式使用`host.docker.internal:<existing-port>`复用已有MinIO，不需要host network或业务代码地址重写。公开TLS握手配置、远程生产网络、对象锁/生命周期和真实来源入库仍未验收，不独立完成EV-007-003。

## 007 S02-T02I 现有 MinIO 保留策略只读审计（2026-09-16）

`evidence.audit` 只依赖MinIO的bucket HEAD、版本化、对象锁和生命周期公开读取接口；`audit_minio_bucket.py`只负责从`HOTKEY_S3_*`构造已有适配器并输出脱敏JSON。审计不写对象、不列对象名、不修改bucket策略，也不输出bucket、endpoint、rule ID/prefix或凭据。AccessDenied与策略缺席使用不同状态，其他S3错误保持非0退出。

现有bucket可达，四项读取均有权限。实例当前是`unversioned`，对象锁为`absent`，生命周期为`absent`。因此现有精确删除不受bucket默认保留锁限制，也不会产生需要额外删除的历史版本；同时服务端没有自动过期规则或`AbortIncompleteMultipartUpload`，不能仅依靠bucket策略保证保留期与中止分片清理。

离线fake client共16项配置/适配器/审计测试通过，Ruff通过，官方mypy检查135个源/脚本文件且无错误。功能提交`79e16970`的[完整远端CI](https://github.com/StephenQiu30/hotkey-server/actions/runs/35091475304)通过，包括PostgreSQL、RabbitMQ、OpenAPI/UmiOpenAPI契约、Chromium、备份恢复和回退演练。结构化结果见 [existing-minio-retention-audit.json](evidence/existing-minio-retention-audit.json)。本片不自动修改外部MinIO；应用依旧需以删除账本/对账作为可变保留天数的主路径，生产启用前应在bucket配置有界的未完multipart中止规则并复核是否需要版本历史。该审计不独立完成EV-007-003/010。

## 007 S02-T02J B站公开真实持久链 canary（2026-09-16）

个人非商业学习范围下，先以无登录、无Cookie、无环境代理的有界客户端各请求一次B站搜索、正文、根评论和回复；四项均返回HTTP 200，解析出1条搜索引用、1条正文、3条根评论和首个父级19条回复。输出与仓库证据只保留状态、数量和字节数，没有正文、作者或平台对象ID。

正式产品首轮在真实Worker完成来源搜索后，写入现有MinIO时发现Compose没有固定此前POC使用的主机网关。布局Red测试精确失败后，`x-app`统一加入`host.docker.internal:host-gateway`，backend、worker和scheduler均从同一部署事实继承。原样复跑进一步确认现有MinIO宿主机端口已停止监听；没有启动CI fixture或新建bucket冒充既有实例。为隔离来源链与外部实例状态，使用一次性MinIO fixture继续验证，bucket由产品镜像中的锁定`minio-py 7.2.20`创建。

真实prefork链完成搜索与正文并派生评论：搜索以`partial/page_limit`结算、正文`ok`，评论请求返回`access_denied`并原子失败，未继续请求回复。搜索、正文和评论拒绝响应共3个RawPage、3个Checkpoint；3个对象均从fixture按SHA-256完整读回，正文形成1个内容版本/观察。随后通过应用删除账本删除全部3个对象和所有版本，0失败、0余留；隔离PostgreSQL、RabbitMQ、MinIO卷、容器和临时env全部删除。没有对评论拒绝高频重试，总内容采购费用为0。

结构化证据见 [EV-007-003-bilibili-live-canary.json](evidence/007/EV-007-003-bilibili-live-canary.json)。功能提交`c8d2c320301a9bd489ccacfaad99b7f772449a38`的[远端CI #35095350599](https://github.com/StephenQiu30/hotkey-server/actions/runs/35095350599)在3分31秒内通过，复现194项后端测试、10项Chromium、OpenAPI/UmiOpenAPI契约、真实PostgreSQL/RabbitMQ prefork、代理替换、备份恢复、旧应用回退和无SIGKILL停机门禁。它证明真实来源的搜索/正文持久主链和拒绝状态处理，但不证明真实评论/回复持久化、现有MinIO当前可用、生产来源准入或七日稳定性；EV-007-003仅部分通过，EV-007-004保持未完成。
