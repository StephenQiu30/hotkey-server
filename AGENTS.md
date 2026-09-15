# HotKey 工程规范

本文件适用于整个仓库。个人学习项目，固定 SQLAlchemy 2 和 RabbitMQ。

后端工程的规范事实源为 [008 FastAPI后端工程治理设计](docs/design/008-FastAPI后端工程治理设计.md)。外部教程和公司规范用于提供依据，不直接覆盖本项目已决定的同步SQLAlchemy、模块化单体、OpenAPI和任务账本边界；需要偏离008时先修改Design、Plan和架构测试。

## 任务开始前的目录与选型门禁

- 每个实现切片开始前先读 [007 Design](docs/design/007-热点事件与评论知识库设计.md) 第3.1节技术登记、第13节目录/依赖规范，以及 [007 Plan](docs/plans/007-热点事件与评论知识库计划.md) 的文件迁移表；列明本片新增、修改、移动和生成文件及职责，不先写文件后找目录。
- 技术选择已有用户决定的直接沿用；影响本片的部署、费用、平台范围或新框架等未决项，先给出选项和影响询问用户，未答复不执行依赖该决定的工作。普通实现细节按已定规范处理，不重复确认已确定的技术栈。
- 用户已明确：复用现有MinIO作对象存储；不付费采购内容，采用免费或自建服务；X等主流信息平台优先发现，评论研究覆盖B站/微博/小红书/抖音。AI推理费用范围单独确认，不将内容预算解释成付费AI授权。
- 新后端模块、前端功能目录必须登记职责并纳入全源码/依赖检查，不能因不在固定扫描名单而漏检。当前全面门禁尚待S00实现；不能仅凭文档声称检查已覆盖新模块。
- 按业务切片创建目录，不提前创建空模块。来源适配器放sources/adapters，MinIO适配器放evidence/adapters，模型SDK适配器放ai/adapters；业务状态仍由业务模块持有。前端使用app/features，生成客户端固定在src/api，Axios封装固定在src/request.ts，不创建shared层。

- `backend/` 是唯一后端：Python 3.12、FastAPI、SQLAlchemy、Alembic、Celery。禁止恢复 Go 后端、独立旧 Agent 或兼容旧接口。
- `frontend/` 是 React/TypeScript 工作台。唯一 HTTP 契约由 FastAPI 路由与 Pydantic 模型生成，运行时位于 `/openapi.json`，可复现快照为 `docs/openapi/openapi.json`；前端端点函数与类型全部由 `@umijs/openapi` 生成，禁止手写端点请求。
- `docs/` 保存 PRD、Design、Plan、Acceptance、Operations。历史实现从 Git 查询，不在工作树中归档。目标能力不得描述为已完成。
- 修改前阅读相关设计和测试。行为变化先验证失败，再实现；修复需针对实际故障验证。
- API 路由负责协议、认证和验证；业务服务负责事务；SQLAlchemy 模型负责持久化。禁止路由直接执行 SQL 或发布消息。
- Session 不跨线程或任务共享。同步数据库端点使用同步路由；进程拥有连接池，Celery 在 fork 后初始化。
- 业务服务只能直接导入本领域ORM模型；跨领域读取使用所属模块提供的函数/DTO，跨领域原子写显式传入同一Session。禁止为绕过边界建立全局repository或共享models目录。
- 顶层模块只在当前切片真实创建时登记；architecture测试不得预先白名单未来模块。新增模块必须先以失败测试证明未登记代码会被拒绝。
- Alembic 迁移随应用目录部署；应用启动不自动改表。不得修改已应用迁移，不导入不可信旧库。
- 业务状态与 Outbox 同事务提交。RabbitMQ 至少一次交付；epoch、fencing、租约和唯一约束保证幂等。Celery result backend 不保存业务事实。
- 唯一运行编排为根 Compose。生产差异使用两个 `-f` 文件叠加，无第二套服务栈。不得删除用户持久卷。
- 认证信息不入日志或 Git；配置使用 `HOTKEY_` 前缀。公开错误只含稳定代码和请求 ID。
- HTTP完成日志只记录request_id、方法、路由模板、状态码和耗时；禁止记录原始URL/query、请求/响应正文、Cookie、Token或连接字符串。未处理异常记录类型与堆栈，但不回显给客户端。
- 锁定依赖；运行 Ruff、mypy、pytest、OpenAPI 漂移检查及前端类型检查/构建。数据库和消息行为必须用真实 PostgreSQL/RabbitMQ 验证，UI 必须用浏览器验证。
- 只采集公开或获授权数据；平台连接器必须明确能力、分页、限流和失败状态。禁止把模拟数据、空结果或诊断任务当成采集成功。
- 不擅自提交、推送或删除远端引用；用户明确授权后审查差异、敏感信息和远端状态，使用中文 Conventional Commit。

