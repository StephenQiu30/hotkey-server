# HotKey BACKLOG

更新日期：2026-09-25。本文件是**唯一的进度看板**：阶段、验收、任务卡、待办、风险都在这里更新。任务卡的详细内容（文件、要点、验收）见 [Plan 001](docs/plans/001-热点舆情监控平台总计划.md)，需求见 [PRD 001](docs/prd/001-热点舆情监控平台需求.md)，设计见 [Design 001](docs/design/001-热点舆情监控平台总体设计.md)。

状态：`todo` 未开始 · `doing` 进行中 · `review` 待审查/验证 · `done` 完成 · `blocked` 受阻。执行方：Codex 实现 → Claude 审查、验证、提交；标"你"的需要你本人操作。

## 1. 总览

| 阶段 | 目标 | 卡片完成 | 状态 |
|---|---|---|---|
| P0 | 评审、需求、设计、计划统一 | 3/3 | done |
| P1 | HN 竖切 → 调度 → 分析 → 日报 + Obsidian → 飞书/邮件 → 3 天验收 | 0/19 | doing |
| P2 | MediaCrawler（B 站、微博）、热榜、事件、事件笔记 | 0/6 | todo |
| P3 | 周报、主题笔记、问答、导出 | 0/4 | todo |
| P4 | 告警、账号追踪、Reddit/小红书/抖音/公众号/X | 0/7 | todo |

### 验收状态

| AC | 内容 | 相关卡 | 状态 |
|---|---|---|---|
| AC-001-100 | HN 竖切：搜索、评论、楼中楼、重放幂等 | A1—A5 | todo |
| AC-001-101 | 持续采集 72 小时 | A6、B1—B3、F | todo |
| AC-001-102 | 评论完整性 | A4、B3、P2-2 | todo |
| AC-001-103 | 分析准确率（相关 ≥90%、情感 ≥80%） | C2、F | todo |
| AC-001-104 | 连续 3 天 09:15 前收到日报并可追溯 | D1—D4、E1、E2、F | todo |
| AC-001-105 | 事件归并 | P2-5、P2-6 | todo |
| AC-001-106 | 周报 | P3-1 | todo |
| AC-001-107 | 问答 | P3-3 | todo |
| AC-001-108 | 故障降级与限流不重复推送 | D2、E1、F | todo |
| AC-001-109 | Obsidian 安全写入 | D3 | todo |
| AC-001-110 | P4 各项 | P4-* | todo |

## 2. 当前迭代

目标：完成 AC-001-100（Hacker News 竖切）。

| 顺序 | 卡 | 内容 | 执行方 | 状态 | 备注 |
|---|---|---|---|---|---|
| 1 | A2 | 适配器加主机白名单与重定向校验，提交已写的 HN/RSS/SearXNG 适配器 | Codex | doing | 代码已在工作区，缺白名单 |
| 1 | C1 | Codex 最小环境变量、`ai_calls`、预算门禁、验证能否关闭 shell 工具 | Codex | doing | 客户端已在工作区；`test_ai_calls.py` 为失败测试 |
| 1 | A1 | 来源预设 CLI + 连接版本 `config` | Codex | todo | 可与 A2、C1 并行 |
| 2 | A3 | 注册 `keyword.search`；无发布时间按发现时间；按任务类型截止 | Codex | todo | 依赖 A1、A2 |
| 3 | A4 | `source.comments` 处理器 | Codex | todo | 依赖 A3 |
| 4 | A5 | 竖切验收记录 | Claude | todo | 依赖 A4 |

## 3. P1 全部任务卡

| 卡 | 内容 | 依赖 | 状态 | 提交 |
|---|---|---|---|---|
| A1 | 来源预设与连接配置 | — | todo | |
| A2 | 适配器安全修正并提交 | — | doing | |
| A3 | `keyword.search` 处理器、时间口径、任务截止 | A1、A2 | todo | |
| A4 | `source.comments` 处理器 | A3 | todo | |
| A5 | HN 竖切验收（AC-001-100） | A4 | todo | |
| A6 | Google News、SearXNG、行业 RSS 预设 | A3 | todo | |
| B1 | 主题设置（来源、频率、报告时间、推送目标）与调度表 | A1 | todo | |
| B2 | 调度进程 `python -m worker.scheduler` 与采集扫描 | B1、A3 | todo | |
| B3 | 评论扫描 | B2、A4 | todo | |
| C1 | AI 领域：隔离、调用记录、门禁 | — | doing | |
| C2 | `analysis.annotate` 批量标注与分析扫描 | C1、B2 | todo | |
| D1 | 日报：冻结输入、确定性取数、模板版 | C2 | todo | |
| D2 | 日报模型润色与引用校验 | D1、C1 | todo | |
| D3 | Obsidian 日报导出 | D1 | todo | |
| D4 | Web 报告页 | D1 | todo | |
| E1 | 飞书推送与投递状态机 | D1、你-1 | todo | |
| E2 | 邮件推送 | E1、你-2 | todo | |
| E3 | `unknown` 投递的人工确认入口 | E1 | todo | |
| F | 连续 3 天真实运行验收 | 全部 | todo | |

