# 执行计划审核、依赖与需求覆盖

更新：2026-09-26，按当前工作树重新核对。Design/PRD 001—007承担总纲与M1—M6 Epic；逐Issue计划从001连续到057，编号是身份，不是执行顺序。本文是索引，不是总Plan或Epic。计划中的新增表、接口、文件、测试均为实施合同，尚未创建或运行，不代表实现/验收完成。

## 本次审核与修正

| 影响执行的问题 | 修正 |
|---|---|
| 旧30卡主要继承旧TASK，遗漏配置到用户页面的链路 | 补031—041：受理/调度、预算周期、到期事实、逐来源采集、分析流水线及覆盖/热榜/内容页面 |
| 开工再定DTO/表/文件，使执行者会实现不同功能 | 各卡写明文件职责、接口/错误、唯一键/事务、重放、测试场景；Design同步决策 |
| 002包配置/调度/重试，007包搜索/评论，030包报告/数据 | 分为002/031/032、007/038、030/045；保留编号，收窄职责并改名 |
| 只用Job算覆盖会漏未受理窗口，时效分母不清 | 033到期事实→004只读API→006计算→034页面→009同窗判定 |
| 报告设置/水位、事件页面、通知配置、检索、恢复无人负责 | 补042/043/044/052/051，分别提供可验收产物 |
| 准入研究被当成平台实现 | 046—050逐来源准入Issue明确blocked；获准能力后再建实施Plan，AC-007-003不关闭 |
| 通用G0—G6不能指导实现 | 补具体CHK→SPEC→测试/操作→预期断言，通用门禁不替代功能验收 |

## 执行和关闭规则

1. 读源PRD、Design和本卡并核对代码漂移。`depends_on`是本卡消费的技术交付，不要求上游所有产品连续窗口已通过；真实输入门槛另在卡中明确。
2. `planned`只说明计划已定义。就绪要求关键设计、具体规格/文件/测试和技术前置齐备；真实账号/费用/渠道条件未满足时只阻塞对应真实步骤。未冻结能力契约的研究卡保持blocked。
3. 按具体CHK先保存失败/基线，再实现、运行适用门禁和局部真实验收。带真实条件的Plan不能仅因单测通过标completed；技术通过、产品待观测时保持in_progress，在BACKLOG分别记录，后续只消费已证实技术产物。
4. 避免验收环：001配置技术产物供031，首周期在009汇合；014/015/016技术产物供042，四卡汇合M3产品结论。普通实现细节无需重复用户确认。
5. 历史046 S03是旧全局HTTP响应契约的真实通过门槛，新Plan046是Reddit准入研究，两者不是同一份计划；056负责该契约的现行维护回归，不替代历史门槛。保留库DDL先满足051同库/Schema版本恢复证据。新文件只在所属Issue实施时创建。

## Issue与技术依赖

001—030为既有修订卡，031—052为新增/拆分卡，053—057接续审计缺口。026、046—050、053—054、057为blocked，其余planned；账号、渠道、真实样本和长窗另按各卡放行。表内Design编号对应同号PRD（001→PRD001，002→PRD002，依此类推），因而每行同时给出Design/PRD映射；现行文件路径由链接确定。

