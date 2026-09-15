# HotKey Backend

后端直接作为 FastAPI 应用运行，代码统一放在 `backend/src/`，src 下不增加 hotkey 或 app 包装层。uv 管理依赖，不构建独立 wheel。SQLAlchemy 与 RabbitMQ 保持固定。

下图描述现有目录。后续扩展与文件迁移先遵循[007目标目录与模块职责](../docs/design/007-热点事件与评论知识库设计.md#13-实现前目录规划与文件归属)及[007 S00计划](../docs/plans/007-热点事件与评论知识库计划.md)，完成所需选型和依赖门禁后再开始业务切片。计划目录尚未实际创建或迁移。

```text
backend/
├── src/
│   ├── main.py                 # FastAPI 工厂
│   ├── api/                    # HTTP 路由和依赖
│   ├── core/ / db/             # 配置、数据库资源
│   ├── identity/ / monitors/ / jobs/  # 业务模块及持久任务账本
│   ├── collection/ / contents/ # 单页采集编排、内容与收件箱
│   ├── evidence/               # 证据协议、元数据与MinIO适配器
│   ├── sources/                # 查询预览、原始页契约与有界适配器
│   ├── worker/                 # app.py、messaging.py：Celery 与 RabbitMQ
│   ├── cli/                    # __main__.py、commands.py：管理命令
│   ├── audit/ / migrations/    # 审计、数据库迁移
│   └── tools/                  # 契约生成
├── scripts/                    # 容器验证
├── tests/                      # 单元、集成与架构检查
├── pyproject.toml / uv.lock
└── Dockerfile
```

从仓库根目录执行：

```sh
uv sync --project backend --locked
uv run --project backend ruff check backend
uv run --directory backend mypy
uv run --project backend pytest backend/tests -q
uv run --directory backend/src python -m tools.export_openapi --check
docker compose up -d --build
docker compose exec backend python -m cli owner-init learner
```

Python 运行工作目录为 backend/src，Docker 内为 /app/src。ASGI 入口 `main:create_app`，Worker 入口 `worker.app:app`，管理命令 `python -m cli`。迁移目录随应用复制进镜像；启动应用不自动改库。

FastAPI 从路由、状态码和 Pydantic 模型自动维护接口文档。服务启动后访问 `http://localhost:8867/docs` 查看 Swagger UI，`http://localhost:8867/openapi.json` 获取运行时契约。`docs/openapi/openapi.json` 只能由 `python -m tools.export_openapi` 导出，前端再由 `@umijs/openapi` 生成请求函数和类型；这些生成文件均禁止手写。

业务分层与命名规则见 [AGENTS](../AGENTS.md)，实际服务与浏览器验证见 [Operations](../docs/operations/006-Python运行与验证.md)。未配置测试数据库/vhost 时集成测试会 skip，不能视为完整通过。`collect_page` 已接入同一 Job/Outbox/Celery 账本；scheduler为active配置生成最近一个已结束周期槽，并在创建任务前按monitor version与UTC日期原子预留一次请求。`POST /api/v1/monitors/{id}/runs` 创建持久运行，`GET /api/v1/collection-runs` 分页列出运行，`GET /api/v1/collection-runs/{id}` 查询状态。创建操作仍受来源用途准入和完整 MinIO 配置双门禁；当前来源目录均未准入，所以这些接口和scheduler不会对真实平台发起请求。

来源准入使用两个JSON数组，元素必须是精确的 `source.operation`：

```sh
HOTKEY_SOURCE_RIGHTS_ALLOWED='["bilibili.search_posts","bilibili.fetch_post"]'
HOTKEY_SOURCE_PIPELINES_CONNECTED='["bilibili.search_posts","bilibili.fetch_post"]'
```

第一项是部署者对采集、保存和派生用途的确认，第二项只在该操作的持久消费者与证据链通过POC后设置。两项默认空；connected还要求完整 `HOTKEY_S3_*`。API在lifespan启动时校验，scheduler与Worker读取同一Settings。当前版本实现 `bilibili.search_posts` 与 `bilibili.fetch_post` 持久入口；搜索依赖详情，两项必须分别准入。评论、回复、其他平台operation或未知键仍会启动失败。本示例仅说明格式，不能替代真实用途确认与现有MinIO验收。

来源探测无需数据库或 RabbitMQ 配置，每次只发送一个有界请求：

```sh
uv run --directory backend/src python -m cli source-probe bluesky search --keyword science --since 2026-09-01T00:00:00Z --until 2026-09-08T00:00:00Z --limit 20
uv run --directory backend/src python -m cli source-probe bluesky thread --uri 'at://did:plc:YOUR_DID/app.bsky.feed.post/YOUR_RECORD_KEY' --depth 1 --max-nodes 20
uv run --directory backend/src python -m cli source-probe bilibili search --keyword 人工智能 --limit 3
```

线程示例中的 DID 和记录键需替换成真实帖子标识。CLI 输出 JSON；输入错误退出 2，来源失败退出 1，ok/empty/partial 退出 0，因此必须读取 status/code 判断是否部分结果。HTTP 查询预览为已认证的 `POST /api/v1/sources/query-preview`，需要会话、Origin 与 CSRF；仅验证和规范化查询，不发起外部请求。

MinIO 只连接已存在的私有实例和 bucket。设置全部 `HOTKEY_S3_*` 连接变量后，可执行隔离对象协议验证：

```sh
PYTHONPATH=backend/src uv run --project backend python backend/scripts/verify_minio.py
```

脚本要求应用的 PostgreSQL/RabbitMQ 配置同时有效，但不会连接它们。它只在现有 bucket 的 `raw/poc/` 前缀写入合成对象，验证上传、完整读回与幂等重投后删除该对象；不会创建 bucket、修改策略或清理其他对象。
