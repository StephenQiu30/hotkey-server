---
layer: Design
scope: backend
doc_no: "008"
title: FastAPI后端工程治理设计
status: accepted
version: v1.0
owner: HotKey Team
canonical_path: docs/design/008-FastAPI后端工程治理设计.md
prd: docs/prd/008-FastAPI后端工程治理.md
plan: docs/plans/008-FastAPI后端工程治理计划.md
---

# 008 FastAPI后端工程治理设计

## 1. 结论与适用范围

HotKey 当前后端已经具备可持续演进的模块化单体基础：单一应用工厂、APIRouter、Pydantic 契约、SQLAlchemy 事务、Alembic 迁移、Celery/RabbitMQ、OpenAPI 生成和分层测试均已存在。它不是需要套用某个互联网公司的目录模板后重写的项目。

本规范把“成熟团队标准”定义为可验证的工程属性，而不是公司名或目录层数：明确所有权、单向依赖、生命周期可关闭、事务原子性、契约稳定、安全默认、可观测和自动化门禁。规范适用于 `backend/src/`、`backend/tests/`、后端容器与后端生成的 OpenAPI；业务范围继续由006/007文档治理。

规范词义：**必须**是合入门禁，**应该**是存在明确收益的默认选择，偏离时要在Design记录理由，**可以**是按实际复杂度选用。历史迁移不可为满足新风格而改写。

## 2. 依据与边界

采用一手资料建立基线，并按本项目约束裁剪：