| Plan | 唯一交付 | Design | 技术前置 |
|---|---|---|---|
| [001](001-监控主题与关键词规则执行计划.md) | 主题配置/规则/版本 | 002 | 既有身份/主题底座 |
| [002](002-来源预设版本与预算配置执行计划.md) | 版本化策略与预算配置 | 002 | 既有连接/预算底座 |
| [003](003-空榜与失败时间桶执行计划.md) | 空快照与原桶失败 | 002 | 033 |
| [004](004-采集覆盖查询执行计划.md) | 覆盖只读API | 002 | 033、003、005 |
| [005](005-标注异常与有效结论执行计划.md) | 有效/失败/无效标注 | 002 | 既有分析底座 |
| [006](006-逐来源时效与分母统计执行计划.md) | 时效与分母API/CLI | 002 | 004、005 |
| [007](007-HN关键词采集与重放执行计划.md) | HN搜索持久化/重放 | 002 | 031、033 |
| [008](008-六榜逐榜核对执行计划.md) | 六榜采集/排名/命中 | 002 | 003、031、033 |
| [009](009-M1连续72小时与运行库保留执行计划.md) | M1同窗72小时验收 | 002 | 001—008、031—041、051适用交付 |
| [010](010-MediaCrawler三类版本证据执行计划.md) | B站版本校验/Job关联 | 003 | 002 |
| [011](011-B站风控分类与人工恢复执行计划.md) | 风控分类/停用/恢复 | 003 | 010、031、032 |
| [012](012-B站本人账号低频真实采集执行计划.md) | 视频/同轮评论低频链路 | 003 | 010、011、034、040、041 |
| [013](013-B站72小时无风控运行执行计划.md) | M2独立72小时验收 | 003 | 006、010—012、051 |
| [014](014-事件候选与稳定身份执行计划.md) | 候选/确认/事件成员 | 004 | 005、040、041 |
| [015](015-事件人工合并拆分与审计执行计划.md) | 人工修订写API/审计 | 004 | 014 |
| [016](016-事件热度与升温复算执行计划.md) | 热度快照/升温 | 004 | 014、015 |
| [017](017-分析质量抽检执行计划.md) | 评论情感/观点与质量抽检 | 005 | 005、040、041 |
| [018](018-日报生成降级与存档执行计划.md) | 日报/降级/存档/Web | 005 | 017、043、004 |
| [019](019-周报冻结输入与存档执行计划.md) | ISO周报告/日报合计/Web | 005 | 018、043；事件另等042 |
| [020](020-Obsidian日报笔记执行计划.md) | 安全写入/日报笔记 | 005 | 018 |
| [021](021-Obsidian事件与精选帖执行计划.md) | 精选帖→事件→补链 | 005 | 020、042 |
| [022](022-Obsidian周报与主题笔记执行计划.md) | 周报/主题笔记 | 005 | 019、020；事件链接另等021 |
| [023](023-知识库问答执行计划.md) | 回答/Web/CLI/问答笔记 | 005 | 020、052 |
| [024](024-投递身份与未知状态执行计划.md) | 状态机/unknown/处置服务 | 006 | 018 |
| [025](025-SMTP报告邮件执行计划.md) | SMTP适配/真实送达 | 006 | 024、044、019；真实条件待定 |
| [026](026-飞书渠道真实送达执行计划.md) | 飞书适配/真实送达 | 006 | 024、044、019；暂缓 |
| [027](027-突发告警执行计划.md) | 规则/一小时阈值/冷却/投递 | 007 | 016、017、024、044；真实渠道等025或026 |
| [028](028-指定账号追踪执行计划.md) | 稳定账号/公共扫描/页面 | 007 | 029、031、033、041；实际能力另行准入 |
| [029](029-后续来源准入规则执行计划.md) | 准入记录与执行前校验 | 007 | 002 |
| [030](030-报告Markdown与PDF导出执行计划.md) | 报告MD/PDF/下载 | 007 | 018、019 |
| [031](031-来源调度与手动采集入口执行计划.md) | 调度/立即运行/暂停恢复 | 002 | 001、002、033 |
| [032](032-人工重试预算周期执行计划.md) | 人工/自动重试预算时钟 | 002 | 002 |
| [033](033-到期窗口与采集计量事实执行计划.md) | 到期/计量事实与DTO | 002 | 002 |
| [034](034-来源覆盖工作台执行计划.md) | 窗口筛选/状态/下钻 | 002 | 004、006 |
| [035](035-GoogleNews搜索采集执行计划.md) | Google News逐来源链路 | 002 | 031、033 |
| [036](036-SearXNG新闻搜索执行计划.md) | SearXNG引擎/持久化 | 002 | 031、033 |
| [037](037-36Kr快讯关键词采集执行计划.md) | 36Kr快讯主题采集 | 002 | 031、033 |
| [038](038-HN评论分页与旧帖新回复执行计划.md) | HN父链/分页/旧帖回复 | 002 | 007、032、033 |
| [039](039-热榜历史快照与排名页面执行计划.md) | 历史快照API/热榜页 | 002 | 003、008 |
| [040](040-Codex分析调度与批处理执行计划.md) | 分析队列/批处理/恢复 | 002 | 002、005 |
| [041](041-内容评论与分析结果阅读执行计划.md) | 内容/评论/标注阅读 | 002 | 005、038、040 |
| [042](042-事件查询与人工修订页面执行计划.md) | 事件读取/时间线/修订交互 | 004 | 014、015、016 |
| [043](043-报告配置与冻结调度执行计划.md) | monitor_topics单一设置源/冻结/水位/调度 | 005 | 005、040 |
| [044](044-推送目标与投递记录页面执行计划.md) | 目标配置/历史/人工确认 | 006 | 024 |
| [045](045-原始数据CSV与JSON导出执行计划.md) | 原始数据CSV/JSON/下载 | 007 | 041 |
| [046](046-Reddit官方OAuth来源准入执行计划.md) | Reddit准入与能力合同 | 007 | 029；OAuth/许可/预算未定 |
| [047](047-X官方API来源准入执行计划.md) | X准入/费用/能力合同 | 007 | 029；OPEN-001-104未定 |
| [048](048-小红书来源准入执行计划.md) | 小红书准入与能力合同 | 007 | 029；授权/组件/能力未定 |
| [049](049-抖音来源准入执行计划.md) | 抖音准入与能力合同 | 007 | 029；授权/组件/能力未定 |
| [050](050-微信公众号来源准入执行计划.md) | 公众号逐路由准入合同 | 007 | 029；范围/路由/授权未定 |
| [051](051-运行库保留与恢复演练执行计划.md) | 同库/Schema可恢复证据 | 001 | 既有备份底座/隔离恢复目标 |
| [052](052-知识资料检索与引用快照执行计划.md) | pg_trgm检索/引用快照 | 005 | 041、018；事件另等042 |
| [053](053-微博登录来源准入研究执行计划.md) | 微博登录逐能力准入研究，blocked | 007 | 029；本人授权/许可/预算未定 |
| [054](054-知乎登录来源准入研究执行计划.md) | 知乎登录逐能力准入研究，blocked | 007 | 029；OPEN-001-107未定 |
| [055](055-证据保留删除与谱系回归执行计划.md) | 生命周期cleanup-once/删除/谱系共享回归 | 001 | 051 |
| [056](056-HTTP契约客户端生成与CI门禁执行计划.md) | HTTP契约/OpenAPI→客户端/CI维护回归 | 001 | 历史046 S03门槛 |
| [057](057-通用网页与浏览器采集底座回归执行计划.md) | webpage.collect/Firecrawl/Browser/probe冻结回归，blocked | 001 | 055、056；目标来源与授权未定 |

