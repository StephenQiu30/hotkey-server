# HotKey Server 交接

更新日期：2026-09-24。

## 本机 PostgreSQL 开发库

2026-09-24：复用已运行的本机 PostgreSQL `127.0.0.1:5432`，仅新建 `hotkey-server` 数据库并应用唯一 `backend/database/schema.sql`；31 张表与 SQLAlchemy 元数据表名完全一致。忽略提交的 `backend/.env` 已指向该库，示例配置中的数据库名也已固定为 `hotkey-server`；不配置 `HOTKEY_TEST_DATABASE_URL`，避免集成测试清表逻辑碰到运行数据库。原 `hotkey_dev` 未改动，没有启动 Compose PostgreSQL 或第二套依赖；现有 API 原位重启后 `/api/ready` 为 200，Web 未重启且首页为 200。

## 001 核心服务能力规划

2026-09-23：001 Design v1.2 增加首版能力链“监控/账号配置 → 持久任务与采集 → 事件研究 → 证据化分析/修订 → 变化与成果交付”，并明确各步的事实 owner、子 Plan 映射与身份/预算/幂等/审计/生命周期横切保护。没有新增代码目录、服务栈、依赖或产品需求。Design 仍为 proposed：首轮样本、真实来源访问条件、实际资源与 B0/Q0 仍待冻结；服务能力图可指导与外部来源无关的内部切片，但不是实现/验收完成证据。

## X 官方 API 离线接入 S01a

006 S00 与 X 身份解析离线子片（2026-09-23）：新增同编号 Design 和仅接受 `MockTransport` 的官方单用户名 Lookup；本地从 `@handle` 或精确 X 主页提取用户名，请求前需批准最坏 1 个 User，返回严格校验稳定数字 ID、用户名与展示名，失败按未知用量回调。同名显示名不同 ID 与同 ID 改名受控样本均不自动改绑。专用 36 tests，后端无隔离数据库配置的全量 320 passed/174 skipped，Ruff/format/mypy 通过。尚无关注持久化/API/Web、真实费用装配、App/Token、实际 X 请求或产品 AC；006 保持 in_progress、0/6 AC。

2026-09-23 补充按已确认稳定 ID 的离线回查：`GET /2/users/{id}` 只经 `MockTransport`，响应 ID 必须匹配；非法 ID 不消耗预算、不请求，改名可更新别名但旧用户名被他人占用不能改绑。专用 45 tests，后端全量 329 passed/174 skipped、Ruff/format/mypy 通过。仍无真实 X 请求、关注持久化或产品 AC；用户当前要求优先规划 HotKey 顶层服务能力，不继续扩展 X 接口实现。

2026-09-23 用户将 X 自动采集从网页登录/twscrape 改为官方 API；目前尚无开发者 App/Token，要求先离线实现。002 S01a 已在 `sources/adapters/x_api.py` 加入仅接收 `MockTransport` 的 Recent Search 只读适配器，覆盖 recency/relevancy、分页、作品字段、合法空和认证/限流/协议失败，复用共享 SourcePage；旧 `x_twscrape.py` 不注册 Worker，也不读取现有浏览器登录。15 项模拟 HTTP 测试、后端全量 227 passed/161 skipped（未设置集成/live 测试条件）、Ruff/format/mypy 通过；没有真实 X 请求、额外服务或脚本。

同日离线费用预留接口补片：移除可能产生额外 User 资源的作者展开；每页在模拟发送前必须授权最大 Post 数，完整响应回报实际数，失败/取消/意外扩展资源回报未知供后续保守结算。新片先红 20 项，额外扩展资源回归另有 1 项先红，最终 23 项通过；后端全量 235 passed/161 skipped，Ruff/format/mypy 通过。未改 DDL、Worker、API 或运行进程；这不是持久费用门禁，现有 `paid` 拒绝仍生效。

同日按官方 Recent Search 端点/OpenAPI 修正 `post.fields`：移除不在端点允许值内的 `referenced_posts`，不为获取关系数据启用可能产生额外资源的扩展；字段越界模拟 400 先红后绿，缺作者 ID 仍按协议失败及未知费用处理。25 项 X 离线单测、一次性空库后端全量 407 passed/5 skipped、Ruff/format/mypy 通过；未发真实 X 请求，作者与关系字段的真实返回仍待具备 App/Token 和费用上限后验证。

同日为共享搜索请求增加可选 UTC 起止时间；X Recent Search 两页模拟请求均传相同 `start_time`/`end_time`，超出最近 7 天、未来及被非校验复制的无效窗口在费用授权前拒绝。失败优先回归、来源契约与 X 单测 44 项、一次性空库全量 420 passed/5 skipped，Ruff/format/mypy 通过。此参数不等于平台终点证明，010 水位与旧于 7 天的回补不因此完成；无 X 实调、Worker、第二套服务或新增脚本。

同日补齐离线总时限：模拟传输跨过适配器累计截止后即使返回有效 HTTP 200，也只能停止而不能产出完整页，已授权尝试按未知 Post 数结算。目标测试先红后绿，X 单测 32 项、当前未配置集成环境的后端全量 257 passed/170 skipped、Ruff/format/mypy 通过；未运行真实 X 或启动依赖。此修正不保证阻塞中的单次底层读取被硬中断，不改变 S01b 费用/凭据门禁和 0/10 产品 AC。

同日按官方作者时间线端点补离线 `author_posts`：已有共享作者请求通过 `/2/users/{id}/tweets` 的 MockTransport 翻页，逐页沿用最大 Post 数授权和实际/未知结算；路径作者补足未返回的作者字段，明示错归属为协议失败。旧适配器对作者请求 `unsupported` 的目标用例及非校验复制的超长/字符串页长用例先红，最终 X 单测 41 passed、隔离 PostgreSQL 后端全量 435 passed/5 skipped，Ruff/format/mypy 通过。没有真实 X、Worker/HTTP/DDL/代理变更，未证明作者作品关系或产品验收；App/Token、账期上限及 037 付费门禁仍缺，002 AC 保持 0/10。

同日按官方 `in_reply_to_tweet_id:` 补一级/嵌套直接回复离线路径：复用 Recent Search 和共享评论请求，每次查询一个父帖的直接子回复；根帖/父回复身份冲突、未知根帖和非法 ID 均拒绝，不用会话搜索结果猜父链。旧适配器 4 项目标失败，最终 X 单测 49 passed、隔离 PostgreSQL 后端全量 443 passed/5 skipped，Ruff/format/mypy 通过。MockTransport 以外仍不可用，不证明逐层遍历、旧于 7 天的回复或平台完整性；App/Token、账期上限、037 付费门禁、S02 实采与 002 产品 0/10 AC 均待执行。