- [FastAPI Bigger Applications](https://fastapi.tiangolo.com/tutorial/bigger-applications/)：使用APIRouter拆分较大应用并集中依赖。
- [FastAPI Lifespan](https://fastapi.tiangolo.com/advanced/events/)：共享资源在lifespan中创建并在退出时释放。
- [FastAPI Dependencies with yield](https://fastapi.tiangolo.com/tutorial/dependencies/dependencies-with-yield/)：需要请求后清理的资源用yield依赖表达。
- [FastAPI Response Model](https://fastapi.tiangolo.com/tutorial/response-model/)：响应模型用于校验、OpenAPI和输出字段过滤。
- [SQLAlchemy Session Basics](https://docs.sqlalchemy.org/en/20/orm/session_basics.html)：同步Session每线程/执行单元独立，以上下文管理器固定事务边界。
- [Alembic Autogenerate](https://alembic.sqlalchemy.org/en/latest/autogenerate.html)：自动生成只是候选，必须人工审查；`alembic check`/metadata比较可作为漂移门禁。
- [Celery Tasks](https://docs.celeryq.dev/en/stable/userguide/tasks.html)：`acks_late`可能重复执行，任务必须幂等。
- [OWASP API Security Top 10 2023](https://api-security.owasp.org/editions/2023/en/0x00-header/)：对象/功能授权、资源消耗、SSRF、安全配置、API清单和第三方输入是设计检查项。
- [Google AIP-121](https://google.aip.dev/121)与[AIP-158](https://google.aip.dev/158)：资源化接口、稳定分页和不暴露数据库结构作为行业参考；不机械复制RPC命名。

FastAPI 官方没有唯一的生产目录。外部指南只提供原则，`AGENTS.md`和本文才是HotKey的规范事实源；冲突时先修改Design和门禁，不在代码里暗设例外。

## 3. 模块与依赖

```text
main/lifespan -> api router/dependencies -> domain services -> own models
                                            |             -> domain contracts/services
worker/cli ---------------------------------+
adapters -> contracts/schemas
migrations -> db.metadata -> all domain models
```

1. `main.py`是唯一FastAPI构造点，只做Settings、共享资源、middleware、exception handler和router装配。
2. `api/routers`只处理HTTP协议、认证依赖、输入和输出，不导入ORM模型、SQLAlchemy、Celery或业务服务实现。
3. 业务模块拥有自己的`models.py / schemas.py / services.py`。服务不得导入其他业务模块的ORM模型；跨域协作通过函数、DTO或契约完成，并在需要原子提交时显式传入同一Session。
4. `sources/adapters`和`evidence/adapters`只实现外部协议，不获得业务数据库、API或Worker依赖。
5. 新顶层模块必须在实际切片中创建并同步登记。架构测试不得预先白名单尚不存在的`ai/analysis/knowledge/notifications`等未来名字。
6. 不默认增加repository、unit-of-work、container、manager、utils或base service。只有重复规则或可替换边界已经出现时才抽象。

## 4. 生命周期、并发与事务

- API连接池和共享无状态服务在FastAPI lifespan创建，关闭时释放；模块导入不得连接数据库、来源或对象存储。
- 当前psycopg/SQLAlchemy路径是同步I/O，因此数据库路由使用普通`def`，由FastAPI在线程池执行。不得在`async def`路由中调用同步数据库或阻塞SDK。
- 如未来出现真正异步的端到端路径，单独设计AsyncEngine/AsyncSession；AsyncSession每个并发task独立，不能与同步Session混用。
- 每个应用服务方法拥有一个有界Session上下文。写用`sessionmaker.begin()`自动提交/回滚；读用`sessionmaker()`并在退出时关闭。禁止跨请求、线程、Celery任务共享Session。
- 一个业务用例只能有一个外层提交者。跨模块写入接受调用方Session，不自行commit；业务状态、审计、Job和Outbox按用例要求同事务提交。
- Celery prefork子进程初始化各自连接池；消息只携带版本化的小型DTO和标识，不携带ORM对象、Session或凭据。

## 5. HTTP与OpenAPI契约

- 业务路由统一位于`/api`，健康检查除外。URL不携带`v1`等数字版本段；当前单用户MVP通过OpenAPI契约、生成客户端和同步发布管理破坏性变更，不为未发布客户端预设URL版本或兼容路由。`/api`只在`api/router.py`的聚合路由声明，资源路由只声明资源路径。路径以资源名为主；不能自然表示为CRUD的动作使用POST并保持全项目命名一致。
- 每个操作必须有唯一、人工命名的`operation_id`、tag、明确成功状态和Pydantic响应模型；204不得返回响应体。
- 输入模型继承严格Input，默认拒绝未知字段并限制字符串、集合、页大小、正文和幂等键。输出使用独立Schema，不直接返回ORM模型或泄漏内部字段。
- OpenAPI由FastAPI运行时生成，`docs/openapi/openapi.json`为可复现快照，前端调用由该快照生成。数据库模型不是客户端契约。
- 错误只返回稳定`code`与`request_id`。可预期业务冲突转为AppError；不得把任意IntegrityError都当成已知业务冲突后静默吞掉。
- 列表首版即有界分页。新公开接口应该使用不透明、URL安全的cursor；现有UUID或`timestamp|UUID`游标在私有v1中暂时兼容，公开前统一迁移并给出版本策略。
- 错误响应按操作显式声明并统一使用`ErrorView`；应用工厂不注入全局错误集合，避免Swagger为每个端点虚报无法产生的状态码。

## 6. 安全与配置

- Settings是唯一环境入口，使用`HOTKEY_`前缀；密钥使用SecretStr，错误隐藏输入。生产启动必须拒绝HTTP Origin、不安全Cookie和半配置存储。
- 身份验证、对象授权和功能授权分别验证；存在ID不代表调用者有权操作。当前单owner不等价于未来多租户授权已经完成。
- 写请求校验精确Origin和会话绑定CSRF；会话Cookie保持HttpOnly/Secure/SameSite，CSRF Cookie可由浏览器读取但不得含会话令牌。
- 请求体、分页、外部节点、重试、任务时限和每日请求均有硬上限。公网速率限制、TLS终止和WAF由入口层负责，但必须在上线验收中实测。
- 外部URL、跳转、DNS和第三方正文均不可信。来源适配器执行SSRF/用途/权限门禁，日志和消息不包含Cookie、Authorization、正文或完整查询参数。

## 7. 可观测性与错误

- 每个HTTP请求分配`request_id`并回传响应头；完成日志至少包含request_id、HTTP方法、路由模板、状态码和耗时。
- 日志记录路由模板而不是原始URL，不记录query、request/response body、Cookie、Token或数据库连接字符串。
- 未处理异常记录异常类型和堆栈；外部响应仍使用稳定错误码。健康、业务作业、来源新鲜度是不同信号。
- `/health/live`只表示进程可服务；`/health/ready`只表示数据库可达且schema revision匹配。不得据此声称RabbitMQ、Worker、MinIO或来源健康。
- 分布式trace、metrics和告警阈值属于部署能力；引入前先定义业务指标和接收端，不为了“像大厂”增加无消费者的SDK。

## 8. 数据库、迁移与任务

- PostgreSQL是业务事实源。ORM模型与迁移必须一致，数据库约束兜底状态机、唯一性和引用完整性。
- Alembic只追加revision；应用启动不自动迁移。发布先运行独立migrate角色，API readiness核对唯一head。
- 自动生成迁移必须人工审查重命名、约束、数据回填、锁和降级影响。CI在一次性真实PostgreSQL上从空库升级并比较metadata。
- RabbitMQ负责传输，Job/Attempt/Outbox负责业务状态。`acks_late`、worker丢失和重复投递由幂等键、租约、epoch、fencing和唯一约束处理。
- 外部调用设置连接、读取、总时限与资源预算；Celery硬时限只作最后保护，不替代客户端超时与取消语义。

## 9. 测试与合入门禁

每次后端变更至少执行Ruff format/check、mypy strict、unit、architecture和OpenAPI drift。涉及数据库、迁移、事务或消息时必须使用可丢弃的真实PostgreSQL/RabbitMQ；涉及容器生命周期运行Compose验证；涉及用户界面运行浏览器主链。

架构测试必须扫描全部Python源码并对未知顶层目录失败，至少强制：唯一FastAPI构造点、无隐藏`__init__`逻辑、无相对导入、路由不访问ORM/消息、service不反向依赖API、service不跨域导入ORM、adapter不访问数据库、导入图无环、Alembic唯一head与readiness一致。

测试通过只证明对应边界。skip的集成测试、未运行的真实来源、现有MinIO、TLS、备份恢复、性能和持续运行必须在Acceptance中分别列出。

## 10. 当前审计与决策

| 项目 | 结论 | 处理 |
|---|---|---|
| 工厂、router、lifespan | 符合 | 保持单一`main:create_app`和关闭连接池 |
| 同步I/O并发模型 | 符合 | 固定同步路由+独立Session，纠正006早期AsyncSession旧描述 |
| 事务与任务幂等 | 符合当前设计 | 保持service事务和Job/Outbox/fencing真实集成测试 |
| 模块依赖 | 部分符合 | 已消除两处跨域ORM导入并新增门禁；移除未来模块预白名单 |
| API契约 | 符合当前私有v1 | operation_id、响应模型、按操作错误声明和生成客户端已有；透明cursor列为公开前债务 |
| 安全边界 | 本地基线符合 | 公网入口限流、TLS、备份和多租户授权未验收 |
| 可观测性 | 原先不足 | 已增加脱敏HTTP完成日志；metrics/trace按真实部署再接入 |
| 测试与CI | 较完整 | 本地/CI证据与真实外部能力继续分开报告 |

## 11. 非目标与演进触发条件

本轮不切换异步SQLAlchemy、不引入Prisma/Django ORM、不把模块化单体拆微服务、不增加通用repository/DI框架、不建设无接收端的APM。出现可量化的线程池瓶颈、独立伸缩/权限边界、两个真实实现需要替换或公开API兼容需求时，再以Design变更触发对应演进。