## 执行顺序

当前仅修订文档，不据此恢复业务实施。获准后一次完成一个Issue，同组不代表并发改共享文件。

1. 推荐M1技术顺序为 `002 → 033 → 031`；同时完成001、005、032、003的**受控技术切片**，不把其真实G5当作下游实施前置。
2. 随后007、035—037、008、040、004；再038、039、041、006、034。相同共享文件按下节串行交接，页面和来源真实联验写009或Acceptance。
3. 最终库/Schema版本重做051恢复门槛，核对001—008、031—041的局部证据后启动009的共同72小时窗口。010→011可技术准备，010三类版本证明必须先于012；012需本人核查后低频，013另起M2窗口。
4. 014→015→016→042；017与043→018→019；018→020→021/022；018/041→052→023。事件产品数据必须等M3。
5. 018→024→044→025；026暂缓。事件/分析与渠道齐备后027；029→028公共能力及046—050准入；030/045独立验文件。

## FR与NFR覆盖

| 需求 | 承接Issue |
|---|---|
| FR-002-001 | 001配置/规则，031运行入口 |
| FR-002-002 | 002策略、031调度/手动、032重试、033漏窗 |
| FR-002-003 | 007/035/036/037逐来源，033计量，041阅读 |
| FR-002-004 | 038评论，041父链阅读 |
| FR-002-005 | 005有效性，040持续处理，041结果，006/009时效 |
| FR-002-006 | 003空/失败，008六榜，039历史页面，040命中分析 |
| FR-002-007 | 033→004→006→034，009连续对账 |
| FR-003-001/002 | 010版本、011停用、012同轮视频/评论，013连续性；复用M1配置、计量、页面 |
| FR-004-001/002 | 014事件、015修订、016热度、042页面 |
| FR-005-001 | 043在monitor_topics既有字段上的唯一报告设置 |
| FR-005-002 | 017评论情感/观点与抽检，040基础批处理 |
| FR-005-003/004/005 | 043冻结/调度，018日报/阅读，019周报/合计，事件区块等042 |
| FR-005-006 | 020日报/安全写入，021事件/精选帖，022周报/主题 |
| FR-005-007 | 052检索，023回答/界面/问答笔记 |
| FR-006-001/002 | 044目标设置，024状态，025 SMTP，026飞书 |
| FR-007-001 | 030报告MD/PDF，045原始CSV/JSON |
| FR-007-002/003 | 027告警；028公共追踪链路+获准作者能力实现（尚缺实际平台承接） |
| FR-007-004 | 029执行准入，046—050/053/054研究；X既有x_api/x_user_lookup归047代码台账；**平台采集实现未就绪，获准后从058另建实施卡** |
| NFR-001-101/102 | 各来源身份/时间/版本；033/004证据、014—016事件、018/019/052冻结引用，055证据删除与谱系 |
| NFR-001-103、NFR-002-001、NFR-003-002、NFR-005-001 | 006计算、009/013观察；043/018生成、025/026送达分别计时 |
| NFR-001-104/105/112 | 031/032/033调度/重试/缺口，各执行器重放，024unknown，051恢复，056契约CI |
| NFR-001-106/107 | 002预算/版本，031/032执行，040模型隔离，各来源白名单，020文件，024—030/044/045权限，055证据生命周期、056契约、057通用网页/浏览器 |
| NFR-001-108 | 017每主题100条，005异常不计有效 |
| NFR-001-109、NFR-005-002 | 020基础，021/022/023各类笔记 |
| NFR-001-110、NFR-003-001 | 010—013，048/049独立准入，不借B站结论 |
| NFR-001-111 | 033/004/006/034与逐来源对账，009/013长窗 |

## 产品AC证据汇合

