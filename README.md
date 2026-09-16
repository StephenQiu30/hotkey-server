# HotKey

个人热点监控学习项目。服务端已统一为 **Python / FastAPI / SQLAlchemy / RabbitMQ**，采用 PostgreSQL、Alembic、Celery；浏览器端使用 React、TypeScript 和 Vite。业务API统一位于`/api/*`，URL不携带数字版本段，不保留旧路径兼容层。

目前可以初始化单用户账号、登录和退出、保存/编辑多来源关键词监控草稿、提交/查询/取消诊断任务。活动监控可从工作台立即采集：前端只用Swagger/UmiOpenAPI生成函数提交当前版本与幂等键，后端与周期调度共用同一批次编排，从监控快照派生来源、查询、时间窗和保留期，并在一个事务中预留整批预算。诊断经 PostgreSQL Outbox → RabbitMQ → Celery prefork → PostgreSQL 结果表，支持幂等、租约、重投与过期执行拒绝。工作台仅在页面可见且存在排队或运行中的任务时，每2秒读取一次生成的Job与CollectionRun接口；页面隐藏会停止并取消在途轮询，两类活动项都进入终态后完整刷新一次并停止。活动采集运行可从运行卡片直接取消，页面只调用Swagger生成的Job取消函数并重读后端终态。

新增 Bluesky 查询预览和有界 CLI 来源探测，可读取公开线程并区分受限、空结果和部分结果。持久采集账本、MinIO适配器、UTC日预算、周期槽、运行列表和收件箱基础已经实现；收件箱可按主题、来源、发现时间和审核状态筛选，用户可对某条主题命中的具体内容启动真实评论追踪，也可忽略或恢复待处理状态，同一内容的其他主题不受影响。评论追踪会在同一事务创建或重放下一步CollectionRun、Job与Outbox并占用预算，成功后才显示“评论追踪中”；普通审核接口不能伪造该状态。用户可在收件箱或事件档案中按需展开已入库内容的根帖、父评论和有界评论列表，详情读取不会触发采集。B站搜索引用可依次派生正文详情，每个重点帖最多采集两页根评论，并按平台顺序为最多两条根评论分别采集两页回复和保留独立证据。来源429会按Retry-After建立新epoch任务，暂时失败采用有界退避，每次后续尝试重新占用日预算；永久失败、预算/尝试耗尽会让采集运行与Job原子失败。最后一页的证据、内容、Checkpoint、CollectionRun与Job现在同事务结算，消息在提交后重投不会再次请求来源。用户取消采集时Job与CollectionRun原子结算；请求或对象上传期间取消均不能越过页提交屏障，上传后拒绝的对象会精确补偿或进入清理账本。工作台已支持人工事件归并/拆分、分平台趋势、可比较时间桶阈值提醒、冻结评论样本、人工观点分析和带引用的知识快照。知识库提供精确/语义检索、受控评论统计、确定性证据问答和在线撤权屏障；语义索引通过自建 Ollama 与 pgvector 可恢复建立。删除清单、共享证据页保守撤回、MinIO精确键全版本删除和恢复后重放已完成合成数据闭环；旧应用已通过固定提交与隔离旧Schema快照完成数据库强制只读回退演练。**现有MinIO已在主机和本机Compose容器通过隔离对象读写/全版本删除，公开端点严格TLS与远程生产网络仍未验收；真实来源尚未准入，超过两页或两个父级的全量评论树、真实内容盲测、自动模型标注和生产备份恢复尚未验收。** 草稿和页面读取不会启动采集，诊断不会生成社交数据。当前实现进度见 [007 Plan](docs/plans/007-热点事件与评论知识库计划.md)。

2026-09-15 新规划：**监控主题 → 发现收件箱 → 评论追踪 → 事件档案 → 分析与知识库**。已形成 [007 产品需求](docs/prd/007-热点事件与评论知识库.md)、[信息模型与技术设计](docs/design/007-热点事件与评论知识库设计.md)、[实施计划](docs/plans/007-热点事件与评论知识库计划.md)。工程门禁和不依赖真实来源的S02基础已实施，继续按真实来源与数据闭环逐片交付。

## 启动

需要 Docker Compose v2.24.4+。从仓库根目录执行：

