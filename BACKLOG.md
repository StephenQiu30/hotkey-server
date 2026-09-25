# HotKey BACKLOG

更新日期：2026-09-25。本文件只记录任务卡状态；需求见 [PRD 001](docs/prd/001-热点舆情监控平台需求.md)，设计见 [Design 001](docs/design/001-热点舆情监控平台总体设计.md)，任务卡内容见 [Plan 001](docs/plans/001-热点舆情监控平台总计划.md)。

**当前阶段：P1-A Hacker News 竖切。** 执行方式：任务卡交给 Codex 实现，Claude 审查、验证、提交。

## P1

| 卡 | 内容 | 状态 |
|---|---|---|
| A1 | 来源预设与连接配置 | planned |
| A2 | 适配器安全修正并提交（HN/RSS/SearXNG 已写未提交） | in_progress |
| A3 | 注册 `keyword.search`；无发布时间按发现时间；按任务类型截止 | planned |
| A4 | `source.comments` 处理器 | planned |
| A5 | 竖切验收（AC-001-100） | planned |
| A6 | Google News、SearXNG、行业 RSS 预设 | planned |
| B1 | 主题设置与调度表 | planned |
| B2 | 调度进程与采集扫描 | planned |
| B3 | 评论扫描 | planned |
| C1 | AI 领域：隔离、调用记录、门禁（Codex 客户端已写未提交） | in_progress |
| C2 | 批量标注 | planned |
| D1 | 日报（确定性 + 模板版） | planned |
| D2 | 模型润色 | planned |
| D3 | Obsidian 日报导出 | planned |
| D4 | Web 报告页 | planned |
| E1 | 飞书推送 | planned |
| E2 | 邮件推送 | planned |
| E3 | 待确认投递处理 | planned |
| F | 连续 3 天验收 | planned |

建议顺序：A1/A2/C1 并行 → A3 → A4 → A5 → A6/B1 → B2 → B3/C2 → D1 → D2/D3/D4/E1 → E2/E3 → F。

## 已完成

P0 文档整理与统一规划；评论线程入库（`c10f2a37`）；契约字段与移除 twscrape（`12fdc334`）；本地 RSSHub/SearXNG（`795ffa8e`）。

## 未决

OPEN-001-104 X 官方 API（P4）；OPEN-001-107 知乎接入方式（P2）。

## 冻结与取消

冻结：032 S03+、033 公平派发/熔断、028 S02+、029 S03、039 S02+、040、042 B0、010 历史回补、Flutter App。取消：pgvector/向量检索、团队协作、044、045。企业微信延后。