| AC | 必需证据与汇合Issue |
|---|---|
| AC-002-001/003 | 007真实搜索/重放，038十帖父链/分页/旧帖新回复，041阅读；009整体核对 |
| AC-002-002/004 | 001/031首周期，007/035/036/037四入口/时间/去重，033/004/034缺口，006指标；009共窗 |
| AC-002-005 | 003/008/039六榜快照/空/失败/排名/命中，040相关性；009按T0冻结的实际间隔逐榜计算应到期桶及≥90%阈值，1800秒才是144/130 |
| AC-002-006/007/008 | 004/034状态对账、032重试预算、005/040有效标注、006分母；009连续三天 |
| AC-003-001/003 | 010版本、011受控风控/恢复、012本人真实1词2视频/同轮评论 |
| AC-003-002/004 | 012计量、006口径、013独立72小时无风控/900秒/95% |
| AC-004-001 | 014/015/016/042真实三平台、人工修订、可复算升温 |
| AC-005-001 | 017每主题100条人工判定，相关性≥90%/情感≥80%，失败/无效留分母 |
| AC-005-002/005/007 | 018三自然日日报、10条原帖、数字/缺口/降级/Web；019周报另验 |
| AC-005-003 | 019真实周一、ISO周、冻结每日版本及同口径合计；042事件区块条件独立 |
| AC-005-004 | 052真实检索+023十问至少八问正确、出处可打开、问答笔记 |
| AC-005-006/008 | 020三日日报、021事件/精选帖、022周报/主题、023问答；真实vault/用户区/无悬链 |
| AC-006-001 | 024/044幂等unknown；025/026分别真实核收3日日报+周一周报及时送达及降级 |
| AC-007-001/002 | 027真实告警/冷却/核收；028及获准作者能力分别新帖/页尾/缺口 |
| AC-007-003 | 029、046—050、053/054只供准入；后续各获准能力从058起建实施卡并有真实采集才逐项判定，当前未就绪 |
| AC-007-004 | 030 MD/PDF、045 CSV/JSON分格式核对权限、版本、引用、字段和失败 |

### 逐条 AC → Plan 责任表

表内前置/实施卡只贡献本卡证据；产品结论仍在同号 Acceptance 汇合。研究卡与共享技术卡不单独关闭产品 AC。

| AC | 必需 Plan 与本卡责任 |
|---|---|
| AC-002-001 | 007 HN搜索/重放、038评论父链、041阅读、009共同核对 |
| AC-002-002 | 001主题、002策略、031调度、033到期、007/035/036/037四关键词来源、004/006/034覆盖与时效、009长窗 |
| AC-002-003 | 038十帖分页/旧帖新回复、041父链阅读、009汇合 |
| AC-002-004 | 007/035/036/037逐来源身份/时间/缺口、033计量、004/034展示、009共同核对 |
| AC-002-005 | 003空/失败桶、008六榜采集、039历史排名、040命中分析、009按冻结间隔逐榜72小时 |
| AC-002-006 | 033计量、004API、005异常、032重试、034页面、009三天对账 |
| AC-002-007 | 032人工/自动重试预算周期、009真实运行汇合 |
| AC-002-008 | 005有效性、040持续分析、041阅读、006时效分母、009十来源60分钟核对 |
| AC-003-001 | 010三版本、011风控边界、012本人账号视频/同轮评论及分析、034/040/041复用 |
| AC-003-002 | 010—012版本/真实窗口、006指标、013三天覆盖与缺口 |
| AC-003-003 | 010版本、011停用/人工恢复、012恢复后低频真实采集 |
| AC-003-004 | 010—012放行、006指标、051同版本恢复结论、013独立72小时 |
| AC-004-001 | 014候选、015人工修订、016热度、042查询/修订页面 |
| AC-005-001 | 005/040有效性与分析流水线、017每主题100条人工质量抽检 |
| AC-005-002 | 043唯一设置/冻结、018三自然日日报与存档 |
| AC-005-003 | 043唯一设置/冻结、018日报版本、019真实周一ISO周/合计；042事件区块另验 |
| AC-005-004 | 052检索/引用快照、023十问回答/可打开出处 |
| AC-005-005 | 043冻结、018日报数字/引用/降级，019周报对账 |
| AC-005-006 | 020日报、021事件/精选帖、022周报/主题、023问答笔记 |
| AC-005-007 | 043调度、018日报与Web阅读、019周报与Web阅读 |
| AC-005-008 | 020—023真实vault用户区/无悬链 |
| AC-006-001 | 024投递状态/unknown、044目标和可查记录、025 SMTP/026飞书各自真实核收 |
| AC-007-001 | 027真实告警/冷却、024/044状态与记录、获准渠道025或026 |
| AC-007-002 | 028指定账号公共链路、029及获准来源未来实施卡；真实作者能力待建卡 |
| AC-007-003 | 029通用准入、046—050与053/054逐来源研究；获准后从058另建逐能力实施卡并真实验收 |
| AC-007-004 | 030报告MD/PDF、045原始CSV/JSON，分格式权限、版本、引用与失败 |

### 已交付共享底座台账

“已有”仅指代码/表存在；现有验证栏记可核查的历史证据入口与本轮边界，不代表新Issue已通过。所有行维护责任为所属领域或路径所有者；发生新行为变更时按对应Issue或新Issue实施。审计第3节列出的孤儿实现均在此登记。

