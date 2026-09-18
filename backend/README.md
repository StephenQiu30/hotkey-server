# HotKey Backend

## 架构

采用模块化单体，按业务领域组织代码，在领域内区分协议、业务、契约和持久化职责。API、Worker 和 CLI 复用业务服务，各自独立运行。

| 职责 | 实现 | 边界 |
|---|---|---|
| HTTP 接口 | FastAPI APIRouter | 解析请求、身份校验、调用服务、声明响应模型 |
| 依赖装配 | FastAPI Depends | 注入服务与请求级资源，管理资源释放 |
| 业务服务 | `<domain>/services.py` | 业务规则、资源权限、用例编排、事务边界 |
| 输入输出 | `<domain>/schemas.py` | Pydantic 请求、响应与服务 DTO，不依赖 ORM |
| 持久化 | `<domain>/models.py` | SQLAlchemy 表、关系、索引与约束 |
| 数据查询 | Service；按需拆出 `<domain>/repositories.py` | 查询和持久化，不自行提交事务 |
| 外部系统 | `<domain>/adapters/` | 隔离 HTTP、对象存储和模型 SDK |

Router、Service 和 Repository 分层是项目约定；FastAPI 不强制 MVC、MVVM 或特定业务目录结构。

## 技术选型

| 技术 | 用途 |
|---|---|
| Python 3.12 + FastAPI + Uvicorn | 语言、HTTP 框架与 ASGI 服务 |
| Pydantic 2 + pydantic-settings | 输入输出校验、环境变量配置 |
| SQLAlchemy 2 + psycopg 3 | PostgreSQL ORM 和驱动，默认同步 Session |
| Alembic | 唯一数据库迁移工具 |
| PostgreSQL | 业务数据、执行状态、幂等记录和 Outbox |
| Redis | 缓存、限流、可重建临时状态 |
| Kafka | 持久任务事件与异步消费 |
| MinIO | 文件与证据存储 |
| Ruff + mypy + pytest + HTTPX | 格式与静态检查、类型、测试、HTTP 测试客户端 |

Kafka、Redis、MinIO 客户端及依赖精确版本在底座实现时锁定。只有完整非阻塞调用链才使用 AsyncSession；不混用同步 ORM 与异步路由。

## 目标目录

以下是实现时的目录约定；业务模块按实际切片创建，不预建空目录。Python 包包含 `__init__.py`，图中省略。

```text
backend/
├── pyproject.toml             # 依赖、Python 版本与工具配置
├── Dockerfile
├── alembic.ini
├── migrations/               # Alembic env.py 与 versions/
├── src/
│   ├── main.py               # create_app 与 lifespan 装配
│   ├── api/
│   │   ├── router.py         # 汇总 APIRouter
│   │   ├── dependencies.py   # Session、身份与服务注入
│   │   ├── middleware.py     # 请求 ID、访问日志
│   │   ├── exception_handlers.py
│   │   └── routers/
│   │       └── monitors.py   # 按资源划分 HTTP 接口
│   ├── core/
│   │   ├── config.py         # pydantic-settings，HOTKEY_ 前缀
│   │   ├── errors.py         # 与 HTTP 无关的业务异常
│   │   └── schemas.py        # 公共输入输出基类
│   ├── db/
│   │   ├── base.py           # 唯一 DeclarativeBase
│   │   ├── session.py        # Engine 与 Session 工厂
│   │   └── metadata.py       # Alembic 模型注册入口
│   ├── monitors/            # 监控领域示例
│   │   ├── models.py
│   │   ├── schemas.py
│   │   └── services.py
│   ├── identity/            # 身份领域，同样按职责组织
│   ├── jobs/                # 任务、Outbox、执行状态与恢复
│   ├── sources/             # 来源契约与 adapters/
│   ├── evidence/            # 证据数据与 MinIO adapters/
│   ├── worker/
│   │   ├── __main__.py      # python -m worker
│   │   ├── app.py           # 消费者生命周期与服务装配
│   │   └── messaging.py     # Kafka 传输
│   └── cli/
│       ├── __main__.py      # python -m cli
│       └── commands.py      # 运维命令
└── tests/
    ├── conftest.py
    ├── unit/
    ├── integration/
    └── architecture/
```

## 依赖与事务

1. 路由通过 `api/dependencies.py` 中的类型化依赖取得服务。路由不执行 SQL、不构造服务、不发布消息。
2. Service 可以调用本领域 ORM；复杂或重复查询可拆入本领域 `repositories.py`，不强制增加 Repository 接口和实现两套文件。
3. 跨领域调用使用所属领域的服务和 DTO，不直接操作其他领域 ORM；禁止循环依赖。
4. 最外层业务用例持有事务，成功提交、失败回滚。跨领域原子写传入同一 Session；被调用函数只读写或 flush，不独立 commit。
5. 依赖注入只负责创建与释放 Session，不在请求结束时隐式提交。ORM 对象在 Session 有效期内转换为响应 DTO。
6. 每个请求或任务使用独立 Session，不在线程或并发任务间共享。Engine 和连接池按进程创建，由 lifespan 或 Worker 生命周期释放。
7. Service、Schema、Model 不导入 FastAPI、Request、Depends 或 HTTPException；业务错误在 `api/exception_handlers.py` 映射为 HTTP 响应。
8. `core/`、`db/base.py` 和 `db/session.py` 不依赖业务模块；`db/metadata.py` 仅为迁移注册模型，不作为业务查询入口。

## HTTP 与后台任务

- 每个端点显式声明 `operation_id`、tag、成功响应和适用错误响应。输入与输出模型分离，敏感字段不得出现在响应 DTO。
- FastAPI 生成 `/openapi.json`；快照写入 `docs/openapi/openapi.json`，Umi OpenAPI 生成前端 `src/api/`。
- 持久任务由 Kafka Worker 执行。业务变更与 Outbox 同事务落库，提交后发布消息；消费幂等落库后提交连续完成位置的 offset。
- HTTP 与 Worker 是独立进程；不在 FastAPI lifespan 内启动 Kafka 业务消费循环。
- 连接池与客户端初始化失败时启动失败；存活检查只检查进程，就绪检查检查服务必需依赖。
- HTTP 日志只记录请求 ID、方法、路由模板、状态与耗时；公开错误不暴露异常堆栈和凭据。
- 同步 ORM 对应同步 `def` 路由。异步 Worker 调用同步业务时，将完整用例及 Session 生命周期放入受控线程，避免阻塞事件循环。

## 设计与交付

每个后端切片在 Design 阶段明确领域归属、变更路径、路由和 DTO、服务入口、事务所有者、跨领域依赖、消息与失败恢复行为。简单服务可直接使用函数；只在需要保存注入依赖时使用类，不建立通用 BaseService。

初始化完成后，从 `backend/src/` 运行 `uvicorn main:create_app --factory`；Worker 使用 `python -m worker`，CLI 使用 `python -m cli`。测试配置显式包含 `src` 导入路径。迁移独立执行，不在应用启动中自动改表。

验收覆盖业务规则单元测试、HTTP 与 OpenAPI 契约、真实隔离数据库和消息集成、依赖方向、迁移与资源释放。每次提交运行 Ruff、mypy 和对应 pytest；数据库及消息实现变更时验证事务回滚、重复消费和重启恢复。

当前仅有目录规范，尚未建立可运行后端和依赖锁文件。
