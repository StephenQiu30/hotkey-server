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

交互输入 12–128 位密码，不通过命令参数传入。owner 只能初始化一次，数据库唯一约束阻止并发创建第二账号。打开 http://localhost:8010；当前只有监控草稿和诊断任务。来源、评论、分析和报告尚未接入。

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
npm run format:check --prefix frontend
npm run build --prefix frontend
npm audit --prefix frontend --audit-level=high
```

修改 API 后执行 `uv run --directory backend/src python -m tools.export_openapi`，然后 `npm run generate --prefix frontend`，同时审查 JSON 与生成类型；不要手工编辑生成文件。Cookie 认证方案、CSRF Header、输入输出和稳定错误格式均包含在契约中。

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
cd frontend
npx playwright install chromium
# 在当前进程环境中设置 HOTKEY_E2E_USERNAME 与 HOTKEY_E2E_PASSWORD，勿提交凭据。
npm run test:e2e
cd ..
uv run --project backend python backend/scripts/verify_shutdown.py
```

可选 `HOTKEY_WEB_URL` 改变测试 Web 地址，默认 localhost:8010。Playwright 验证登录、新建/编辑草稿、真实诊断完成、页面刷新保留状态、390px 布局和退出后 401。截图只含合成数据。CI 使用一次性项目，结束时只清理其自身资源。

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