| 模块/表 | 代码位置 | 现有验证证据 | 维护责任 | Design/PRD映射 |
|---|---|---|---|---|
| 身份、会话、初始化/重置命令；`identity_users`、`identity_sessions` | `backend/app/identity/`、`api/routers/identity.py`、`cli/commands.py` | `test_application_security.py`及HTTP契约测试现有；本轮未重跑，真实部署身份核对归051/056 | identity领域；HTTP契约变更归056 | 001 / PRD001 §2、NFR-001-107 |
| 公开首页、品牌、登录/初始化Web | `frontend/src/app/page.tsx`、`app/components/`、`components/brand/`、`app/login/`、`app/register/`、`components/auth/` | 现有页面代码；本轮未浏览器重验，图标现行母版为`app/icon.png` | 前端路由/brand/auth所有者；新增行为另建卡 | 001 / PRD001 §2、NFR-001-107 |
| 主题工作台及共享状态页 | `frontend/src/app/events/`、`components/monitors/topic-list.tsx`、`components/system/page-state.tsx` | 已有入口；业务事件查询待042，不能把首页当事件验收 | 前端路由；事件业务归042 | 001/004 / PRD001、FR-004-001 |
| HTTP路由汇总、文档、健康、异常、请求ID；同源代理/请求 | `backend/app/api/{router,docs,middleware,exception_handlers,dependencies}.py`、`api/routers/health.py`、`frontend/src/{request,proxy}.ts`、`app/api/[[...path]]/route.ts`、`app/health/route.ts` | 历史046 S03真实门槛见Design001 §5；`test_http_contract.py`/`test_http_contract_046.py`现有，本轮未重跑 | api/core与Web传输；056回归 | 001 / NFR-001-104/107 |
| Web错误/加载入口与生成客户端 | `frontend/src/app/{layout,error,global-error,not-found,loading}.tsx`、`src/api/` | 现有代码/生成物；同提交生成漂移待056验证 | Web根布局与056契约 | 001 / NFR-001-104/107 |
| 数据库、配置、日志与应用工厂 | `backend/app/db/{base,metadata,session}.py`、`core/{config,errors,schemas,logging}.py`、`main.py` | 现有架构/配置测试；本轮未重跑，配置按各行为卡扩展 | db/core；配置字段由所属Issue修改并串行 | 001 / NFR-001-104/107 |
| Kafka/租约/Outbox/预算；`jobs`、`job_stage_attempts`、`job_attempts`、`outbox_messages`、`processed_messages`、`resource_budget_policies`、`resource_budget_windows`、`resource_budget_reservations`、`resource_component_policies`、`resource_usage_attempts` | `backend/app/jobs/`、`worker/{app,execution,messaging,scheduler}.py`、`cli/jobs.py` | 现有`test_worker_messaging.py`、`test_worker_execution.py`、预算测试；开发库约4小时不是72小时 | jobs/worker；002/031—033/040/051按职责串行 | 001/002 / NFR-001-104/106/112、FR-002-002/007 |
| `evidence_retention_policies`、`evidence_resources`、`evidence_deletions`、`evidence_cleanup_targets`、`provenance_manifests`、`provenance_manifest_inputs`、MinIO缓存/对象及`lifecycle cleanup-once` | `backend/app/evidence/{models,schemas,services}.py`、`evidence/adapters/{cache,minio}.py`、`cli/commands.py` | `test_data_lifecycle.py`、`test_provenance_replay.py`现有；本轮未做真实对象清理回归 | evidence；055回归，051只管恢复 | 001 / NFR-001-101/107 |
| 单网页持久任务与页面捕获入口 | `backend/app/content/collection.py`、`worker/app.py`、`frontend/src/app/content/components/webpage-capture-form.tsx` | `test_webpage_persistence.py`、表单测试现有；不代表任一平台接入 | content/worker/Web；057冻结回归 | 001/002 / NFR-001-107 |
| Firecrawl、HTTP来源与安全目标、Browser运行时 | `backend/app/sources/adapters/{http_source,firecrawl,browser_runtime,web_targets}.py`、`connections/adapters/local_secrets.py` | `test_webpage_adapter.py`、`test_browser_adapter.py`、`test_browser_collection.py`现有；G4-002换版旧写仍待验 | sources/connections；057冻结回归 | 001/002 / NFR-001-107/110 |
| 浏览器Server、seccomp、代理与镜像网络 | `backend/browser/{server.js,seccomp.json,squid.conf}`、`backend/Dockerfile`、`docker-compose.yml` | S03运行底座历史技术验证；真实平台出口/请求计量未证明 | browser/部署；057 | 001/002 / NFR-001-104/107 |
| `sources probe-webpage`、`probe-browser` | `backend/app/cli/commands.py` | CLI探针只能证明本机通路，不能提升来源状态；本轮未重跑 | cli/sources；057 | 001/002 / NFR-001-107 |
| `version`、`identity reset-password`、preset/连接CLI | `backend/app/cli/commands.py` | 现有命令；本轮未重跑；来源预设/浏览器状态受002/010—012/057约束 | cli与所属identity/connections域 | 001/002/003 / NFR-001-107/110 |
| 来源、Job与内容用户页面 | `frontend/src/app/{sources,jobs,content}/` | 现有列表/详情测试；034/041新增业务阅读，单网页表单归057 | 各路由所有者；034/041/057 | 002 / FR-002-003/007 |
| 后端/契约/前端/运行CI | `.github/workflows/{backend,contract,frontend,runtime}.yml` | 现有工作流；本轮未触发远端最终CI，运行门禁不等于来源旅程 | CI；056 | 001 / NFR-001-104/107 |
| Compose底座与API/Web镜像 | `docker-compose.yml`、`backend/Dockerfile`、`frontend/Dockerfile` | 现有编排；051需同库恢复，057浏览器出口另验 | 部署；051/056/057按变更范围 | 001 / NFR-001-104/106/107 |
| X只读适配与作者解析 | `backend/app/sources/adapters/{x_api,x_user_lookup}.py` | 既有MockTransport受控测试；无凭据/月度上限，零真实请求 | sources；047准入代码台账，获准后新实施卡 | 007 / FR-007-004、AC-007-003 |