同日补 027 S01 / 002 S01a 的 X 离线身份预检：6 组坏 ID/关系目标/互斥引用响应中，旧实现 5 组误报完整；现在整页协议失败、未知 Post 数保守结算。X 单测 55 passed、现有 PostgreSQL 一次性空 QA 库后端全量 449 passed/5 skipped、Ruff/format/mypy 通过。`sources` 纯映射和测试以外无运行变化，QA 库验后删除；不发真实 X 请求。官方端点对查询 ID 的位数约束外推至响应对象仍须实调核对；`post.fields` 与 Quickstart 的 `tweet.fields=author_id` 文档冲突未解除，不放行付费门禁，不宣称 027 父链持久化或 002 真实接入。

005 S01 内部执行器原先从 Job 读取 UTC 范围，却向适配器发送无界 `SearchRequest`；隔离 PostgreSQL 用例先复现 Latest 续页 `(None, None)`，修复后 Latest/Top 均逐页携带冻结窗口。专用 2 项、一次性空库后端全量 420 passed/5 skipped 及 Ruff/format/mypy 通过；没有接 X 付费回调或注册 Worker/HTTP，不能据此确认真实来源终点或完成 005/010 产品验收。

随后收紧公开任务请求：`POST /api/jobs` 只接受 Worker 已注册的 `webpage.collect`，历史 `monitor.collect` 不再能被公开 API 假受理。运行时 OpenAPI 和实际 FastAPI 422 拒绝测试通过；后端全量 331 passed/181 skipped、Ruff/format/mypy，前端 42 tests/lint/typecheck/format/build 与运行时 `pnpm openapi:check` 通过。数据库集成测试因未设置专用 `HOTKEY_TEST_DATABASE_URL` 跳过，未使用开发库。关键词 Worker/API/结果入口、合法来源、S01 G3/G4 与产品 AC 0/6 仍未关闭。

2026-09-24 完成 005 S02a 本地规则筛选及 S02b 命中依据预览内部技术子片。S02a 由 `MonitorTopicService` 读取 owner 的精确不可变版本，并复用 NFKC/casefold 规则筛除无关项；S02b 扩展 evaluator 与 `/api/topics/preview`、生成客户端及现有 UI，展示实际命中的 any/all/exclude 词项。S02b 目标 PostgreSQL API 测试 1 passed、后端全量 333 passed/181 skipped，Ruff/format/mypy，前端 43 tests/lint/format/typecheck/build 与 OpenAPI 生成通过；真实 API＋Web 的隔离 owner 浏览器验证覆盖 1440×900、390×844、焦点循环与 0 个 axe violations。临时 QA 库已确认无连接后删除，业务库未迁移；无 DDL、依赖、脚本或来源请求。S01 完整门禁、S02 搜索结果闭环、真实来源及全部产品 AC 仍未关闭。既有有界多页/预算停止/零页失败已由代码和测试覆盖，不重复建设。

033 S00 已接受仅限内部状态分项的 Design：现有 039 运行快照按 owner、来源和能力分别汇总七态。同一 X 搜索能力一成一败、X 评论成功及另一来源部分成功可同时保留；无来源任务只进总数。隔离 PostgreSQL 用例先红后绿，专用 6 项、一次性空库后端全量 421 passed/5 skipped 与 Ruff/format/mypy 通过。当前 Worker 仍为单循环，未证明资源公平、熔断或真实局部故障恢复；033 S01 G3/G4 与全部产品 AC 未关闭。

002/005/037 及 BACKLOG 已登记费用例外：X 的 App/Token 和控制台账期美元上限未确认，037 当前数据库仍拒绝 paid 核心组件，因此不能挂入 Worker、发起付费请求或把 002/005/037 产品 AC 标为通过。下一步依序完成 037 的完整 paid 准入、账期确认与请求/费用原子预留，004/034 的秘密连接，再验证 002 的真实搜索/作者/回复能力及接入 005 处理器。其他来源和基础分析继续免费/自建；不自动充值或切换网页登录路径。

## 本地网页与浏览器采集 S01 / S02 / S03-T00—T03

已建立 [047 Research](docs/research/047-本地网页与浏览器采集调研.md)、[047 Design](docs/design/047-本地网页与浏览器采集设计.md) 与 [047 Plan](docs/plans/047-本地网页与浏览器采集计划.md)。Plan 为 `in_progress`，G1/G2/G3/G4-001 已关闭，G4-002 未关闭，产品 AC 仍为 0/8。`web` 已具备无凭据版本、精确允许域名、collector_call、类型化 API→Outbox→Kafka→Worker→Firecrawl→webpage 原子结果与恢复；现有内容页可提交 URL，任务页可取消、手动重试、显示部分状态并打开持久资料。S03-T03 已提供共享 Worker 单任务监督技术证据：硬截止、取消资源结清、未知退出后 lease 重放，并在现有 Browser 服务验证 renderer/context 随取消释放；Browser 业务处理器及完整凭据上下文/换版拒写端到端矩阵仍未接入。2026-09-24 官方公开资料复核记录四个平台各自的权限、费用或书面许可门槛；未发平台请求，也未启用平台会话。四平台评论均为必需，后续按真实准入和样本逐平台交付。

2026-09-24 Firecrawl 复验：现有服务对 `example.com` 返回 HTTP 500 `SCRAPE_RETRY_LIMIT`/`document_antibot`，HotKey 现在将该结构化失败映射为 `access_denied`；无页面正文，故不宣称采集成功，047 产品 AC 仍 0/8。后端 347 passed/187 skipped、Ruff/format/mypy 通过（DB/Kafka 隔离集成项跳过）；未写 `hotkey-server`、重启现有进程或启动第二套依赖。

2026-09-24 深入只读诊断：Firecrawl readiness 与 Playwright health 为 200，但受控 `example.com` 仍无正文；内部 Browser `/scrape` 的页面状态为 403（安全分类 `private_block`）。出口代理 CONNECT 与 DoH 探针成功，运行时解析的目标地址为公网。只读代码审查发现出口路径与既有代理策略的一致性仍需安全复核；实现路径及复现细节不在公开交接文档披露，且没有证据证明该疑点导致当前 403。保持 SSRF 与目标白名单，出口路径修复需先形成安全设计/获授权；本轮未改代码、数据库或服务，也未重启容器。