## FastAPI 目录与命名（必须执行）

- 后端工程及 Compose HTTP 服务均为 `backend`，作为普通应用运行，不构建独立安装包；禁止恢复 `server/` 别名。部署入口为 `main:create_app`。
- 应用代码统一放在 `backend/src/`；禁止在 src 下增加 hotkey 或 app 包装层；`main.py` 只做应用工厂和 lifespan 装配；`api/router.py` 汇总路由，`api/routers/*.py` 按资源组织，依赖和 HTTP 横切逻辑分别在 dependencies.py、middleware.py、exception_handlers.py。
- `identity/`、`monitors/`、`jobs/` 拥有各自 models.py、schemas.py、services.py。models 定义 SQLAlchemy 持久结构，schemas 定义 Pydantic 契约，services 拥有事务和业务行为。禁止用通用 Workspace/BaseService 聚合无关领域；不为每个简单查询增加无意义仓储层。
- `core/` 只放配置、通用错误、输入输出基类和时间函数，不反向依赖业务模块。`db/` 拥有 DeclarativeBase、连接池及元数据注册；`audit/` 承载跨领域审计。`jobs/execution.py` 维护执行状态机，`worker/messaging.py` 对接 RabbitMQ；`worker/app.py` 装配 Celery，`cli/commands.py` 实现管理命令，`cli/__main__.py` 为命令入口。运行目录为 backend/src，Worker 入口为 worker.app:app。
- 路由禁止导入 SQLAlchemy、业务 models、services 实现、执行器或消息组件；只能通过 `api/dependencies.py` 注入服务。禁止经 request.app.state 在路由中绕过业务服务读写数据库或发布任务。服务、模型、Schema 不导入 FastAPI/Starlette/HTTP 路由；Schema 不导入 ORM 或数据库资源。
- 每个HTTP操作必须有唯一人工`operation_id`、tag、成功状态和Pydantic响应模型；错误响应按操作显式声明，不在应用级虚报所有状态码。输入继承严格Input并给集合、字符串、页大小和正文设置上限。现有透明cursor属于008登记债务，新接口不得复制扩散。
- Python 文件、目录、函数使用 snake_case，类使用 PascalCase，常量使用 UPPER_SNAKE_CASE；同类职责文件统一使用 models.py / schemas.py / services.py。绝对导入；`__init__.py` 仅标识包或说明包，不放业务代码和重导出别名。迁移 revision 文件属于已冻结历史，禁止为命名美观改写或重编号。
- 测试放 `backend/tests/unit/`、`backend/tests/integration/`、`backend/tests/architecture/`，公共 fixture 放 tests/conftest.py；独立容器验证脚本为 `backend/scripts/verify_*.py`，不得伪装成 pytest 测试。组件使用 PascalCase.tsx；生成客户端固定在 `frontend/src/api/`，Axios 传输封装固定在 `frontend/src/request.ts`，不创建 shared 层，英文 README 为 README.en.md。
- 结构与依赖方向由 architecture 测试强制检查；Ruff 检查命名/绝对导入，mypy 严格检查应用与脚本；锁文件、应用迁移、HTTP 和消息契约必须在目录重构中保持可验证。增加架构例外需同步 Design 和约束测试，禁止为让检查通过直接删除检查或添加宽泛忽略。
- 不因“异步更先进”将同步psycopg调用放进`async def`路由。只有整条调用链非阻塞且有独立并发/连接池验证时才引入AsyncSession，并保证每个并发task独立Session。

- `sources/` 的适配器不依赖 API、ORM、Worker 或 CLI；业务来源契约不导入 HTTP 客户端。来源探测只经独立 CLI 显式执行，查询预览不发送网络请求。未通过持久化采集验收前，来源连接状态保持 not_connected。