其余现有43张DDL表与领域模块的业务归属按上方Issue、Design001领域表及各卡SPEC管理；身份、证据/谱系和预算/任务共享表已在本台账显式点名。共享台账不代表它们满足真实来源或产品AC。

### 共享文件所有权与串行顺序

同一文件只允许当前Issue的明确修改进入该切片；下一个Issue在最终版本上重读、补失败测试并接续，不并行覆盖。若 Schema 或生成客户端版本变化，051及B/C/F门禁按最终版本重新核对。

| 共享文件 | 首要责任与串行顺序 | 合并/验收门槛 |
|---|---|---|
| `backend/database/schema.sql`及ORM/元数据 | db为DDL守门；033→032→各业务建表卡（043不建report_schedules）→030/045/052→055 | 每卡新空库原子初始化、ORM/DDL一致；保留库先051同版本恢复 |
| `backend/app/worker/scheduler.py` | worker调度；033→031→040→043→019→052 | 到期/Job/Outbox唯一键、旧kind回归、未知kind先失败 |
| `backend/app/worker/app.py`、`execution.py` | worker运行；031→040→019→030→045→052→057（后者冻结） | 每个kind注册、硬截止/取消/未知退出、连续offset回归 |
| `backend/app/content/discovery.py`、`discovery_execution.py` | content发现；007→035→036→037，再038评论、008热榜独立文件 | 上一卡身份/分页/计量测试转绿后交接，不混源覆盖 |
| `backend/app/core/config.py` | core配置；002→031→040→030→045→052→057 | 新kind先以未知kind失败测试证明门禁，登记截止后全kind回归 |
| `backend/app/monitors/services.py`、`reports/services.py` | monitors设置/报告冻结；001→031→043→018→019 | `monitor_topics`三字段唯一设置源，冻结版本不漂移 |
| `backend/app/api/router.py`、`dependencies.py` | api装配；各业务路由按Issue顺序，056最后核契约 | operation_id/错误模型/权限、运行OpenAPI与客户端同批 |
| `frontend/src/api/`、`src/request.ts` | 业务Issue只生成/消费；056维护传输/生成门禁 | 从同提交运行Schema重生、C/F与CI最终结果，无手改 |
| `.github/workflows/*.yml`、`docker-compose.yml` | 056维护CI、051恢复、057浏览器出口 | 文件差异单卡审查，不删除持久卷；最终Schema/客户端/运行门禁复验 |

### 逐卡文件变更分类

本表是每卡主文件的新增/修改/生成清单；`B/`=`backend/app/`，`F/`=`frontend/src/`，`DDL`=`backend/database/schema.sql`，`Client`=`frontend/src/api/`。实际实现须以卡内SPEC/CHK列出的其余精确测试文件一起交付；条件项只有需求确需时才改，未列类型为“无”。研究卡只修改Design/PRD/索引，不预建适配器。表内均是计划责任，不表示本轮已修改代码。