调研发现小红书 0 值配置文档与源码不一致、微博候选缺楼中楼继续分页、opencli B站评论仅单页；MediaCrawler 的置顶漏采已修复但许可证限制仍需遵循，Nemo2011/bilibili-api 已关停。S03-T00 在相同受控页面比较 Scrapling 0.4.15 与直接 Playwright：两者均可展开并保留字符串 ID，但 Scrapling 动作失败/超时仍返回 200、自适应误认另一评论 ID，默认记录完整 URL。选用 Playwright 1.63.0 原生 WS 单浏览器拓扑，不引入 Scrapling/Selector/CDP。S03-T01 已构建非 root、sandbox、只读、内部 WS 且默认断网的 browser；Worker 一次性容器的无网络探针成功，公网/宿主数据库端点不可达。Linux-arm64 Docker Desktop 证据不代替生产主机或真实平台验收。

S01 已增加独立 `page_content` 文档契约、精确域名/默认端口目标约束、固定 `firecrawl/2.11.162` HTTPX 适配器、配置及 DDL 能力同步；`python -m cli sources probe-webpage` 只做显式、无持久化诊断，不输出目标和正文。独立 Firecrawl `main` 的 `6d9fb16` 完成中央日志脱敏、Playwright 原始请求日志移除和出站目标加固，只原位恢复既有两个容器并复用宿主依赖。HotKey CLI 的普通页 180 字符、动态页 1574 字符、私网拒绝和日志 query 标记 0 命中通过。S02 已将类型化 `webpage.collect` 接入真实 Kafka/Worker/Firecrawl，调用前锁定租约、连接/域名、政策/保留和预算，调用后结算 collector attempt，并把正文/观察/发现、生命周期、能力证据和 checkpoint 原子提交；任务状态暴露 `result_content_id`。真实 PostgreSQL 用例覆盖消息重放、换版、限流、未知 kind、partial、结算前与页提交后中断恢复，显式 live Kafka 链实际读取当前 Firecrawl 的 `example.com`。内容页新增 URL 表单，任务页可取消、手动重试、显示部分状态并打开结果；桌面/390×844、双击单请求、operation ID 重试、CSRF 请求头和 axe 0 violation 已验证。此前后端全量 310 tests、前端 35 tests 及静态/契约/构建通过。尚无会话、真实平台评论证据或 Acceptance。

S03-T02 的受控动态交互与离线状态子片已用现有断网 browser 内网通过 2 项 live 测试：内存页面输入、滚动、展开、有限页结束、无限分页显式上限；状态在临时 0700 目录以 owner/连接/版本不可变保存并显式加载到新 context，未传状态的新 context 不继承 Cookie/localStorage。状态存储单测覆盖 0600 文件、符号链接/宽权限/跨 owner 拒绝；没有永久平台秘密、新服务或真实平台请求。

S03-T02 连接执行门禁又增加 `browser_state` DDL/ORM 精确引用约束与事务内当前版本、停用、认证失效、缺文件拒绝。在现有 PostgreSQL 的一次性空库应用完整 `schema.sql` 后，真实集成及后端全量 320 passed/2 skipped，Ruff/mypy 通过；库随验证删除并确认临时库 0 个，既有开发/测试库未改动。此实现仍无平台目录、人工登录/换版 CLI 或业务任务调用，不能当作真实会话可用。

S03-T02 又为每次 BrowserContext 交互设置默认及最大 45 秒截止；现有 browser 内网 live 测试增至 3 项，超时、主动取消后页面关闭且可重新建 context。一次性 Worker 客户端持有页面时 browser 渲染进程为 1，`SIGKILL` 后回到 0，browser 保持 healthy，测试容器自动移除；这只证明本机渲染进程回收，不证明业务任务崩溃恢复。计时不包含连接/清理。人工登录导出拟复用 Playwright 官方 `codegen --save-storage`，仍未接入真实平台或人工登录采集命令。

S03-T02 现有 `browser_state` 连接维护 CLI 已落地：可从 0600 捕获文件换版，或按当前版本停用；文件/父目录权限、符号链接、当前版本、重复停用、再启用及输出脱敏由受控测试覆盖。复用既有 PostgreSQL 的一次性空库执行后端全量 325 passed/3 skipped，Ruff/mypy 通过，临时库删除；未改现有开发/测试库或启动第二套依赖。CLI 不创建首个真实平台连接，未执行平台人工登录、业务 Worker 或产品验收；G4-002/EV-047-005、AC 0/8 不变。

S03-T02 又将 context 创建纳入原有 45 秒协作式截止，关闭 context/连接分别以 5 秒等待预算请求取消，前者超时仍尝试后者。失败优先单测、现有 browser 内网 live 超时关闭与重连、一次性空库后端全量 328 passed/3 skipped、Ruff/mypy 通过；未启动第二套依赖。Python `wait_for` 可能等待吞没取消的协程，且 WS 建连/管理器退出与业务任务总时限未覆盖；G4-002/EV-047-005、产品 AC 0/8 仍未关闭。

S03-T02 再将 WS 建连纳入同一次默认/最大 45 秒协作式截止；旧实现的慢连接失败测试、11 项浏览器单测、现有 browser 内网 3 项 live 和一次性空库后端全量 329 passed/3 skipped 通过，Ruff/mypy 通过，测试库删除。Playwright 管理器进入/退出、关闭与业务 Job 总时限仍未覆盖；真实平台登录、出站边界、G4-002/EV-047-005 和产品 AC 0/8 不变。

S03 控制面已从公开根路径切换到本机私有、含 48 个随机十六进制字符的 `/ws/` 路径：根 Compose 将同一 URL 注入现有 browser 与一次性 Worker 客户端，Python 配置以 `SecretStr` 脱敏，browser 缺配置/弱路径拒绝启动。原位重建后 CLI 探针、旧路径拒绝及现有代理/动态交互/状态/取消共 5 项 live 通过；一次性空库后端全量 329 passed/5 skipped、Ruff/format/mypy 通过，库已删除。密钥仅在忽略 Git 的 0600 根 `.env`，未启动第二套依赖服务。真实平台会话、业务任务总截止/崩溃恢复与 G4-002/EV-047-005、产品 AC 0/8 仍待后续。

