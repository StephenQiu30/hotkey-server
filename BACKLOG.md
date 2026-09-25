# HotKey BACKLOG

更新日期：2026-09-25。本文件只记录阶段与任务状态；需求见 [PRD 001](docs/prd/001-热点舆情监控平台需求.md)，设计见 [Design 001](docs/design/001-热点舆情监控平台总体设计.md)，任务细节见 [Plan 001](docs/plans/001-热点舆情监控平台总计划.md)。v1.x 的 BACKLOG（34 项 P0、0/34 验收）见 Git 提交 `3a72611a`，已作废。

**当前阶段：P0 整理完成，等待用户确认 OPEN 项后进入 P1。**

## 阶段

| 阶段 | 周 | 目标 | 验收 | 状态 |
|---|---|---|---|---|
| P0 整理 | 0 | 代码评审、需求与文档重整 | OPEN-001-101/102/106 有结论 | in_progress |
| P1 主链路 | 1—2 | A 档来源 → 帖子+评论 → 分析 → 日报 → 推送 | AC-001-101—104 | planned |
| P2 国内来源与事件 | 3—4 | B 站、微博、知乎；热榜；事件归并与热度 | AC-001-105 | planned |
| P3 周报与知识库 | 5—6 | 周报、检索、问答、导出 | AC-001-106/107/108 | planned |
| P4 扩展 | 7+ | 突发告警、账号追踪、X/小红书/抖音/公众号 | AC-001-109 | planned |

## 待用户决定

| ID | 问题 | 阻塞 |
|---|---|---|
| OPEN-001-101 | 是否同意转向与冻结 | 全部 P1 |
| OPEN-001-102 | 是否允许付费调用外部大模型及月度上限 | P1-T06 起 |
| OPEN-001-106 | 默认推送渠道（建议飞书 + 邮件） | P1-T09 |
| OPEN-001-103 | 小红书/抖音/公众号：自建还是第三方数据服务 | P4 |
| OPEN-001-104 | 是否申请 X 官方 API | P4 |
| OPEN-001-105 | 个人工具还是团队共享 | P1-T09 推送对象 |

## P0

| 任务 | 状态 |
|---|---|
| P0-T01 代码评审与可行性评估 | completed |
| P0-T02 文档整理（PRD/Design/Plan、docs、BACKLOG、HANDOVER、PROJECT、AGENTS） | completed，待提交 |
| P0-T03 OPEN 项确认 | 待用户 |

## P1（第 1—2 周）

| 任务 | 内容 | 状态 |
|---|---|---|
| P1-T01 | 来源契约字段 + HOTLIST；清理 twscrape | planned |
| P1-T02 | 评论线程表与评论入库 | planned |
| P1-T03 | 调度表与 Worker 调度循环；主题频率设置 | planned |
| P1-T04 | RSS / Hacker News / Reddit 适配器 | planned |
| P1-T05 | 注册 source.search / source.comments 处理器 | planned |
| P1-T06 | ai 领域：模型适配器、调用记录、成本上限 | blocked（OPEN-001-102） |
| P1-T07 | analysis：相关性、摘要、情感 | planned |
| P1-T08 | reports：日报生成 | planned |
| P1-T09 | notifications：飞书/企微/邮件推送 | blocked（OPEN-001-106） |
| P1-T10 | Web：报告页、推送设置页 | planned |
| P1-T11 | 3 天真实运行验收 | planned |

P2—P4 任务见 Plan 001 第 5—7 节，进入该阶段时再展开到本表。

## 冻结项

032 备份 S03+、033 公平派发/熔断、028 S02+、029 S03、039 S02+、040 可访问性、042 B0 容量、010 历史回补、Flutter App。代码与测试保留，不作为新功能的前置门禁。移出范围：044 模型平台信息采集、045 模型联网检索。