| Plan | 新增主文件 | 修改主文件 | 生成 |
|---|---|---|---|
| 001 | 无 | `B/monitors/{schemas,services,models}.py`、`B/api/routers/monitor_topics.py`、`F/components/monitors/topic-settings-fields.tsx`、主题表单/编辑器 | Client |
| 002 | 无 | `B/connections/{presets,schemas,services}.py`、`B/jobs/services.py`；必要时DDL | Client（DTO变化时） |
| 003 | 无 | `B/content/{hotlist,hotlist_execution,models,schemas}.py`、`B/sources/adapters/rsshub_hotlist.py`、DDL | 无 |
| 004 | `B/api/routers/collection_coverage.py` | `B/jobs/{coverage,schemas}.py`、`B/content/services.py`、`B/analysis/services.py`、API注册/依赖 | Client |
| 005 | 无 | `B/analysis/{services,schemas,models}.py`、DDL | 无 |
| 006 | `B/jobs/metrics.py` | `B/jobs/{schemas,coverage}.py`、`B/analysis/services.py`、`B/api/routers/collection_coverage.py`、`B/cli/jobs.py` | Client |
| 007 | 无 | `B/sources/adapters/hackernews.py`、`B/connections/presets.py`、`B/content/{discovery,discovery_execution}.py` | 无 |
| 008 | 无 | `B/sources/adapters/rsshub_hotlist.py`、`B/content/{hotlist,hotlist_execution}.py`、`B/connections/presets.py` | 无 |
| 009 | 无 | M1 Acceptance证据（产生后） | 无 |
| 010 | 无 | `B/sources/adapters/mediacrawler.py`、`B/connections/presets.py`、`B/jobs/{schemas,models}.py`、DDL | 无 |
| 011 | 无 | `B/sources/adapters/mediacrawler.py`、`B/connections/{services,schemas}.py`、`B/worker/{execution,app}.py`、来源连接API/页面 | Client（契约变化时） |
| 012 | 无 | `B/sources/adapters/mediacrawler.py`、`B/content/{discovery_execution,comments_execution}.py`、`B/connections/presets.py` | 无 |
| 013 | 无 | M2 Acceptance证据（产生后） | 无 |
| 014 | `B/events/{__init__,models,schemas,services,clustering}.py` | DDL、`B/db/metadata.py`、`B/worker/{scheduler,app}.py` | 无 |
| 015 | `B/events/revisions.py`、`B/api/routers/events.py` | `B/events/{models,schemas,services}.py`、DDL、API注册/依赖 | Client |
| 016 | `B/events/heat.py` | `B/events/{models,schemas,services}.py`、DDL、`B/worker/{scheduler,app}.py` | 无 |
| 017 | 无 | `B/analysis/{schemas,prompts,services,models}.py`、DDL | 无 |
| 018 | 无 | `B/reports/{services,schemas,render,prompts,models}.py`、`B/api/routers/reports.py`、报告列表/详情页面 | Client |
| 019 | `B/reports/weekly.py` | `B/reports/{services,schemas,render}.py`、`B/worker/{app,scheduler}.py`、报告页面 | Client（契约变化时） |
| 020 | 无 | `B/knowledge/{obsidian,services,schemas,models}.py`、`B/worker/scheduler.py`；必要时DDL | 无 |
| 021 | `B/knowledge/objects.py` | `B/knowledge/{schemas,services,obsidian}.py` | 无 |
| 022 | 无 | `B/knowledge/{objects,schemas,services,obsidian}.py` | 无 |
| 023 | `B/knowledge/answers.py`、`B/api/routers/knowledge.py`、`F/app/knowledge/page.tsx` | `B/knowledge/{models,schemas}.py`、DDL、Worker/调度/CLI/API注册 | Client |
| 024 | 无 | `B/notifications/{models,schemas,services,executor}.py`、`B/worker/{app,scheduler}.py`、DDL | Client（契约变化时） |
| 025 | `B/notifications/smtp.py` | `B/notifications/{executor,schemas}.py`、`B/core/config.py`、`backend/.env.example`、Worker装配 | 无 |
| 026 | 无 | `B/notifications/{feishu,executor,services}.py`（解阻后按故障） | 无 |
| 027 | `B/notifications/alerts.py`、`B/api/routers/alerts.py`、`F/app/alerts/page.tsx` | notifications模型/Schema、DDL、Worker/调度/API注册 | Client |
| 028 | `B/monitors/tracking.py`、`B/api/routers/followed_accounts.py`、`F/app/accounts/page.tsx` | monitors Schema/服务、`B/content/discovery_execution.py`、Worker/调度、DDL | Client |
| 029 | 无 | `B/connections/{presets,schemas,services,catalog}.py` | 无 |
| 030 | `B/reports/exports.py`、`B/api/routers/report_exports.py` | reports模型/Schema、DDL、`B/core/config.py`、`B/worker/{app,execution}.py`、报告详情页面 | Client |
| 031 | `F/app/monitors/[topicId]/components/topic-run-actions.tsx` | `B/api/routers/monitor_topics.py`、`B/monitors/{schemas,services}.py`、`B/worker/scheduler.py`、主题编辑器 | Client |
| 032 | 无 | `B/jobs/{models,schemas,services,execution}.py`、DDL、发现/评论/热榜执行器、Job详情页面 | Client |
| 033 | `B/jobs/coverage.py` | `B/jobs/{models,schemas}.py`、DDL、发现/评论/热榜计量服务 | 无 |
| 034 | `F/app/sources/components/{source-coverage-panel,coverage-window-table,coverage-window-detail}.tsx` | `F/app/sources/page.tsx` | 无 |
| 035 | 无 | `B/sources/adapters/rss.py`、`B/sources/contracts.py`、`B/content/{discovery,discovery_execution}.py`、`B/connections/presets.py` | 无 |
| 036 | 无 | `B/sources/adapters/web_search.py`、`B/connections/presets.py`、`B/content/{discovery,discovery_execution}.py` | 无 |
| 037 | 无 | `B/sources/adapters/rss.py`、`B/connections/presets.py`、`B/content/{discovery,discovery_execution}.py` | 无 |
| 038 | 评论显式受理路由（卡内SPEC） | `B/content/{comments,comments_execution,services}.py`、`B/connections/presets.py`、API注册 | Client |
| 039 | `F/app/hotlists/page.tsx`及该路由组件 | `B/api/routers/hotlists.py`、`B/content/hotlist.py`、API注册 | Client |
| 040 | 无 | `B/analysis/{services,schemas,prompts}.py`、`B/ai/adapters/codex_app_server.py`、`B/worker/{scheduler,app}.py` | 无 |
| 041 | 评论/标注详情组件 | `B/content/{schemas,services}.py`、`B/api/routers/content_records.py`、内容列表/详情页面 | Client |
| 042 | `B/api/routers/events.py`、`F/app/events/[eventId]/page.tsx`及详情组件 | `F/app/events/components/events-workspace.tsx`、API注册/依赖 | Client |
| 043 | 无 | `B/monitors/{services,schemas}.py`、`B/reports/services.py`、`B/worker/scheduler.py`、主题编辑器；不新增report_schedules | Client（主题契约变化时） |
| 044 | `B/api/routers/notifications.py`、`F/app/notifications/page.tsx`及路由组件 | `B/notifications/{schemas,services}.py`、API注册/依赖、报告详情链接 | Client |
| 045 | `B/content/exports.py`、`B/api/routers/content_exports.py` | `B/content/{models,schemas}.py`、DDL、`B/core/config.py`、`B/worker/{app,execution}.py`、内容列表 | Client |
| 046 | 无 | Design007与Plan索引的Reddit准入记录 | 无 |
| 047 | 无 | Design007与Plan索引的X准入记录；既有`B/sources/adapters/{x_api,x_user_lookup}.py`只读审计 | 无 |
| 048 | 无 | Design007与Plan索引的小红书准入记录 | 无 |
| 049 | 无 | Design007与Plan索引的抖音准入记录 | 无 |
| 050 | 无 | Design007与Plan索引的公众号逐路由准入记录 | 无 |
| 051 | 无 | 共享Acceptance的恢复演练证据（产生后）；既有backups代码仅故障时修 | 无 |
| 052 | `B/knowledge/retrieval.py` | `B/knowledge/{models,schemas}.py`、DDL、`B/db/metadata.py`、`B/core/config.py`、`B/worker/{scheduler,app,execution}.py` | Client（新增API时） |
| 053 | 无 | Design007、PRD007与本索引的微博登录研究记录 | 无 |
| 054 | 无 | Design007、PRD007与本索引的知乎登录研究记录 | 无 |
| 055 | 无 | `B/evidence/{services,models,schemas}.py`、`B/evidence/adapters/minio.py`、`B/cli/commands.py`；必要时DDL | 无 |
| 056 | 无 | `B/api/{exception_handlers,middleware,docs,router,dependencies}.py`、`F/{request,proxy}.ts`、四份CI工作流 | Client |
| 057 | 无；获准后Browser处理器路径先补Design | `B/sources/adapters/{web_targets,http_source,firecrawl,browser_runtime}.py`、`B/connections/adapters/local_secrets.py`、`B/worker/{app,execution}.py`、`B/cli/commands.py`、浏览器/Compose文件（当前冻结） | Client（契约变化时） |