S03-T02 管理器启动现与 WS/context/交互共用 45 秒协作式截止，退出单独以 5 秒请求清理；即使前面关闭失败，也尝试停止本地 Playwright 驱动。两个失败优先单测、13 项浏览器单测、现有 browser/代理 5 项 live、一次性空库后端全量 331 passed/5 skipped 及 Ruff/format/mypy 通过，临时库删除。此限时不抗取消吞没，也不是业务任务硬截止；真实平台会话及 G4-002/EV-047-005、产品 AC 0/8 仍未完成。

2026-09-24 更新：S03-T03 已在共享 Worker 层实现并验证 spawn 子进程任务总截止、terminate→kill 回收、取消后的 usage/Job/inbox 事务结清，以及未知退出不提交 offset 并等待 lease 重放；PostgreSQL/Kafka 子片、后端全量及现有 Browser renderer 取消释放均通过。此共享能力不等于 047 Browser 业务处理器已接入，也不关闭 G4-002。四平台官方准入复核仅为文档证据：抖音权限/用户授权、微博认证/付费、小红书暂无公开评论 scope、B站需书面许可；真实会话、样本、S04/S05 和产品 0/8 AC 仍待完成。

047 S04/008 评论所需的 [010 增量与历史回补 Design](docs/design/010-增量更新与历史回补设计.md) 已接受来源无关的内部契约，Plan 保持 in_progress，S00 G0—G2 与 S01 G3/G4 内部门禁已关闭：`coverage_windows` 的 owner/范围唯一、部分缺口、连续确认及最近 Job 复合外键已与唯一 DDL/ORM 同步；`ContentService.persist_post_in_transaction` 允许受控分页中的作品观察、发现、范围状态和 Job checkpoint 同一事务。两轮重叠得到 1 个作品、2 条发现、2 次观察；前窗未确认时水位不越过缺口，回滚不留进度。一次性空库 342 passed/5 skipped 和静态门禁通过，临时库已删除；无真实多页来源样本、API/UI 或既有开发库迁移，不设置通用 10 分钟重叠或旧帖刷新周期。产品 0/6 AC、Acceptance 均未变化。

010 S02 时间标记子片已推进：范围 Job 必须显式标识 `new_scan|refresh|backfill`，旧 Job 类型缺失时保守返回 `null`；作品发现详情通过运行时 OpenAPI/生成客户端显示历史回补标签，原始发布时间与本次观察时间继续分列。隔离 PostgreSQL 全量 343 passed/5 skipped，前端 36 tests、静态/构建及桌面/390px 受控响应（axe 0 violation）通过；本轮未修改 DDL、未迁移现有开发库、未新建依赖服务。受控浏览器响应不代表真实来源；游标有限重扫、实际回补和通知抑制尚未实现，S02 G3/G4 与产品 0/6 AC 仍未关闭。