```sh
docker compose up -d --build
docker compose exec backend python -m cli owner-init learner
```

第二条命令交互输入至少 12 位密码，无默认账号。打开 http://localhost:8010。首次运行迁移服务先建立新库，API/Worker 启动不修改表。开发端口仅绑定本机回环地址，可复制 `.env.example` 修改端口；改变网页端口时也要修改 Compose 中的允许来源。

## 工程与验证

- `backend/src/`：main 应用工厂、api 协议层、identity/monitors/jobs/sources 业务模块、core/db 公共设施、worker/ 队列执行、cli/ 管理命令及应用迁移。目录规范见 [backend README](backend/README.md)。
- `frontend/src/`：`app/features` 工作台、`api/` 自动生成端点，以及根级 `request.ts` Axios封装；不设置shared层。
- `docs/openapi/openapi.json`：FastAPI 自动导出的唯一发布契约快照；运行时 Swagger UI 为 `http://localhost:8867/docs`，OpenAPI JSON 为 `http://localhost:8867/openapi.json`。
- 根 Compose：唯一运行编排；生产使用覆盖文件。

```sh
uv sync --project backend --locked
uv run --project backend ruff check backend
uv run --directory backend mypy
uv run --project backend pytest backend/tests
uv run --directory backend/src python -m tools.export_openapi --check
npm ci --prefix frontend
npm run check:contract --prefix frontend
npm run check:boundaries --prefix frontend
npm run test:boundaries --prefix frontend
npm run build --prefix frontend
```

完整数据库/消息测试需要可丢弃的 `hotkey_test` 数据库和同名 RabbitMQ vhost，否则明确跳过；不能把跳过视为完整验证。设置步骤、浏览器测试和生产覆盖见 [运行手册](docs/operations/006-Python运行与验证.md)。

真实采集还需同时配置完整的 `HOTKEY_S3_*` 与两个JSON数组：`HOTKEY_SOURCE_RIGHTS_ALLOWED` 表示已确认的用途范围，`HOTKEY_SOURCE_PIPELINES_CONNECTED` 表示已通过持久化POC的操作。默认都为 `[]`。当前代码允许将 `bilibili.search_posts`、`bilibili.fetch_post`、`bilibili.list_comments` 和 `bilibili.list_replies` 声明为connected，并要求四者都准入后才启用B站搜索；未知键或尚无消费者的操作会使服务启动失败，避免界面与Worker对准入状态理解不一致。

语义检索需要后端容器可访问的现有自建 Ollama 服务，并同时配置 `HOTKEY_EMBEDDING_BASE_URL` 和该模型的 `HOTKEY_EMBEDDING_MODEL_DIGEST`。模型和维度固定为 `qwen3-embedding:latest` 与 1024；数据库镜像包含 pgvector 0.8.6。未配置时知识精确检索保持可用，语义索引和语义查询返回明确的不可用状态。

删除操作先在数据库建立在线屏障，再由运维显式执行 `python -m cli deletion-export <私有路径>` 导出0600权限清单、`deletion-replay <私有路径>` 在恢复后重放、`evidence-reconcile --limit 20` 有界清理MinIO对象。CI已在可丢弃Compose库中用真实`pg_dump -Fc`和`pg_restore`验证“旧备份恢复→删除清单重放两次”的顺序。对象清理需要完整的 `HOTKEY_S3_*` 配置；应用不会创建bucket、修改对象锁或生命周期策略。现有实例完成TLS、删除权限、版本历史和未完成multipart生命周期核对前，不能将数据库恢复POC视为生产物理删除或完整灾备验收。

旧应用回退不会放宽Schema就绪检查或增加兼容路由。CI使用固定历史提交，把发布前Schema恢复到隔离数据库，再以PostgreSQL强制只读角色启动旧应用并验证就绪、会话和事件列表。当前数据库及新表全程保留；生产仍需按实际备份位置演练流量切换。

旧实现、旧契约和旧验收文档已从工作树删除，可通过 Git 历史查阅；新服务不兼容旧接口、不读取旧库、不自动迁移旧账号。已有持久卷不会被清理。使用与贡献请参阅 [规范](AGENTS.md)、[安全说明](SECURITY.md) 和 [LICENSE](LICENSE)。
