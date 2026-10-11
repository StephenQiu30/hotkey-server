# Ripplesight BACKLOG

需求、设计和计划按 [00–58 同编号目录](docs/index.md) 维护。本文记录当前实现依据、缺口、待办和验收边界，不记录完成流水。

实现核对基于本地 `main` / `b8795786` 及现有工作区文件，覆盖59组编号文档；本轮另按REQ-06 MON-01修复暂停保护。“已实现”指已找到服务/接口/页面实现路径，**不代表本轮工程检查通过或能力可用**。“待补齐”是已确认的合同或行为差距；“待核对”是尚不能下结论的全路径范围；“待验收”需要独立环境、真实数据、自然时间或浏览器证据。

暂停保护改动包含主题采集保护、对应固定样本测试、旧MediaCrawler搜索安全拒绝及PROJECT/06设计/计划与本文件；未调用真实平台、模型、通知或业务库，未启动/改动业务服务。后端测试仅连接独立测试库，两份本轮测试库均已删除；工程检查与独立审查结果见06。其余条目的源码依据不等于需求验收通过。

## P0 当前执行队列

按保护 → 配置与采集 → 相关性与事件 → 持续监测 → 提醒的依赖推进；可并行准备无需外部请求的样本和合同。

- [x] **06/10/47 暂停保护（代码与固定样本）**：搜索/评论任务冻结主题状态序号，领取、逐请求、取消探测、心跳及重试复核；暂停恢复不复活旧任务。父进程补取消覆盖和在途用量结算，保留已提交材料/已确认窗口。独立审查已完成；工程检查与真实验收边界见06。
- [ ] **01/02/33/44 查询合同**：补主题语言、窗口、上限与版本读回；逐词列出上游支持、本地精筛、不支持和因上限未执行的条件，保留三组词与本地预览。
- [ ] **03/36/46 增量与覆盖**：复用身份/版本/观察/窗口回执，补新增、完全重复、旧帖更新的互斥计数与分母；明确B站根评论/回复及单页上限，不宣称全量。
- [ ] **09/04 核心分析解耦**：解除普通关键词相关性对情感输出的强制依赖；明确逐评论结合原帖/父链的独立相关性执行范围，保留未知/错误/限流延后。
- [ ] **04/05/38/39 事件解释**：复用候选、原生/语义召回与修订，补用户可见线索/候选状态、本轮新增/补证/重复差分及进入热点理由；单材料候选已有实现，不重复建设。
- [ ] **07/12/32 趋势统计**：补24小时唯一相关帖/可识别作者/新事实与24小时桶，标明窗口、规则、来源覆盖和不可比；保留现有48小时attention及刊物日窗口计数。
- [ ] **07/35 首次热点提醒**：补首次达标合同、事件+规则版本去重，复用冷却/通知/未知发送核对；免费渠道实际送达是独立门槛。
- [ ] **08/11 首轮与质量门槛**：确定首个已准入免费来源（先核对B站/HN）、主题/语言/查询、账号、本机模型名、窗口/上限及真实请求授权；准备分层校准集/保留集，先测基线再冻结阈值。
- [ ] **02–07/11 真实验收**：两轮真实采集 → 相关性/事件独立评测 → 至少七个连续自然日监测且两轮自然成功、有事件更新 → 至少一次自然热点提醒本人收到；核心页面同步验收。各门槛分别保留证据，未满足不得整体打勾。

## 00–11 核心能力逐项核对

### [00 产品目标与边界](docs/requirement/00-产品目标与边界.md) · [设计](docs/design/00-产品目标与边界.md) · [计划](docs/plan/00-产品目标与边界.md)