## 验证命令与证据

卡内标为“新增”的测试文件是计划产物。以下命令供实施阶段运行，本次文档修订未运行应用门禁。

- **B（后端）**：`backend/` 下 `uv run ruff check .`、`uv run ruff format --check .`、`uv run mypy`及卡内 `uv run pytest ...`；交付前按仓库要求运行受影响全套pytest。DDL/事务/消息使用隔离真实PostgreSQL/Redis/Kafka，MinIO/browser相关卡验证相应真实依赖。
- **C（契约）**：同提交FastAPI运行OpenAPI，核对类型化响应、权限、422脱敏；`frontend/` 下 `pnpm openapi:generate`，审生成差异并同批交付，然后 `pnpm openapi:check` 验证无漂移，不手改生成物。
- **F（前端）**：`frontend/` 下 `pnpm lint`、`pnpm test`、`pnpm typecheck`、`pnpm format:check`、`pnpm build`；真实浏览器验证桌面/窄屏路径及加载/空/错误/权限/部分状态。

每条SPEC的CHK须记录输入/预期/实际、代码/Schema/配置版本、测试或命令、环境/库、Job/内容/窗口ID、真实证据与限制。通用pytest通过不是某个功能满足的唯一说明。验收目标路径（尚未产出时不是有效链接）：`docs/acceptance/001-共享运行门槛验收.md`、`002-信息获取主链路验收.md`、`003-本人账号B站试点验收.md`、`004-事件与热度验收.md`、`005-报告与知识库验收.md`、`006-推送验收.md`、`007-扩展能力验收.md`；分别记录技术、真实和产品结论。

## 历史与职责迁移

旧七份里程碑Plan原文见Git历史；当前001—030以ba15f126为修订基线，FR/NFR/AC不重编号。002原“来源节奏与预算安全重试”拆至002/031/032；007原“HN重放与评论父链”拆至007/038；030原“报告与原始数据导出”拆至030/045，三份文件按新职责更名。004页面迁034、008历史页面迁039、事件页面迁042、报告调度迁043、通知配置迁044、009运行库保留迁051、023检索迁052。旧SPEC/CHK见前一Git版本，责任以本表和各卡具体合同为准。

后续Issue取058起。新046/047分别为Reddit/X，历史HTTP/browser门槛必须带“历史”说明。所有本轮改动属于文档设计修订，未执行代码实现、平台请求、连续运行或产品验收。
