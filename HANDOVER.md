# HotKey Server 交接

更新日期：2026-09-26。本文件只记录当前实现快照（≤5 KB）；需求见 [PRD 001 v5.0](docs/prd/001-热点舆情监控平台需求.md)，Epic 设计见 [Design 001 v4.0](docs/design/001-热点舆情监控平台总体设计.md) 及对应 Design，任务与证据见 [逐 Issue Plan 索引](docs/plan/README.md) 和 [BACKLOG](BACKLOG.md)。本次仅重构计划文档，代码实现和产品验收状态未改变。

## 当前边界

- 信息获取是当前核心：四关键词来源 HN Algolia、Google News 搜索 RSS、本机 SearXNG `duckduckgo news`、本机 RSSHub `/36kr/newsflashes`；六个 RSSHub 公开热榜；HN 评论父链、来源 × 能力 × 时间窗覆盖和本机 Codex 相关性。M1 须同窗连续 72 小时真实验收。日报/周报、Obsidian 与推送各自后验，飞书暂缓。
- 单 owner 登录、主题、连接、内容、Outbox/Kafka/Worker、预算、备份、统一错误契约及 Web 工作台已有代码。宿主机单 Worker 已注册 `webpage.collect`、`keyword.search`、`source.comments`、`source.hotlist`、`analysis.annotate`、`report.daily`、`notification.send`、`knowledge.export`；独立调度进程已存在。事件归并、SMTP 与统一覆盖查询仍待实现，旧“Worker 只有网页处理器”的记录已过期。
- 本人账号来源本轮仅 B 站试点。MediaCrawler 固定补丁及独立 CDP 资料记录在 `~/Desktop/Docker/mediacrawler-start-local/`，HotKey 从宿主机启动子进程；评论只读同轮缓存的一级评论每帖 ≤20 条。修复前真实尝试失败，修复后尚未真实采集，开关关闭。未知非零退出误归认证失效、组件三类版本证据、停用和人工恢复仍需修复/验收；不把离线回放称为接入成功。

## 证据快照

2026-09-26 的可重建开发库 `hotkey_p1` 约 4 小时运行：四来源分别入库 Google News 365、SearXNG 94、HN 61、36Kr 8；六榜 59 快照，最近两小时 23 成功、1 失败；Codex 738 标注中相关字段为空 3 条。HN 45 线程来自较早且已重建的库，不可与当前库合并。四来源和六榜尚无连续 72 小时产品验收；开发库结果仅证明所述真实运行范围。B 站新建库离线回放与旧失败运行均不满足 AC-113/115/122。

## 本地运行与数据门槛

| 组件 | 入口/边界 |
|---|---|
| HotKey | 根 `docker-compose.yml`；API `127.0.0.1:8867`，Web `127.0.0.1:3000`；在 `backend/app/` 下运行 `uvicorn main:create_app --factory`、`python -m worker`、`python -m worker.scheduler` |
| RSSHub、SearXNG | 分别在 `~/Desktop/Docker/rsshub-start-local/`、`~/Desktop/Docker/searxng-start-local/` 独立 Compose；端口 `1200`、`8888` |
| Firecrawl | 独立本地编排；公开网页业务结果单独验收 |
| MediaCrawler | `~/Desktop/Docker/mediacrawler-start-local/` 保留补丁、资料和运行说明；HotKey B 站入口是宿主机子进程、`127.0.0.1` 独立 CDP 端口 |
| Codex | 本机 app-server，分析模型由 `HOTKEY_AI_MODEL` 配置，不发付费模型请求 |
| Obsidian | 现有 `~/Desktop/Markdown/Obsidian`，只写 `HotKey/`；真实写入另验 |

可丢弃开发库可按完整 `schema.sql` 新建；M1 连续运行库或正式库需保留数据时，先备份并实际验证恢复，再新建库、原子应用完整 Schema、导入并核对核心表、身份/外键、连接版本、Job/Outbox/offset、覆盖和预算，旧库保留回退。不得就地执行完整 Schema 或把开发库重建后的测试算作旧运行证据。

## 下一步

按 BACKLOG 的建议顺序先处理 M1 覆盖/空榜/失败桶/标注异常/时效与 HN 重放，并按 Design 002 第 3.1 节/Design 001 第 6.2 节 重新评估 stash `paused: partial P2-2b + A7b implementation (2026-09-26)`；再进行 M1 72 小时运行。M2 版本与风控门槛可并行评估，本人账号登录和产品决策由用户承担。所有代码提交、推送须先有用户明确授权。