- **已实现（源码核对）**：默认本机模型、来源准入、预算和公开/个人读取已有分层。[capability_routing.py](backend/app/ai/capability_routing.py#L18)；[dependencies.py](backend/app/api/dependencies.py#L527)
- **待补齐/待核对**：统一分发及 Codex 公告仍留有工作台/首页入口、路由和实现，冻结功能的退役尚未完成。[page.tsx](frontend/src/app/workspace/page.tsx#L61)；[home-content.tsx](frontend/src/app/components/home-content.tsx#L116)。配置默认关闭不能当作所有入口已经清理；先查消费者与备份恢复，再决定退役范围。
- **待验收**：当前运行配置、外部请求开关和授权未作现场检查；不得据残留源码认定正在启用。保留零费用、单人账号、公开或本人可访问内容、风控即停的边界。

### [01 关键词配置](docs/requirement/01-关键词配置.md) · [设计](docs/design/01-关键词配置.md) · [计划](docs/plan/01-关键词配置.md)

- **已实现（源码核对）**：任一词/必含词/排除词、NFKC 与大小写归一、稳定去重、跨组冲突校验、词边界精筛、规则版本、草稿预览已有实现。[services.py](backend/app/monitors/services.py)；[topic-rule-preview.tsx](frontend/src/components/monitors/topic-rule-preview.tsx#L52)
- **待补齐/待核对**：主题输入没有语言、搜索窗口和主题级采集上限。现有来源策略有上限，但查询直接按 max_queries 截断，未逐词解释未执行项；补配置读写、冻结版本及逐查询回执，不重写三组词。[schemas.py](backend/app/monitors/schemas.py#L41)；[runs.py](backend/app/monitors/runs.py#L163)
- **待验收**：上游可支持条件、本地精筛条件和不支持条件逐平台对照；检验中英文、短词、同形词、规则切换和冲突。已有本地预览不证明真实搜索覆盖。

### [02 关键词采集](docs/requirement/02-关键词采集.md) · [设计](docs/design/02-关键词采集.md) · [计划](docs/plan/02-关键词采集.md)

- **已实现（源码核对）**：搜索适配、采集执行、预算/游标合同和原生身份归档可复用；现有关键词工厂有 B 站 Chrome 与 HN 路径。[discovery_execution.py](backend/app/content/discovery_execution.py)。B 站受限单页搜索与根评论；HN 评论树可保留父链并分页读取。[bilibili_chrome.py](backend/app/sources/adapters/bilibili_chrome.py#L225)；[hackernews.py](backend/app/sources/adapters/hackernews.py#L59)
- **待补齐/待核对**：B 站搜索、评论有明确小样本上限，回复请求不支持，不能标全量；需要按来源补足需求范围或明确部分/不支持的回执。X/Instagram 关键词路径尚不满足目标，见08。先核对原帖、根评论、回复各自受理与归档的完整链路。
- **待验收**：选择一个已准入免费来源，按同账号/查询/排序/窗口做至少两轮真实对照；原生 ID、发布时间、父链、返回数、预算和终止证据可追溯。本轮没有发出平台请求。

### [03 增量去重与覆盖](docs/requirement/03-增量去重与覆盖.md) · [设计](docs/design/03-增量去重与覆盖.md) · [计划](docs/plan/03-增量去重与覆盖.md)

- **已实现（源码核对）**：owner/来源/类型/原生 ID 身份、不可变正文版本、观察记录、游标预算、应执行窗口和 missing 回执已有实现。[services.py](backend/app/content/services.py#L632)；[coverage.py](backend/app/jobs/coverage.py#L59)；[schemas.py](backend/app/jobs/schemas.py#L176)
- **待补齐/待核对**：现有 deduplicated_count 是“观察数减首次入库数”，旧帖新正文版本也归入其中；补互斥的新增/完全重复/旧帖更新分类，并解释上游返回、过滤后观察与唯一身份的不同分母。逐查询未执行项见01。
- **待验收**：至少两轮真实采集验证重复不造新帖、旧帖更新保留身份、分页循环与游标失效有界停止、零结果/部分/错误/停止可辨。存在计数字段不等于口径通过。

### [04 热点事件识别](docs/requirement/04-热点事件识别.md) · [设计](docs/design/04-热点事件识别.md) · [计划](docs/plan/04-热点事件识别.md)

- **已实现（源码核对）**：普通主题从有效相关性标注承接材料，标题/时间/关键词召回与模型关系判断分层，原生/语义召回及事件修订已有路径。单材料在可接纳证据条件下已能形成候选，不应重建。[services.py](backend/app/events/services.py#L104)；[clustering.py](backend/app/events/clustering.py#L237)
- **待补齐/待核对**：事件读取合同只有 active/merged 与 active/watching/settled 等阶段，缺少用户可辨的线索/候选状态及进入热点的解释。[schemas.py](backend/app/events/schemas.py#L107)。普通关键词路径对情感的强制依赖见09；先解除，再校准事件质量。
- **待验收**：独立标注同事件/不同事件，覆盖同主体多事件、旧闻重发、传闻与确认、跨平台转载、单材料及信息不足；冻结误合并/漏合并阈值后用保留样本评测。原生召回和标题相似不是判定通过。

### [05 事件证据与新事实](docs/requirement/05-事件证据与新事实.md) · [设计](docs/design/05-事件证据与新事实.md) · [计划](docs/plan/05-事件证据与新事实.md)

- **已实现（源码核对）**：事实角色、固定材料版本、修订时间线及合并/拆分/移动纠错可复用；事件页按固定 revision 读取事实。[event-facts.tsx](frontend/src/app/events/[eventId]/components/event-facts.tsx#L48)；[attention.py](backend/app/events/attention.py#L51)
- **待补齐/待核对**：当前事实列表没有本轮新增/补证/重复的基准差分展示；补差分合同与证据引用。已有最低参与来源和编辑证据等热点资格规则，需明确面向关键词事件的窗口、最小样本与理由，不能以事实条数直接代替新事实。
- **待验收**：真实材料逐事实核对引用、来源时间、修订与撤回；转载/重复不造进展，错误归并可纠正且身份可追踪。热点规则需先校准冻结，再验收；事件页五状态及操作待浏览器验收。

### [06 持续监测与恢复](docs/requirement/06-持续监测与恢复.md) · [设计](docs/design/06-持续监测与恢复.md) · [计划](docs/plan/06-持续监测与恢复.md)

- **已实现（源码核对）**：低频调度、应执行窗口、稳定操作 ID、租约/outbox、预算与准入、任务取消、连接版本检查、有限补轮和失败恢复已有实现；页面刷新只是读取。[scheduler.py](backend/app/worker/scheduler.py#L90)；[execution.py](backend/app/jobs/execution.py)；[discovery.py](backend/app/content/discovery.py)
- **已修复（暂停保护）**：关键词搜索与评论受理时服务端冻结主题状态事件序号；同 owner 的 active 状态与序号在一个数据库快照中读取，领取/预算前/逐请求/取消探测/心跳/手动重试复核。暂停、归档或自动暂停后旧任务安全取消；同一时间暂停后恢复不会复活旧任务，缺少序号的历史任务拒绝执行，幂等重放不重绑。[collection_topics.py](backend/app/jobs/collection_topics.py)；[execution.py](backend/app/jobs/execution.py)；[services.py](backend/app/monitors/services.py)。父进程在终止子进程后记录 partial/cancelled 覆盖并保守结算 STARTED 请求，重投不重复扣量；已提交材料、checkpoint 和 confirmed 窗口保留，损坏 scope 不造覆盖。[collection_cancellation.py](backend/app/content/collection_cancellation.py)；[app.py](backend/app/worker/app.py)。
- **停止边界**：暂停提交后进入请求准入检查的新请求被拒绝；暂停前已获准的请求按在途处理，可能结束或被父进程终止，尚未提交的响应不承诺入库。排队旧任务在领取时、运行旧任务在下一次检查时落取消回执；暂停接口不批量改写全部历史任务，也不自动补发旧操作。非主题热榜和其他主题/账号不受该主题暂停影响。旧 MediaCrawler 搜索桥只有整批预扣与子进程轮询，不能保证逐HTTP暂停，线上工厂已在启动前拒绝并返回不可重试的 `search_request_guard_unavailable`；Chrome逐请求方式、本地样本及缓存评论保留。
- **受控验证与工程检查**：`ruff check`、`ruff format --check`、`mypy`（439份源码）与185份Markdown文档检查通过；最终补充回归142通过/1跳过（Kafka未配置），其中暂停专项28项全通过。后端全量回归2550通过/33跳过（Kafka、Redis、MinIO未配置）；后续新增保护与测试已由上述补充回归覆盖。跳过项未记作通过；未改变API/数据库结构/UI，Web构建与浏览器验收不属于本次变更。专项覆盖搜索/评论排队与执行、同时间暂停恢复、无新预算/请求、在途与分页材料保留、旧失败重试/幂等重放、归档/自动暂停、跨 owner/其他主题/热榜、主题行锁不阻塞准入，以及实际 dispatcher + 受控 supervisor 的取消、在途结算、损坏旧 scope、checkpoint/confirmed 保留和已受理MediaCrawler任务在计费/子进程前拒绝。[test_topic_collection_pause.py](backend/tests/integration/test_topic_collection_pause.py)。只读独立审查提出的父进程收尾和损坏旧 scope 问题已修复并复核，无剩余阻断项。
- **待补齐/待验收**：如需恢复MediaCrawler线上搜索，先补逐实际HTTP请求的准入桥和受控验证，不能靠轮询宣称停止保证。暂停代码与固定样本结果不代替真实平台；到期、验证码/限流、超时未知、重启与模型积压仍需各自核验，再连续至少七个自然日观察，至少两轮自然成功并有事件更新。逐应执行窗口解释成功/部分/失败/漏轮、延迟和人工干预。

### [07 趋势与告警](docs/requirement/07-趋势与告警.md) · [设计](docs/design/07-趋势与告警.md) · [计划](docs/plan/07-趋势与告警.md)

- **已实现（源码核对）**：现有 attention 为48小时窗口、24小时半衰期、6小时比较，含覆盖可比性与未知处理；刊物还有日窗口来源参与计数。告警已有 negative_count/heat_increment、持久冷却、窗口幂等和未知 sending 抑制。[attention.py](backend/app/events/attention.py#L51)；[heat.py](backend/app/events/heat.py#L421)；[alert_services.py](backend/app/notifications/alert_services.py#L373)
- **待补齐/待核对**：缺真正的24小时唯一相关帖/可识别作者/新事实组合指标与24个小时命中桶；不能用48小时热度或最近5条代替。[topic-overview.tsx](frontend/src/app/monitors/[topicId]/components/topic-overview.tsx#L71)。缺首次达到热点条件的触发合同及事件+规则版本去重；现有冷却到期可再触发，不能声称已实现首次一次性提醒。[alert_schemas.py](backend/app/notifications/alert_schemas.py#L10)
- **待验收**：冻结窗口/分母/规则版本，验证断采、覆盖变化不造趋势；复用通知链，选定一个获授权免费渠道，完成自然触发到本人收到。供应商受理或发送记录不算实际收到；无自然触发则继续观察。

### [08 平台接入](docs/requirement/08-平台接入.md) · [设计](docs/design/08-平台接入.md) · [计划](docs/plan/08-平台接入.md)

- **已实现（源码核对）**：能力目录、连接版本、准入和预算可复用，实际关键词入口见02；X 适配器只允许 MockTransport，编辑采集工厂也有免费边界拒绝。[x_api.py](backend/app/sources/adapters/x_api.py#L62)；[editorial_factory.py](backend/app/sources/editorial_factory.py#L305)
- **待补齐/待核对**：X 目标保留，但当前收费路径不实施；Instagram 任意正文关键词仍有可行性/准入缺口，标签或账号发现不能冒充关键词覆盖。OpenRouter 是模型服务与辅助搜索候选，不能列作社交采集源。Reddit 批准与其他来源准入分别待明确。
- **待验收**：每个平台独立确认关键词、原帖、评论/回复、历史窗口、分页和限制；首个平台通过不替代 X/Instagram。没有新的平台、云端模型、第三方费用或账号授权。

### [09 模型分析](docs/requirement/09-模型分析.md) · [设计](docs/design/09-模型分析.md) · [计划](docs/plan/09-模型分析.md)

- **已实现（源码核对）**：本机 Codex app-server 的结构化输出与禁工具执行、prompt/主题版本、分析需要账本及限流延后可复用；兼容模型路由源码存在，默认开关关闭，不代表 OpenRouter 已配置启用。[codex_app_server.py](backend/app/ai/adapters/codex_app_server.py#L68)；[services.py](backend/app/analysis/services.py#L632)；[capability_routing.py](backend/app/ai/capability_routing.py#L18)
- **待补齐/待核对**：普通关键词主题的 relevant=true 强制 sentiment 非空，并由有效 annotation 进入事件，违背情感P2不阻塞核心的要求；内部编辑主题有单独路径，不能推广为所有事件都耦合。[schemas.py](backend/app/analysis/schemas.py#L235)。另：目前只分析 post，读取最多51条评论（含截断哨兵），实际 prompt 最多50条且受总字符预算约束；评论作为平铺样本，缺逐评论结合原帖/父链的独立相关性合同。[services.py](backend/app/content/services.py#L3359)；[services.py](backend/app/analysis/services.py#L1151)；[prompts.py](backend/app/analysis/prompts.py#L118)
- **待验收**：先解除相关性与情感强耦合，验证 bool/理由/未知/失败/延后；用分层真实样本评测。评论上下文范围明确后单独验收；情感/观点和200条样本评测留P2，不伪装已经完成。

### [10 权限与数据生命周期](docs/requirement/10-权限与数据生命周期.md) · [设计](docs/design/10-权限与数据生命周期.md) · [计划](docs/plan/10-权限与数据生命周期.md)

- **已实现（源码核对）**：会话 owner、CSRF、独立运营令牌、当前公开许可、精确 observation/version 输入及关联删除已有实现；HTML 渲染有清洗。[dependencies.py](backend/app/api/dependencies.py#L527)；[lifecycle.py](backend/app/content/lifecycle.py#L19)；[content.ts](frontend/src/components/editor/content.ts#L37)
- **待补齐/待核对**：主题暂停保护已补并受控验证，证据与边界见06；不据此认定其他权限路径通过。其余权限与生命周期尚不能仅凭入口断言全通过：逐 API/公开投影/媒体/报告导出/通知复核当前许可，个人来源即使属于发布账号也不得自动公开。退役/物理删表须先核对消费者及独立库恢复。
- **待验收**：跨owner、会话过期、撤许可/撤材料、旧revision、派生删除、恶意HTML与导出失效在独立环境验证；实际配置、浏览器及备份恢复未在本轮验收。不能用权限依赖存在代替全路径隔离通过。

### [11 质量评测与验收](docs/requirement/11-质量评测与验收.md) · [设计](docs/design/11-质量评测与验收.md) · [计划](docs/plan/11-质量评测与验收.md)

- **已实现（源码核对）**：SelectBench 有标注导入、混淆指标和阈值评估；采集/分析时效、应执行窗口和覆盖回执有计算基础，现有受控测试可复用。[evaluation_services.py](backend/app/analysis/evaluation_services.py#L97)；[metrics.py](backend/app/jobs/metrics.py#L49)
- **待补齐/待核对**：编辑精选评测不等于关键词 bool、事件归并和持续运行的独立验收。补分层真实样本、校准/保留样本分离、正式阈值及证据记录；先测基线，再冻结质量、运行成功率与延迟阈值，不能自行编固定目标。
- **待验收**：两轮真实采集、独立相关性/事件评测、七日自然运行及本人收到首次热点提醒均待验收；硬门槛失败阻止通过。工程测试、固定样本、真实平台、自然时间、用户体验和独立审查分别报告。

## 12–58 页面与辅助能力逐项核对

每项均链接同编号需求、设计和计划及实际页面入口。表中的“本轮未确认新的必补功能”只表示未取得可确定的新缺口，不表示全字段、全状态或真实业务验收通过。所有动态页面另须按11核对1440px/390px五状态、键盘、200%缩放和减少动态效果；只读不触发采集，失败保留输入，冲突重读，未知写入先核对。

### 公开阅读与模型榜

| 编号及文档 | 已实现（源码核对） | 待补齐 / 待核对 | 待验收 |
|---|---|---|---|
| [12 今日热点](docs/requirement/12-今日热点.md) · [设计](docs/design/12-今日热点.md) · [计划](docs/plan/12-今日热点.md) | 分类URL、资讯分批展开、上升事件与最新公开日报分别读取，空/错状态保留。 [home-content.tsx](frontend/src/app/components/home-content.tsx#L39) | 四项站点业务统计为“—”，负面区块缺数据；现有site stats不等于所需事件/24h/负面指标。RSS入口与冻结边界对齐。 | 真实零值/未知、热点排序及许可变更；24h依赖07。 |
| [13 探索资讯](docs/requirement/13-探索资讯.md) · [设计](docs/design/13-探索资讯.md) · [计划](docs/plan/13-探索资讯.md) | 公开资讯q/来源筛选、游标与时间线读取；URL变化清游标，搜索可预填新主题。 [page.tsx](frontend/src/app/discover/page.tsx#L33) | 事件/评论检索与情感筛选尚无完整入口；现有搜索只覆盖资讯，禁用项不能算完成。 | 字面AND、来源联动、游标边界及当前许可。 |
| [14 公开专题目录](docs/requirement/14-公开专题目录.md) · [设计](docs/design/14-公开专题目录.md) · [计划](docs/plan/14-公开专题目录.md) | 公司/领域/类型目录、配置专题与真实可读计数。 [page.tsx](frontend/src/app/discover/topics/page.tsx#L15) | 本轮未确认新的必补功能；公开计数/许可完整性待核对。 | 真实空目录、近期窗口与无许可不计数。 |
| [15 公开专题详情](docs/requirement/15-公开专题详情.md) · [设计](docs/design/15-公开专题详情.md) · [计划](docs/plan/15-公开专题详情.md) | 专题定义、总量/近期量、20条分页与关联导航；不存在独立404。 [page.tsx](frontend/src/app/discover/topics/[slug]/page.tsx#L20) | 本轮未确认新的必补功能。 | 动态slug、分页末尾、错误与撤许可。 |
| [16 公开事件目录](docs/requirement/16-公开事件目录.md) · [设计](docs/design/16-公开事件目录.md) · [计划](docs/plan/16-公开事件目录.md) | 独立公开热事件目录，按现行合同最多20条，不伪造分页。 [page.tsx](frontend/src/app/discover/stories/page.tsx#L10) | 事件资格、候选解释和窗口统计依赖04/07。 | 真实排序、零结果及许可收紧。 |
| [17 公开事件详情](docs/requirement/17-公开事件详情.md) · [设计](docs/design/17-公开事件详情.md) · [计划](docs/plan/17-公开事件详情.md) | 事件摘要、可读来源/报道、固定revision与7日发展读取，分享链接可用。 [public-story-reading.tsx](frontend/src/app/discover/stories/[eventId]/components/public-story-reading.tsx#L35) | 关注/收藏按钮禁用；情感/观点、24h历史和本轮事实差分未完整提供。 | 公开许可与无权/不存在、证据可达、时间线去重。 |
| [18 公开资讯阅读](docs/requirement/18-公开资讯阅读.md) · [设计](docs/design/18-公开资讯阅读.md) · [计划](docs/plan/18-公开资讯阅读.md) | 正文/元数据/来源外链、媒体、本机收藏已读与阅读位置恢复。 [item-reader.tsx](frontend/src/app/items/[contentId]/components/item-reader.tsx#L42) | 本轮未确认新的必补功能；媒体及许可全路径仍待核对。 | 真实正文降级、失效媒体、阅读恢复、键盘与清洗。 |
| [19 收藏与阅读记录](docs/requirement/19-收藏与阅读记录.md) · [设计](docs/design/19-收藏与阅读记录.md) · [计划](docs/plan/19-收藏与阅读记录.md) | 本机资讯收藏、已读、搜索、笔记、批量操作、JSON导入导出与Markdown导出已有实现。 [local-reading.tsx](frontend/src/components/publication/local-reading.tsx#L67) | 事件/评论/刊期等多类型收藏与关注尚不可用；不要重复立项已有JSON迁移。 | 500收藏/5000已读上限、2000字笔记、导入冲突、批量删除及导出不带受限全文。 |
| [20 最新公开刊物与个人报告详情](docs/requirement/20-最新公开刊物与个人报告详情.md) · [设计](docs/design/20-最新公开刊物与个人报告详情.md) · [计划](docs/plan/20-最新公开刊物与个人报告详情.md) | 最新公开刊物与UUID个人报告分流、目录/导航独立失败、个人报告详情导出入口。 [page.tsx](frontend/src/app/reports/[reportId]/page.tsx#L21) | 本轮未确认新的必补功能；报告生成和真实投递未验收。 | 固定版本/owner、最新刊缺失、导出与材料撤回。 |
| [21 公开固定刊期阅读](docs/requirement/21-公开固定刊期阅读.md) · [设计](docs/design/21-公开固定刊期阅读.md) · [计划](docs/plan/21-公开固定刊期阅读.md) | 固定刊期读取、正文/亮点/事件事实与打印入口；不存在不回退最新刊。 [public-edition-reader.tsx](frontend/src/app/reports/[reportId]/[key]/components/public-edition-reader.tsx#L37) | 社交槽位、评论/情感分区数据仍不完整；不能按空壳宣称已产出。 | 固定key、修订一致、分区真实空/错及打印可读。 |
| [22 公开刊物归档](docs/requirement/22-公开刊物归档.md) · [设计](docs/design/22-公开刊物归档.md) · [计划](docs/plan/22-公开刊物归档.md) | 公开归档20条和before_key，日刊日历初始取最新条目的月份，空目录按上海时区当前月。 [page.tsx](frontend/src/app/reports/[reportId]/archive/page.tsx#L19) | 本轮未确认新的必补功能。 | 历史刊翻页、跨时区日历和空月份。 |
| [23 模型综合榜](docs/requirement/23-模型综合榜.md) · [设计](docs/design/23-模型综合榜.md) · [计划](docs/plan/23-模型综合榜.md) | 固定发布run、0–100得分、开放权重证据、发布缺失与服务错误区分。 [board-reading.tsx](frontend/src/components/leaderboard/board-reading.tsx#L24) | 名次变化为“—”，周历史未提供，长上下文筛选禁用；保留待补状态。 | 真实发布run、证据/价格快照、未知值和筛选一致性。 |
| [24 模型分项榜](docs/requirement/24-模型分项榜.md) · [设计](docs/design/24-模型分项榜.md) · [计划](docs/plan/24-模型分项榜.md) | 分项参数校验与同一BoardReading/发布run复用。 [page.tsx](frontend/src/app/leaderboard/category/[board]/page.tsx#L12) | 依赖23的历史与筛选缺口，不另造一套数据。 | 动态board、404/503、排序与综合榜口径一致。 |
| [25 模型详情](docs/requirement/25-模型详情.md) · [设计](docs/design/25-模型详情.md) · [计划](docs/plan/25-模型详情.md) | 模型分项、来源证据、比较组及带来源/币种/验证日期的价格。 [model-reading.tsx](frontend/src/app/leaderboard/models/[slug]/components/model-reading.tsx#L38) | 本轮未确认新的必补功能；数据完整性与实际依赖可用性未验收。 | 动态slug、缺项/未知价格、快照与汇率日期。 |
| [26 评测来源目录](docs/requirement/26-评测来源目录.md) · [设计](docs/design/26-评测来源目录.md) · [计划](docs/plan/26-评测来源目录.md) | 评测来源目录、来源状态、预算和run相关信息。 [sources-reading.tsx](frontend/src/app/leaderboard/sources/components/sources-reading.tsx#L16) | 本轮未确认新的必补功能；展示目录不证明已完成真实抓取。 | 发布缺失、失败来源与局部读取失败。 |
| [27 评测来源详情](docs/requirement/27-评测来源详情.md) · [设计](docs/design/27-评测来源详情.md) · [计划](docs/plan/27-评测来源详情.md) | 来源详情、原始快照、别名与排除解释。 [source-reading.tsx](frontend/src/app/leaderboard/sources/[sourceKey]/components/source-reading.tsx#L23) | 本轮未确认新的必补功能。 | 动态sourceKey、快照追溯、许可与错误分支。 |
| [28 模型榜计算规则](docs/requirement/28-模型榜计算规则.md) · [设计](docs/design/28-模型榜计算规则.md) · [计划](docs/plan/28-模型榜计算规则.md) | 只读计算规则与发布run配置/常量展示。 [rules-reading.tsx](frontend/src/app/leaderboard/rules/components/rules-reading.tsx#L24) | 本轮未确认新的必补功能；不能把规则展示等同重算正确。 | 独立数据重算、最低样本/缺失项及版本一致性。 |

### 账户与个人监测工作区

| 编号及文档 | 已实现（源码核对） | 待补齐 / 待核对 | 待验收 |
|---|---|---|---|
| [29 登录与验证注册](docs/requirement/29-登录与验证注册.md) · [设计](docs/design/29-登录与验证注册.md) · [计划](docs/plan/29-登录与验证注册.md) | 首页背景登录弹窗、密码/邮件/GitHub路径、安全returnTo和会话错误状态。 [page.tsx](frontend/src/app/login/page.tsx#L14) | 本轮未确认新的必补功能；真实OAuth/邮件依赖未验收。 | 焦点/Esc/返回、过期验证码、真实身份流程与会话撤销。 |
| [30 账户与订阅设置](docs/requirement/30-账户与订阅设置.md) · [设计](docs/design/30-账户与订阅设置.md) · [计划](docs/plan/30-账户与订阅设置.md) | 个人资料/头像读写、凭据、身份连接和报告邮件订阅入口。 [account-settings.tsx](frontend/src/app/account/components/account-settings.tsx#L55) | 本轮未确认新的必补功能；真实账号挑战与送达未验收。 | 冲突/失败保留输入、凭据变更撤会话、真实邮箱/SMTP。 |
| [31 个人工作台入口](docs/requirement/31-个人工作台入口.md) · [设计](docs/design/31-个人工作台入口.md) · [计划](docs/plan/31-个人工作台入口.md) | 登录工作台复用TopicsWorkspace并保留其他业务入口。 [page.tsx](frontend/src/app/workspace/page.tsx#L23) | 仍有/feeds、/agent等冻结功能入口，按00核对后清理。 | 个人会话隔离、入口可达及窄屏导航。 |
| [32 监控主题工作区](docs/requirement/32-监控主题工作区.md) · [设计](docs/design/32-监控主题工作区.md) · [计划](docs/plan/32-监控主题工作区.md) | 全宽主题列表位于详情上方、默认展开可收起；连续主区与次级结果/运行/设置复用。 [topics-workspace.tsx](frontend/src/app/topics/components/topics-workspace.tsx#L19) | 缺24小时命中桶、平台最近成功/条数摘要与告警已读合同；配置缺口见01、暂停见06。 | 仅主题区布局、旧tab深链接、草稿/版本冲突与1440px/390px五状态。 |
| [33 新建监控主题](docs/requirement/33-新建监控主题.md) · [设计](docs/design/33-新建监控主题.md) · [计划](docs/plan/33-新建监控主题.md) | 三组词、来源能力、频率/报告配置、q预填和本地预览；新建默认暂停。 [topic-form.tsx](frontend/src/app/monitors/new/components/topic-form.tsx#L84) | 语言/窗口/上限及逐词执行说明见01；保存不能隐式采集。 | q长度、默认状态、草稿保留、重复提交与来源不可用。 |
| [34 监控主题详情](docs/requirement/34-监控主题详情.md) · [设计](docs/design/34-监控主题详情.md) · [计划](docs/plan/34-监控主题详情.md) | 指定主题复用同一TopicsWorkspace与编辑/读取链。 [page.tsx](frontend/src/app/monitors/[topicId]/page.tsx#L5) | 与32共用数据缺口，不另建一套详情实现。 | 动态topicId、跨owner、不存在、冲突与恢复。 |
| [35 个人告警](docs/requirement/35-个人告警.md) · [设计](docs/design/35-个人告警.md) · [计划](docs/plan/35-个人告警.md) | 主题/事件选项翻页、规则/版本/目标编辑与最多50条历史。 [alerts-workspace.tsx](frontend/src/app/alerts/components/alerts-workspace.tsx#L429) | 首次热点触发/去重见07；已读无写合同，不能假写成功。 | 冷却、重复评估、目标修订、未知发送与本人实际收到。 |
| [36 私有资料列表](docs/requirement/36-私有资料列表.md) · [设计](docs/design/36-私有资料列表.md) · [计划](docs/plan/36-私有资料列表.md) | q/来源/主题/日期/分析状态筛选、20条游标，网页采集为显式任务。 [content-list.tsx](frontend/src/app/content/components/content-list.tsx#L140) | 每轮新增/重复/更新及命中范围依赖01/03；未分析不等于不相关。 | 字面搜索、UTC边界、cursor上下文、owner与只读不采集。 |
| [37 私有资料与评论](docs/requirement/37-私有资料与评论.md) · [设计](docs/design/37-私有资料与评论.md) · [计划](docs/plan/37-私有资料与评论.md) | 版本/观察/来源、分析面板与评论根/回复分页，缺父与未加载状态可辨；评论刷新显式受理。 [content-detail.tsx](frontend/src/app/content/[contentId]/components/content-detail.tsx#L369) | B站根评论限制见02；父链已能存/展，但逐评论上下文相关性分析缺合同，见09。 | 回复先到、缺根/父、20条分页、版本引用与评论复采停止。 |
| [38 个人事件列表](docs/requirement/38-个人事件列表.md) · [设计](docs/design/38-个人事件列表.md) · [计划](docs/plan/38-个人事件列表.md) | 个人事件20条分页、热事件独立读取与owner作用域。 [event-list.tsx](frontend/src/app/events/components/event-list.tsx#L52) | 线索/候选用户可见状态见04，24h组合指标见07。 | 筛选与排序、边界材料、跨owner及空/错/未知。 |
| [39 个人事件分析](docs/requirement/39-个人事件分析.md) · [设计](docs/design/39-个人事件分析.md) · [计划](docs/plan/39-个人事件分析.md) | 当前/固定修订、20条成员、事实、热度快照和带expected_revision/理由的纠错。 [event-detail.tsx](frontend/src/app/events/[eventId]/components/event-detail.tsx#L40) | 本轮事实差分见05；现有热度不能替代24h作者/帖子/新事实指标。 | 真实事实追溯、并发纠错、撤材料与热度不可比。 |
| [40 平台热榜快照](docs/requirement/40-平台热榜快照.md) · [设计](docs/design/40-平台热榜快照.md) · [计划](docs/plan/40-平台热榜快照.md) | 平台热榜快照与条目各20条，保留原生时间和数值字段。 [hotlist-workspace.tsx](frontend/src/app/hotlists/components/hotlist-workspace.tsx#L113) | 只提供榜单发现，不能替代关键词搜索或跨平台可比热度。 | 真实零/部分、原生单位、来源时间和快照不可变。 |
| [41 个人报告列表](docs/requirement/41-个人报告列表.md) · [设计](docs/design/41-个人报告列表.md) · [计划](docs/plan/41-个人报告列表.md) | 日/周、主题/日期URL筛选、主题选项翻页、20条报告游标及详情入口。 [report-list.tsx](frontend/src/app/reports/components/report-list.tsx#L79) | 本轮未确认新的必补功能；实际生成、自然日报与投递待验收。 | owner、冻结材料、最新修订、日期边界及真实连续生成。 |
| [42 刊期生产档案](docs/requirement/42-刊期生产档案.md) · [设计](docs/design/42-刊期生产档案.md) · [计划](docs/plan/42-刊期生产档案.md) | 内部刊期20条、类型/before_key和显式生产请求，操作ID与版本受理。 [edition-list.tsx](frontend/src/app/editions/components/edition-list.tsx#L23) | 本轮未确认新的必补功能；生产可靠性待验收，优先级在核心之后。 | 幂等/未知受理、窗口重复、失败恢复及任务追溯。 |
| [43 刊期内部详情与修订](docs/requirement/43-刊期内部详情与修订.md) · [设计](docs/design/43-刊期内部详情与修订.md) · [计划](docs/plan/43-刊期内部详情与修订.md) | 固定修订读取、编辑新修订、expected version/操作ID/理由及冲突重读。 [edition-detail.tsx](frontend/src/app/editions/components/edition-detail.tsx#L253) | 本轮未确认新的必补功能。 | 并发修订不覆旧刊、材料撤回、公开/内部边界。 |
| [44 来源连接与采集覆盖](docs/requirement/44-来源连接与采集覆盖.md) · [设计](docs/design/44-来源连接与采集覆盖.md) · [计划](docs/plan/44-来源连接与采集覆盖.md) | 来源能力/连接、Chrome、预算、覆盖窗口与详情/下一步操作。 [sources-workspace.tsx](frontend/src/app/sources/components/sources-workspace.tsx#L12) | 逐词未执行、增量口径、主题暂停保护见01/03/06；目录可见不等于准入或已采集。 | 真实连接版本、到期/风控、应执行分母/漏轮与来源失败隔离。 |
| [45 编辑来源配置](docs/requirement/45-编辑来源配置.md) · [设计](docs/design/45-编辑来源配置.md) · [计划](docs/plan/45-编辑来源配置.md) | 运营令牌内存持有、来源表单/CAS/操作ID、材料/运行读取与显式预览。 [editorial-source-manager.tsx](frontend/src/app/sources/editorial/components/editorial-source-manager.tsx#L95) | 首个来源所需配置先P0；其他编辑来源扩展后置，不默认启用外部预览。 | 准入/预算、草稿冲突、未知写入读回与显式请求边界。 |
| [46 任务记录](docs/requirement/46-任务记录.md) · [设计](docs/design/46-任务记录.md) · [计划](docs/plan/46-任务记录.md) | 20条任务游标、独立连续失败汇总、真实阶段/数量。 [job-history.tsx](frontend/src/app/jobs/components/job-history.tsx#L81) | 逐查询与更新计数见01/03；不能把active主题当执行中任务。 | 真实排队/执行/失败/取消、漏轮汇总及只读不发请求。 |
| [47 任务详情与恢复](docs/requirement/47-任务详情与恢复.md) · [设计](docs/design/47-任务详情与恢复.md) · [计划](docs/plan/47-任务详情与恢复.md) | 任务详情/刷新、持久取消与按资格手动重试，冻结任务输入和幂等恢复。 [job-detail.tsx](frontend/src/app/jobs/[jobId]/components/job-detail.tsx#L150) | 主题暂停已按06复核旧采集任务并拒绝旧任务重试；新代次使用新操作，各类未知结果仍需按原操作ID核对。 | 排队立即取消、执行中停止新请求、lease失效、原job重试与冻结配置。 |
| [48 公开许可与发布管理](docs/requirement/48-公开许可与发布管理.md) · [设计](docs/design/48-公开许可与发布管理.md) · [计划](docs/plan/48-公开许可与发布管理.md) | 运营令牌+会话、来源许可/CAS/理由、覆盖发布与媒体任务/纠错入口。 [publication-manager.tsx](frontend/src/app/publication/manage/components/publication-manager.tsx#L41) | 本轮未确认新的必补功能；全投影当前许可与撤回待逐链验证。 | 许可收紧、旧修订/媒体/导出失效、公开账号与个人源隔离。 |
| [49 运营工作区](docs/requirement/49-运营工作区.md) · [设计](docs/design/49-运营工作区.md) · [计划](docs/plan/49-运营工作区.md) | 健康/预算/维护、反馈/审计/字典、通知与评测面板已有实现。 [operations-workspace.tsx](frontend/src/app/operations/components/operations-workspace.tsx#L917) | 关键词/事件/自然运行评测覆盖缺口见11；不能以健康绿灯认定核心可用。 | 实际权限、持久读写、并发版本/操作ID及故障恢复。 |
| [50 模型配置与成本](docs/requirement/50-模型配置与成本.md) · [设计](docs/design/50-模型配置与成本.md) · [计划](docs/plan/50-模型配置与成本.md) | 模型能力配置/CAS/审计、原币种账本、失败/未知和熔断确认。 [model-manager.tsx](frontend/src/app/operations/models/components/model-manager.tsx#L265) | 本机实际模型名/会话未验收；兼容/付费默认禁用不代表没有相关源码，也不代表OpenRouter已启用。 | 获授权本机模型、切换只影响新任务、限流延后与费用未知不补零。 |

### 站点说明与个人来源

| 编号及文档 | 已实现（源码核对） | 待补齐 / 待核对 | 待验收 |
|---|---|---|---|
| [51 站点联系配置](docs/requirement/51-站点联系配置.md) · [设计](docs/design/51-站点联系配置.md) · [计划](docs/plan/51-站点联系配置.md) | 联系配置、文字/链接/二维码、版本/理由与显式保存。 [site-manager.tsx](frontend/src/app/site/manage/components/site-manager.tsx#L24) | 本轮未确认新的必补功能。 | 运营权限、并发冲突、公开白名单与配置读回。 |
| [52 意见反馈](docs/requirement/52-意见反馈.md) · [设计](docs/design/52-意见反馈.md) · [计划](docs/plan/52-意见反馈.md) | 5000字反馈、可选联系方式/地址、8MiB图片、内存草稿、显式操作ID提交与失败保留输入。 [feedback-form.tsx](frontend/src/app/feedback/components/feedback-form.tsx#L34) | 接口已有HMAC冷却与附件私有处理；草稿说明需与54对齐，不另立缺失API任务。 | 超限/MIME、重复提交、未知受理、冷却与运营删除附件。 |
| [53 关于](docs/requirement/53-关于.md) · [设计](docs/design/53-关于.md) · [计划](docs/plan/53-关于.md) | 静态关于页与个人非商业、公开/个人数据说明。 [page.tsx](frontend/src/app/about/page.tsx#L7) | 本轮未确认新的必补功能；边界措辞随00核对。 | 可读/键盘/窄屏；不适用远程加载/无权限状态。 |
| [54 隐私说明](docs/requirement/54-隐私说明.md) · [设计](docs/design/54-隐私说明.md) · [计划](docs/plan/54-隐私说明.md) | 会话、本机阅读数据、反馈HMAC/附件与撤许可说明。 [page.tsx](frontend/src/app/privacy/page.tsx#L6) | “反馈草稿保存在浏览器”未区分当前组件内存与持久本机记录，需写清刷新后行为并对齐实现。 | 逐句对照真实保存/清理/导出行为；静态页远程状态不适用。 |
| [55 使用条款](docs/requirement/55-使用条款.md) · [设计](docs/design/55-使用条款.md) · [计划](docs/plan/55-使用条款.md) | 代码/内容许可区分、来源/模型授权和费用边界说明。 [page.tsx](frontend/src/app/terms/page.tsx#L6) | 仍含额度到账预测及统一分发读法，须对齐冻结范围；不能将暂停能力描述为当前完整产品承诺。 | 对照00/08/10与当前实际开关；静态页远程状态不适用。 |
| [56 联系](docs/requirement/56-联系.md) · [设计](docs/design/56-联系.md) · [计划](docs/plan/56-联系.md) | 公开白名单联系配置、未配置空态与二维码读取。 [page.tsx](frontend/src/app/contact/page.tsx#L7) | 本轮未确认新的必补功能。 | 启停、配置失败、二维码当前版本及非公开字段不泄漏。 |
| [57 变更记录](docs/requirement/57-变更记录.md) · [设计](docs/design/57-变更记录.md) · [计划](docs/plan/57-变更记录.md) | 静态日期与变更条目，声明真实渠道/持续运行未通过。 [page.tsx](frontend/src/app/changelog/page.tsx#L6) | 欢迎介绍首页、固定头尾、Codex公告/统一分发与“移植进行中”混在记录中；核实实际发布事实，区分历史行为和当前状态，不把计划当发布。 | 逐项核对可证明的发布内容，不虚构验收或删除未经核实的真实历史。 |
| [58 个人来源](docs/requirement/58-个人来源.md) · [设计](docs/design/58-个人来源.md) · [计划](docs/plan/58-个人来源.md) | 本人RSS/网页/JSON来源、会话owner、默认关闭、CAS/操作ID及未知写入核对。 [personal-source-manager.tsx](frontend/src/app/sources/personal/components/personal-source-manager.tsx#L80) | 不能因URL存在认定可采；个人材料不自动公开的完整链路仍待验证。 | 保存不采集、启用准入/预算、跨owner、版本冲突保草稿及材料/job追溯。 |

## 可复用的验证入口与当前边界

这些测试文件可按影响范围复用；本轮暂停专项与后端工程检查结果见06。未设置的外部测试依赖、真实平台与自然时间分别报告，不能把跳过记作通过。

| 范围 | 已有测试入口 | 仍须补证 |
|---|---|---|
| 关键词/采集/覆盖 | [test_monitor_topics.py](backend/tests/integration/test_monitor_topics.py#L32)、[test_keyword_discovery.py](backend/tests/integration/test_keyword_discovery.py)、[test_content_collection_facts.py](backend/tests/integration/test_content_collection_facts.py#L16)、[test_coverage_metrics.py](backend/tests/integration/test_coverage_metrics.py#L23) | 暂停专项已补见06；逐词未执行、互斥增量分类及两轮真实对照仍待补证 |
| 事件/事实 | [test_event_signals.py](backend/tests/integration/test_event_signals.py#L41)、[test_event_clustering.py](backend/tests/integration/test_event_clustering.py#L34)、[test_event_fact_writer.py](backend/tests/integration/test_event_fact_writer.py#L20)、[test_event_corrections.py](backend/tests/integration/test_event_corrections.py#L29) | 独立质量评测、候选解释、本轮事实差分；不把单材料已有路径列为缺失 |
| 调度/恢复/通知 | [test_job_reliability.py](backend/tests/integration/test_job_reliability.py#L58)、[test_collection_jobs.py](backend/tests/integration/test_collection_jobs.py#L23)、[test_notifications_pipeline.py](backend/tests/integration/test_notifications_pipeline.py#L31) | 七日自然窗口、首次热点去重及本人实际接收 |
| 权限/清理/模型 | [test_resource_isolation.py](backend/tests/integration/test_resource_isolation.py#L41)、[test_content_dependency_cleanup.py](backend/tests/integration/test_content_dependency_cleanup.py#L18)、[test_ai_capability_routing.py](backend/tests/integration/test_ai_capability_routing.py#L44) | 全公开/媒体/导出/通知许可、独立库恢复、相关性与情感解耦及真实本机模型 |

## P1 / P2 与外部条件

- [ ] **P1 第二来源**：核心通过后，第二个已准入来源独立验证搜索、评论/回复、增量、事件质量与自然运行；别名/多语言和评论检索按真实噪声/漏报扩展，不自动扩平台或预算。
- [ ] **P1 平台目标**：X费用与有限试点、Instagram标签/账号与任意正文关键词可行性、Reddit官方批准、OpenRouter模型/辅助搜索的数据与费用分别保留；首个来源通过不能关闭这些缺口。
- [ ] **P1 提醒扩展**：首次热点闭环之后，再按证据推进重要新事实/异常增长、已读合同、更多免费渠道与事件报告。
- [ ] **P2 保留页面**：按12–58逐项处理多类型收藏/关注、情感观点、榜单历史/筛选、刊物分区、冻结入口与静态说明对齐；全站统计中的核心24h指标仍按P0。
- [ ] **P2 真实依赖与体验**：真实OAuth、验证邮件/SMTP、自然日报、管理读写、动态参数/导出、实际VoiceOver/侧栏地标、开发服务稳定性及榜单依赖分别验收。SMTP若选作首个提醒渠道，其必要配置/送达提前到P0；影响核心的隔离/许可/服务/无障碍阻断即时升级。

真实验收授权、首个平台与主题/语言、账号、本机模型名、通知渠道及对应配置尚需明确；未就绪只阻塞依赖它的真实请求，合同核对和受控验证可继续。凭据只写本机配置或独立浏览器目录，不进入本文件、聊天或提交。本轮没有新增外部请求授权，也未启用收费接口或云端模型。

正式验收前，固定同账号/来源/查询/排序/窗口的对照方法与分层样本，分开校准与保留样本；质量、成功率和延迟阈值待基线校准后冻结。首次可检索时间不可得就标未知，公开可见集合不扩大为全网召回。

准入、预算、owner隔离、证据可读、身份去重、停止与恢复是硬门槛；停止后出现新请求等失败必须修复并重验受影响阶段。受控时间推进不替代七日自然观察，手动触发不替代自然调度；自然期未出现的异常由受控验证单独补证，不伪造自然故障。至少两轮自然成功、有事件更新且提醒本人实际收到；无自然提醒则继续观察，七日最低长度不是已达到长期SLA。

源码核对、工程检查、固定样本、独立环境、真实平台、自然时间、用户体验和独立审查分别报告。实现改动按AGENTS执行对应检查；本轮代码与固定样本结论仅覆盖06暂停保护及受影响链路，不能据此关闭01–58其他缺口或真实业务验收。