## 4. P2—P4 任务卡

| 卡 | 内容 | 依赖 | 状态 |
|---|---|---|---|
| P2-1 | MediaCrawler 部署（固定提交、容器、登录态存服务端） | P1、你-3 | todo |
| P2-2 | `mediacrawler.py`；B 站、微博搜索、评论、楼中楼 | P2-1 | todo |
| P2-3 | `HOTLIST` 能力 + RSSHub 热榜预设 | P1 | todo |
| P2-4 | 知乎接入方式实测（关闭 OPEN-001-107） | P2-1 | todo |
| P2-5 | 事件归并（`pg_trgm` 候选 + Codex 确认）与热度 | C2 | todo |
| P2-6 | 日报事件区块；Obsidian 事件与帖子笔记 | P2-5、D3 | todo |
| P3-1 | 周报与周报笔记 | P1 | todo |
| P3-2 | Obsidian 主题笔记 | D3 | todo |
| P3-3 | 问答（`pg_trgm` + Codex，写入 `问答/`） | P1 | todo |
| P3-4 | 导出 Markdown/PDF、CSV/JSON | D1 | todo |
| P4-1 | 突发告警 | P2-5 | todo |
| P4-2 | 指定账号追踪 | P1 | todo |
| P4-3 | Reddit 官方 OAuth | 你-4 | todo |
| P4-4 | 小红书（MediaCrawler） | P2-1、你-3 | todo |
| P4-5 | 抖音（MediaCrawler） | P2-1、你-3 | todo |
| P4-6 | 微信公众号（RSSHub） | P1 | todo |
| P4-7 | X 官方 API | OPEN-001-104 | blocked |

## 5. 待办（非代码）

| 编号 | 事项 | 负责 | 需要时间点 | 状态 |
|---|---|---|---|---|
| 你-1 | 创建飞书群自定义机器人，提供 Webhook（和签名密钥）写入本机环境变量 | 你 | E1 前 | todo |
| 你-2 | 准备发信邮箱的 SMTP 主机、端口、账号、授权码与收件人 | 你 | E2 前 | todo |
| 你-3 | 准备 B 站、微博（后续小红书、抖音）采集用小号，用于 MediaCrawler 扫码登录 | 你 | P2-1 前 | todo |
| 你-4 | 注册 Reddit 开发者应用（OAuth） | 你 | P4-3 前 | todo |
| 你-5 | 决定是否申请 X 官方 API 与月度上限 | 你 | P4 | todo |
| 运-1 | 新开会话验证 `/codex:rescue` 子代理可启动（DeepSeek 配置已移除） | Claude | 下次会话 | todo |
| 运-2 | 本地采集栈常驻：RSSHub、SearXNG、Firecrawl 开机自启或启动说明 | Claude | F 前 | todo |
| 运-3 | 宿主机 Worker 与调度进程的常驻方式（launchd 或终端） | Claude | F 前 | todo |

## 6. 未决与风险

| 编号 | 内容 | 处理 |
|---|---|---|
| OPEN-001-104 | X 官方 API | P4 决定 |
| OPEN-001-107 | 知乎走 MediaCrawler 还是 RSSHub | P2-4 实测 |
| RSK-001-207 | 提示注入诱导 Codex 读本机文件 | C1：最小环境变量、空目录、尝试关闭 shell 工具 |
| RSK-001-206 | Codex 账号限流 | 批量标注、限流延后、日报可降级 |
| RSK-001-201 | 国内平台反爬与登录态失效 | P2 适配器隔离、每日冒烟 |

## 7. 已完成

| 日期 | 内容 | 提交 |
|---|---|---|
| 2026-09-25 | 代码评审与可行性评估 | — |
| 2026-09-25 | 文档整理：PRD/Design/Plan 各一份，移除 96 份旧文档 | `e25d4440` |
| 2026-09-25 | 采集栈、模型、推送决策 | `df500ce7` |
| 2026-09-25 | 评论线程表、作者昵称入库 | `c10f2a37` |
| 2026-09-25 | 来源契约字段、移除 twscrape | `12fdc334` |
| 2026-09-25 | 本地 RSSHub、SearXNG；Firecrawl 接 SearXNG | `795ffa8e` |
| 2026-09-25 | 统一需求、设计与任务卡规划 | `71a90d88` |
| 2026-09-25 | Codex 默认模型改为 `gpt-5.6-sol` + `high`；移除 Claude Code 的 DeepSeek 配置（本机配置，不入库） | — |

## 8. 冻结与取消

- 冻结（代码保留，不作门禁）：032 S03+、033 公平派发/熔断、028 S02+、029 S03、039 S02+、040、042 B0、010 历史回补、Flutter App。
- 取消：pgvector/向量检索、团队协作、044、045。延后：企业微信。

## 9. 维护规则

- 卡片状态变化、提交完成、待办完成时即时更新本文件；"卡片完成"计数与验收状态同步更新。
- 新增工作先在 Plan 001 写成任务卡，再登记到本文件；不在这里写实现细节。
- 本文件 ≤ 10 KB；完成超过一个阶段的记录可压缩为一行。