031 S03 状态校准（2026-09-24）：009 已实现 `JobExecutionFailure` 类型化分类、有限重试、终态失败与同事务 inbox；047 已实现 Worker 子进程监督与未知崩溃重放。禁止宽泛捕获异常后确认消息；分类终态记录不另建 Kafka DLT。用户确认逻辑 Job 从计划到期/即时提交计至 `completed_at`、包含队列和重试，并接受保守故障归属：只有同一 Job 的持久来源失败证据才列来源结果，其余列“未归属”并保守计为失败。`webpage.collect` SLA 已冻结为 30 分钟；只读汇总按 SLA 截止归入半开观察窗，完整成功和按期持久来源失败进入分子，恰好 30 分钟算按期，空分母比率未定义。T03 复核未发现重复 HTTP/DDL/客户端路径，补充 CLI 无 owner 失败关闭回归；定向测试 15 passed，隔离 backend CI [#35972829857](https://github.com/StephenQiu30/hotkey-server/actions/runs/35972829857) 为 549 passed/12 skipped，Ruff/format/mypy、[contract](https://github.com/StephenQiu30/hotkey-server/actions/runs/35972829872) 与 [runtime](https://github.com/StephenQiu30/hotkey-server/actions/runs/35972829859) 均通过。`/api/ready` 返回 200；同一后端配置的只读查询确认唯一 `hotkey-server` 数据库 schema 存在但 owner_count=0、job_count=0，CLI 因此以 `identity_uninitialized` 失败关闭，当前没有 HotKey Worker 进程。真实 72 小时观察尚未开始，需先由用户初始化 owner、授权启动现有 Worker，并提供获准的真实工作负载；T01/G3 未全闭环，G4 与产品 AC 保持未通过。

## 当前结构

- `backend/`：Python 3.12、FastAPI、SQLAlchemy 2、PostgreSQL、Redis、Kafka；底座可运行，依赖由 uv 锁定，数据库 DDL 由单一事务化 SQL 文件管理。
- `frontend/`：Next.js App Router、shadcn/ui、Radix UI、Tailwind CSS、Axios；工程可构建。
- `hotkey-app/`：独立 Flutter 客户端仓库。

前端页面位于 `src/app/`。页面专属组件放在所属路由的 `components/`，跨页面复用组件按功能领域放在 `src/components/<feature>/`，shadcn 组件放在 `src/components/ui/`。不使用 `features`、`common`、`patterns`、`shared` 或 `scripts` 目录。

## 前端基础

- `src/request.ts` 是唯一 Axios 请求封装，统一处理凭据、超时、响应数据和错误；业务 HTTP、网络、超时、取消及协议错误保持可判别。
- Umi OpenAPI 读取后端自动生成的 `/openapi.json`，生成文件直接写入 `src/api/`；命令环境变量 `HOTKEY_OPENAPI_URL` 可覆盖默认地址。
- `src/proxy.ts` 处理页面 CSP nonce，并对缺少会话 Cookie 的 `/events` 做乐观登录跳转；`src/app/api/[[...path]]/route.ts` 负责同源 `/api/*` 转发及可控 502/504 错误契约，API 仍是认证和授权边界。
- 页面采用组件优先的无边框设计，只使用 Tailwind 命名尺度及 `sm/md/lg/xl/2xl`。
- App Router 已配置 loading、error、global-error、not-found 和 `/health`。
- Dockerfile 定义 standalone、非 root 用户和健康检查；根 Compose 已用只读文件系统及 tmpfs 实际验证。

## 后端基础

- `app/main.py` 是唯一 FastAPI 应用工厂，lifespan 管理 SQLAlchemy Engine 与 Session 工厂。
- `app/api/` 统一管理路由、依赖、中间件、异常与文档；提供 `/api/health`、数据库 `/api/ready`、`/openapi.json`、`/docs` 和 `/scalar`。
- `app/core/` 管理 `HOTKEY_` 配置、结构化日志、公共错误和 Pydantic 基类；`app/db/` 管理唯一 DeclarativeBase、Engine、Session 和运行时模型元数据。
- `database/schema.sql` 是唯一数据库结构事实源，仅用于全新空库。项目没有 Alembic、revision 目录或应用启动建表逻辑。
- 业务领域目录不提前创建空包；当前实际领域为 `identity`、`jobs`、`sources`、`evidence`、`backups`、`monitors`、`connections`、`content`，其余 `ai`、`audit` 等候选主责遵循 `PROJECT.md`，具体切片落地时再创建实际文件。
- `python -m worker` 是 Kafka Worker 入口。已有 outbox 发布、手动 offset、inbox、租约/检查点恢复的运行装配；尚无具体业务消息处理器时安全退出，不订阅或提交任何消息。处理器只能在对应任务设计完成后注册。
- `python -m cli` 是 Typer 管理入口。`tests/unit`、`tests/integration`、`tests/architecture` 分别承载规则、HTTP 契约和依赖边界验证。
- `app/identity/` 已实现单 owner 初始化、Argon2 密码散列、服务端不透明会话、CSRF、注销和维护恢复；`python -m cli identity reset-password` 从隐藏交互输入读取新密码并撤销全部旧会话。
- 身份 HTTP 契约为 `/api/identity/initialize`、`/api/identity/sessions` 与 `/api/identity/session`；Web 请求层自动为写请求补 CSRF，请求凭据和 Cookie 不进入生成客户端参数。
- `GET /api/identity/workspace` 从有效会话派生当前 owner，不接受客户端归属标识；`require_resource_owner` 为后续业务资源提供默认拒绝规则。Web 已有 `/login` 与受保护 `/events` 空工作台，尚未接入事件业务资源。
- `app/jobs/` 已实现内部持久受理与恢复：任务与 `job.accepted.v2` outbox 同事务写入，owner/kind/operation ID 唯一，绑定非敏感配置版本与来源能力，等价重试返回原任务，异范围重用拒绝；outbox 收到 Kafka 回执后才标记，消费者在数据库事务后手动提交 offset，inbox、lease epoch、attempt 和连续 checkpoint 防止重投与旧执行者覆盖，调度追赶默认最多 3 个窗口。009 已提供任务 HTTP、类型化失败、有限重试、到期 Outbox 与终态失败隔离；047 已登记 `webpage.collect` 处理器。阶段尝试与任务/Worker/资源尝试汇总可按 operation 核对；031 已冻结保守故障归属与 30 分钟 SLA，并通过隔离 CI 验证只读逻辑 Job 截止归窗/按期统计及管理 CLI JSON 输出（backend 549 passed/12 skipped，contract/runtime 全绿）。T03 复核未发现重复入口，并补充未初始化 owner 的失败关闭测试。同一配置只读查询确认 `hotkey-server` 库有 `jobs` schema，但 owner_count=0、job_count=0；CLI 因无 owner 返回 `identity_uninitialized`，当前无 HotKey Worker 进程，真实 72 小时观察仍未开始。
- `app/evidence/` 已实现内部来源访问与在线生命周期控制：owner/source/capability 唯一政策、原子换版、入库前白名单投影；结构化/原始/媒体保留取用户请求与来源上限的更严值，缩期立即作用于已追踪资源；删除或到期后默认拒绝读取，`python -m cli lifecycle cleanup-once` 以 PostgreSQL lease 和有限重试清理 Redis/MinIO 在线副本。`app/backups/` 复用证据 DTO 生成同快照数据库候选归档和 MinIO 引用清单；已有 X 受控适配器、Firecrawl 网页适配器/显式 CLI 与 `webpage.collect` 业务处理器，当前本地 env 已启用既有 Firecrawl。网页 UI、MinIO 内容备份、真实恢复/回补和社交平台授权证据仍未完成。
- 本机既有 PostgreSQL 数据库包含旧系统历史表，不符合当前完整 schema。不得对这些旧库执行 `database/schema.sql`；需要保留数据时先备份，再用新库完整建表并校验导入。

## 运行基线

- 根 `compose.yaml` 是唯一编排，固定 PostgreSQL 17.11、Redis 7.2.16、Kafka 4.1.2；API/Web 仅绑定本机端口，内部依赖不发布宿主端口。
- 当前工作站的未跟踪 `backend/.env` 与 `frontend/.env.local` 直接复用已启动的 Homebrew PostgreSQL 18.4、Redis、Kafka 和 MinIO，不再启动第二套 Compose 依赖。当前代码使用同一 PostgreSQL 服务内的新空库 `hotkey_dev`；旧 `hotkey`、`hotkey-server` 与 `hotkey_test` 数据库未改动。部署镜像仍以本文固定的 PostgreSQL 17.11 为基线，本机 18.4 验证不能替代部署态版本验证。
- `schema.sql` 自带事务边界，通过 PostgreSQL 官方初始化目录仅作用于全新空卷；没有初始化 `.sh`、迁移框架或第二份 DDL。
- `.github/workflows/runtime.yml` 构建镜像并验证真实依赖、API/Web/同源代理、空 Worker、资源快照及部署态 OpenAPI 漂移。
- 验证不增加 `scripts/` 工具文件；复用 Compose、依赖官方 CLI、curl、docker stats 与现有 pnpm 命令。

## 待完成

**[046 前置计划](docs/plans/046-全局异常与响应契约前置计划.md) 已完成并通过 Acceptance。** 未知异常请求标识、5xx/校验信息泄漏、OpenAPI 错误模型、Web 错误读取、同源代理失败及生成客户端漂移门禁均已修复和验证。后续接口继续复用该契约；B00 的外部来源条件继续登记，但不阻塞 B02 内部领域切片。

**[042 计划](docs/plans/042-容量与部署可重复性计划.md) S00/S01 已完成，B01 底座前置已关闭。** 本地隔离 Compose 验证五个长期服务 healthy，API/Web/代理/空 Worker 和部署态客户端生成通过；该结果不代表完整 B0、两干净环境、恢复或 042 的 0/6 产品 AC 已通过。

**[034 计划](docs/plans/034-凭据与应用安全计划.md) S00/S01 与 S02a 当前内容页链接安全技术子片已完成，Plan 保持 in_progress。** S00/S01 的隔离 PostgreSQL/Compose 验证覆盖受控初始化、登录/注销、CSRF、旧会话失效、维护恢复和日志不泄密；S02a 列表/详情共用 HTTP(S) 原文链接校验，前端 43 tests、lint/format/typecheck/build 及合成数据浏览器 DOM 验证通过。S02a 未改 API/DDL/依赖/env/服务；未覆盖 CSV 导出、模型/通知消费者与 Firecrawl 网络级 DNS/重定向。034 产品 AC 仍为 0/6，未建立 Acceptance。

**[035 计划](docs/plans/035-权限与数据隔离计划.md) S00/S01 已完成，S02 当前资源先行输出通过，Plan 保持 in_progress。** 在既有工作区授权上增加主题/任务越权操作矩阵、作品关联失败整笔回滚、连接证据隔离、删除目标引用不展开、混合证据清单整体拒绝及注销后拒绝。修复统一错误响应缺少 no-store 的缓存边界。真实 PostgreSQL 全量 243 tests、前端 22 tests、静态/契约及隔离构建通过；现有 API 已替换为当前代码，原 Web 代理确认禁存头与 request ID 透传，未新增服务/脚本。完整 S02 的评论/事件、实际缓存/异步发布/导出下载及 S03/S04 随业务接入，产品 AC 仍为 0/6，未建立 Acceptance。

**[031 计划](docs/plans/031-可靠执行与幂等计划.md) S00—S02 已完成，S03—S04 保持进行中。** 原子受理、outbox 重发、消费组中断/再均衡、手动 offset、inbox、租约 fencing、checkpoint 恢复、有限调度追赶和 Redis 不可用已有本机 PostgreSQL/Kafka 证据；009 已实现分类失败/有限重试，047 已实现任务监督与未知崩溃重放并登记 `webpage.collect`。用户已确认逻辑 Job 起止时钟、保守归属及 `webpage.collect` 30 分钟 SLA；T02 与 T03 已通过：定向测试 15 passed，backend 隔离 CI 549 passed/12 skipped，contract/runtime workflows 成功，且未增加重复 API/DDL/客户端路径。`/api/ready` 为 200；同配置只读查询确认 `hotkey-server` 有 `jobs` schema 但 owner_count=0、job_count=0；CLI 返回 `identity_uninitialized`，未发现 HotKey Worker。72 小时观察未开始；待 owner 初始化、获准真实工作负载及现有 Worker 启动后再登记窗口，T01/G3 部分开放、产品 AC 仍为 0/6，未建立 Acceptance。

**[036 计划](docs/plans/036-数据访问与生命周期计划.md) S00—S02 已完成，Plan 保持 in_progress。** 来源政策、字段最小化、从严保留与缩期、即时读取屏障、幂等删除、Redis/MinIO 在线清理和有限重试已通过本机既有 PostgreSQL 18.4、Redis 与 MinIO；专用 12 tests、后端全量 63 tests 及静态门禁通过。受控样本不代表任何真实平台已授权；没有 HTTP/UI/Worker 变化或真实业务对象接入，S03 的备份/回补和来源状态、S04 及 036 产品 AC 仍为 0/6，未建立 Acceptance。

**[037 计划](docs/plans/037-费用与资源约束计划.md) S00—S02 与 S03a 技术子片已通过，Plan 保持 in_progress。** `jobs` 领域已增加免费组件策略、副作用前尝试账本、分层持久窗口、原子预留、幂等结算/释放、耗尽延期及 owner 过滤的只读预算窗口快照；当前子片预算目标 42 passed、后端全量 501 passed/5 skipped，Ruff/format/mypy 通过。5 项跳过均需显式启用隔离浏览器服务；本片复用本地 PostgreSQL，仅用一次性 QA 库并已删除。未接入真实 SDK/HTTP/UI/Worker，不证明实耗成本、物理资源分摊、SDK 内部重试、长时占用自动回收或免费业务闭环；X paid 仍未放行，S03/S04 与 037 产品 AC 仍为 0/6，未建立 Acceptance。

2026-09-23 X 离线金额账本子片：沿用同一预算窗口增加 `x_api_usd_micros` 度量、显式 Post 单价的最坏/实际/未知整数换算及 X 来源隔离；不预置费率或额度。新建的一次性空 QA 库按唯一 Schema 验证并发最后额度、缩额、结算、服务与数据库约束，`MockTransport` 两页查询与持久金额窗口联验通过；预算单元 14、集成 19、后端全量 401 passed/5 skipped，Ruff/format/mypy 通过。QA 库和既有测试库验后为空，一次性 QA 库已删除。`paid` 核心组件仍拒绝，控制台账期上限、费率快照、真实请求与产品 0/6 AC 未关闭。

2026-09-23 X 双预算离线子片：`jobs` 在同一事务原子预留 `network_request` 与最坏 Post 金额；金额延期/缺策略回滚请求预留与窗口，两个 ID 必须成对，已结算 ID 不再可作为新请求授权。隔离 PostgreSQL 的并发最后额度与 `MockTransport` 两页联验通过；预算集成 23 项、后端全量 405 passed/5 skipped，Ruff/format/mypy 通过。未改 DDL、HTTP、UI、Worker 或代理，未触达 X；付费尝试账本/账期硬上限/费率快照/凭据仍缺，037 S03/S04 和产品 0/6 AC 保持未完成。

同日 X 报价幂等补片：隔离 PostgreSQL 中，10×5,000 与 5×10,000 微美元的同额报价曾错误复用同一预留；失败优先用例确认后，金额预留强制携带匹配报价并纳入既有指纹，非 X 指纹不变。预算目标 41 passed、后端全量 426 passed/5 skipped，Ruff check/format 与 mypy 通过；5 项跳过需显式启用隔离浏览器服务器。没有 DDL、HTTP、UI、Worker、真实 X 请求或第二套依赖；仅清理本次一次性 QA 库。X App/Token、账期硬上限、费率快照、付费尝试账本原子装配仍缺，037 产品 AC 保持 0/6。

**[038 计划](docs/plans/038-可维护与可替换计划.md) S00/S01 已完成，Plan 保持 in_progress。** `sources` 领域已增加四类纯能力请求、统一作品/评论、显式缺失值与父链、不透明分页/水位、页状态/停止原因及结构化适配器端口；新增 5 tests、相关 23 tests、后端全量 92 tests 及静态门禁通过。2026-09-24 S02 前置核验确认官方 Firecrawl `v2.11.162` tag 的 LICENSE 为 AGPL-3.0-or-later，本地 `6d9fb16` 以该 tag 为父提交。找到权限 0600 的现有 `.env` 并核实 API/Playwright 环境项 46/46、8/8 与运行容器匹配后，从干净提交重建镜像并仅 `--no-deps` 原位重建两个现有容器。最新 readiness/Browser health 均为 200，但受控网页仍未提取正文；出口路径与既有代理策略的一致性待安全复核，细节不在公开交接文档披露。固定样本、S02—S04、G3/G4、产品 AC 仍待执行，未建立 Acceptance。

**[039 计划](docs/plans/039-可观测与可运维计划.md) S00/S01、S02a 与 S02b G4 实现/Green 子片已完成，Plan 保持 in_progress。** S02a 增加按 owner 隔离的连续三次任务失败摘要 API；S02b 在现有任务详情呈现 owner/current-job 限定的持久窗口范围、状态、停止原因和页数，不推断未记录范围。`4820b56c` 后端隔离 PostgreSQL CI 533 passed/15 skipped，窗口投影集成文件 11 passed；Ruff/format/mypy、contract/runtime CI 通过，前端 47 tests/lint/typecheck/format/build 通过。本机合成响应浏览器完成刷新交互、390×844 无横向溢出和 axe 0 violations。首轮 CI 发现精确任务状态快照未列出新增空 `coverage_windows`，补断言后复验成功。实现前隔离 PostgreSQL Red 未执行，S02b G3 保持开放；未写入 `hotkey-server`。完整新鲜度/覆盖缺口、S03—S04 和 039 产品 AC 仍为 0/6，未建立 Acceptance。

**[027 计划](docs/plans/027-数据正确性计划.md) S00 已完成，Plan 保持 in_progress。** Design v1.0 冻结不透明来源身份、不可变观察/版本、零与未知、时间类型、父链缺失和跨领域职责，为 028 S01 提供 `EvidenceResource` 生命周期锚点边界；本切片只有设计与依赖核对，没有代码、DDL、HTTP/UI、环境或产品 Acceptance，来源具体作用域/单位、受控异常样本及 S01—S04 仍待执行。

**[028 计划](docs/plans/028-可追溯与可复现计划.md) S00/S01 已完成，Plan 保持 in_progress。** `evidence` 领域已增加不可变输入清单、subject/reference 比较集、方法版本、受限参数和稳定 SHA-256 指纹；同一 job/result kind 等价重试幂等返回，冲突重用拒绝，创建与读取继续受当前资源到期/删除屏障约束。复用现有 `backend/.env` 与已启动 PostgreSQL 18.4，将确认 0 行的 `hotkey_dev` 按完整 schema 重建为 19 表；专用 4 tests、后端全量 101 tests、12 个架构测试及静态门禁通过，测试后 19 表 0 行，既有 API `/api/ready` 为 200。未新增服务、依赖、脚本、HTTP/UI、真实评分或模型输出；S02—S04 与 028 产品 AC 仍为 0/6，未建立 Acceptance。

**[029 计划](docs/plans/029-时效与数据新鲜度计划.md) S00—S02 技术切片完成，Plan 仍为 in_progress。** 延续现有 `jobs`/`job_attempts` 事实，新增 owner、来源能力及精确配置版本限定的只读新鲜度投影：最近真实尝试、最近完整成功，以及 queued 的限流/预算/瞬时失败/人工重试/内部排队原因；partial 不推进最后成功，延期时长为非负整数微秒。复用现有任务状态 API、OpenAPI 生成链和详情页，无 DDL、新表、依赖、脚本或第二服务。提交 `549abfcd` 的 [backend 隔离 PostgreSQL CI](https://github.com/StephenQiu30/hotkey-server/actions/runs/35953606299) 530 passed/15 skipped，实际聚合和配置版本隔离通过；[runtime](https://github.com/StephenQiu30/hotkey-server/actions/runs/35953606293) 与 [contract](https://github.com/StephenQiu30/hotkey-server/actions/runs/35953606302) CI 均 success。后端本机 357 passed/188 skipped、Ruff/format/mypy，前端 45 tests/lint/typecheck/format/build、OpenAPI 生成校验通过。受控 API 响应下，1440×900 与 390×844 详情页字段、刷新点击、无横向溢出、axe 0 violation 通过，API readiness 200。**S02 G4 已关闭**；本机隔离 DB URL 未配置，因此没有对 `hotkey-server` 运行清理 fixture、写数据库或启动第二套依赖。S03/S04、B0 与产品 AC 仍为 0/6，未建立 Acceptance。

**[032 计划](docs/plans/032-备份与恢复计划.md) S00/S01 已完成，Plan 保持 in_progress。** 维护 CLI 已增加 `backup create-candidate`，复用 PostgreSQL 18.4 官方 `pg_dump`/`pg_restore` 与现有 MinIO SDK：数据库归档、19 表计数和证据引用基于同一导出快照，清单记录 schema/归档 SHA-256、对象 present/missing/deleted 三态及 `0700/0600` 权限；临时 passfile 不把密码放入 argv、环境或候选包。复用现有 `.env` 和已启动服务，专用 5 tests、后端全量 114 tests、12 个架构测试及静态门禁通过，未新增 DDL、依赖、Compose、脚本、HTTP/UI。MinIO 内容仍未复制，输出明确 `candidate`、`inventory_only`、`restore_verified=false`；S02—S04、独立介质、真实恢复、删除重放、B0 与 032 产品 AC 仍为 0/6，未建立 Acceptance。

**[009 计划](docs/plans/009-采集任务控制计划.md) S00—S04a 技术子片已完成，Plan 保持 `in_progress`。** S01—S03 持久受理、进度/取消、失败分类、有限延期、Outbox 重投、消息防重和手动重试证据见 Plan；S04a 新增 owner 过滤的 `GET /api/jobs` 稳定游标分页和 `/jobs` 历史页。后端全量 492 passed/13 skipped、前端 40 tests，静态检查及生产构建通过；运行时 OpenAPI 已生成客户端。复用原有 API/Web 进程与现有依赖，没有新增依赖服务、脚本或 `.sh`。浏览器在 1280×900、390×844 使用 mock 响应验证空/正常/分页/错误/详情导航；窄屏 `/events` 任务入口可达 `/jobs` 且无横向溢出，axe 0 violations；未写入现有开发库。真实来源处理器及最终产品 AC 仍未通过，产品 0/6，未建立 Acceptance。

**[003 计划](docs/plans/003-监控主题管理计划.md) S00—S03 已完成，Plan 保持 in_progress。** `monitors` 领域除本地规则和不可变版本外，已实现 owner UUID 游标列表、独立复制 v1/默认暂停、行锁启停/归档、ready 门禁、归档写保护，以及会话/CSRF 保护的纯本地规则预览。预览不回显标题样本、不写主题/版本/job、不调用来源；本地别名明确 0 查询/0 预算，上游查询和预算保持未知。浏览器验证发现并修复 Portal 内预览提交冒泡导致外层主题表单误提交。运行时 OpenAPI、生成客户端、工作台/详情及共用预览 Dialog 已闭环。后端 149 passed/4 skipped，前端 16 tests、静态检查、OpenAPI 漂移与生产构建通过；桌面/390×844 无横向溢出，axe 0 violation，修复后数据库为 0 主题/0 版本/0 job，QA 数据已清理。没有真实上游扩词、调度器、主题业务 job 或来源，单任务取消继续使用 009 入口；未新增依赖、DDL、服务、脚本或 `.sh`，产品 AC 仍为 0/6，未建立 Acceptance。

**[004 计划](docs/plans/004-平台与连接管理计划.md) S00—S03 内部技术闭环已完成，Plan 保持 in_progress。** `connections` 已实现 X/抖音目录、连接/不可变版本/追加式证据三表、准入政策与当前版本投影、manual/scheduled 独立状态、能力查询和连接配置/替换/启停 API/UI。S02 的 probe/persisted read 分立机制经 S03 收紧为必填执行版本；更换/停用拒绝迟到新增，原事实重放仍幂等。维护者只在服务端配置凭据，浏览器不输入/回读秘密；停用和当前版认证失效阻断新任务/人工重试。受控证据、真实 PostgreSQL 并发及浏览器闭环通过，最新 266 后端/25 前端测试和运行证据见下文。没有真实连接/探测/采集或业务 Worker，产品 AC 仍为 0/6，未建立 Acceptance。

**[007 计划](docs/plans/007-作品资料与上下文计划.md) S00—S03 已完成，Plan 保持 in_progress。** `content` 已交付唯一作品、发现、不可变指标/正文版本、完整度与来源标签、quote/repost、追加式可见性、编辑历史、乱序稳定投影和 PostgreSQL 生命周期清理；只读 API、生成客户端和列表/详情已闭环。S03 提交 `f3d704ad`，技术验证记录见 Plan；QA 后开发/测试库均 30 表 0 行。没有稳定原生 ID 的链接仍只作线索；S04、真实来源及产品 0/6 AC 仍待执行，未建立 Acceptance。

**[002 计划](docs/plans/002-X免费采集与热点监控计划.md) S01 受控技术切片已完成，Plan 为 in_progress。** `sources/adapters/x_twscrape.py` 固定 twscrape 0.20.1，复用 SDK 常量/解析/签名，由有界 HTTPX 统一管理初始化、分页、全部尝试计量、取消和故障停止；不调用默认账号池、隐式重试/轮换及解析落盘。27 项专用测试、后端全量 226 tests、Ruff/format/mypy 通过；应用关闭传输层敏感诊断，CDN 不带会话。复用既有 env 和 PostgreSQL/Redis/Kafka/MinIO；无新 HTTP、DDL、页面、脚本、服务或实际 X 请求。S00 真实准入、S02 的持久预算/连接租约/业务装配及 S03/S04 尚未完成，X 仍未就绪，产品 AC 为 0/10，未建立 Acceptance。

后端采用模块化单体与按业务领域分组的分层结构，完整目录、文件职责、API 契约、事务和依赖方向固定在根目录 [PROJECT.md](PROJECT.md)；执行入口、实现门禁和验证命令见 [AGENTS.md](AGENTS.md#fastapi-目录与命名必须执行)。

1. 047 S02 网页业务闭环及 S03-T00/T01 浏览器基础已完成：固定 Playwright Python/Server 1.63.0 原生 WS，browser 默认断网且非 root/sandbox，暂不引入 Scrapling/Selector/CDP。S03-T02 受控动态动作、离线状态文件、连接执行门禁和已有连接维护 CLI 已局部验证；下一步仍需真实平台登录/连接初始化、业务任务取消/崩溃恢复及平台出站安全验证。不得把内部无网络探针当作四平台评论或产品 Acceptance，产品 AC 仍为 0/8。
2. 保持旧 PostgreSQL 数据库不变；当前 `hotkey_dev` 已按完整 schema 重建，后续存量变更继续采用新库建表与校验导入，不增加运行时迁移。
3. 在业务表和任务接齐后执行 042 S02—S04 的完整 B0、高水位、共同负载、两环境恢复与回滚验证。
4. 按业务切片实现页面并完成桌面、窄屏和端到端验收。

## 检查

后端在 `backend/` 执行 `uv run ruff format --check .`、`uv run ruff check .`、`uv run mypy` 和 `uv run pytest`。前端执行 `pnpm test`、`pnpm lint`、`pnpm typecheck`、`pnpm format:check` 和 `pnpm build`；运行中的后端配合 `pnpm openapi:check` 校验生成漂移。产品进度以 `BACKLOG.md` 和对应 Acceptance 为准。

## 本轮产品文档复核

2026-09-21 先基于 HEAD `9093ed47` 静态复核工程，随后从 `37064d2a` 执行 046 与 042 S00/S01。BACKLOG 已补完整交付内容、跨计划批次、平台扩面及 App 队列；046 技术前置 8/8 AC 已通过，042 运行底座切片已通过，但所有产品 AC 仍未通过，业务流程、完整容量/恢复和验收仍待完成。

本轮已交付 009/003/007/004 S00—S03 和 002 S01，技术证据见上文与各 Plan。验证复用当前 PostgreSQL/Redis/Kafka/MinIO 与 env，未启动第二套依赖；002 S01 收尾后后端 226 tests、前端 22 tests、静态检查、OpenAPI/客户端无漂移及隔离生产构建通过。构建另发现既有路由辅助函数非法导出，已以独立修复提交保留内部函数并新增导出集合断言。尚未执行真实业务处理器/来源、真实连接探测/采集、上游扩词、跨任务来源预算/租约装配、真实评分/模型记录、独立对象备份/真实恢复、完整 B0 或产品 Acceptance。
