---
layer: Design
scope: shared
doc_no: "000"
title: Design 索引
status: accepted
version: v2.8
owner: HotKey Team
canonical_path: docs/design/README.md
---

# Design 索引

Design 记录长期有效的产品与技术决策。状态只使用 `proposed`、`accepted`、`superseded`。001–032、035、037、038 均已评审、实现并随对应 Acceptance 归档；033 是可延后的 X 增强，错误的 X-only 034 已由 035 替代；035 的复杂首版边界又由 037 收敛为参考项目真实可用的热点监控闭环。036 是 037 之后的评论会话候选，尚未授权实现；038 将 037 的通知体验明确收敛为邮件与 WebSocket 两条通道。

具体编号和当前实现差距见 [文档地图](../README.md)。同编号必须链接到对应 [PRD](../prd/README.md) 与 [Plan](../plans/README.md)。

## 交付项

| 编号 | Design | PRD | Plan | 状态 |
|---:|---|---|---|---|
| 001 | [MVP产品边界与模块化单体](archive/001-MVP产品边界与模块化单体设计.md) | [PRD](../prd/archive/001-MVP产品边界与模块化单体.md) | [Plan](../plans/archive/001-MVP产品边界与模块化单体计划.md) | accepted |
| 002 | [Vercel风格与shadcn-ui设计系统](archive/002-Vercel风格与shadcn-ui设计系统设计.md) | [PRD](../prd/archive/002-Vercel风格与shadcn-ui设计系统.md) | [Plan](../plans/archive/002-Vercel风格与shadcn-ui设计系统计划.md) | accepted |
| 003 | [账户认证与会话安全](archive/003-账户认证与会话安全设计.md) | [PRD](../prd/archive/003-账户认证与会话安全.md) | [Plan](../plans/archive/003-账户认证与会话安全计划.md) | accepted |
| 004 | [用户角色与权限管理](archive/004-用户角色与权限管理设计.md) | [PRD](../prd/archive/004-用户角色与权限管理.md) | [Plan](../plans/archive/004-用户角色与权限管理计划.md) | accepted |
| 005 | [监控主题CRUD与生命周期](archive/005-监控主题CRUD与生命周期设计.md) | [PRD](../prd/archive/005-监控主题CRUD与生命周期.md) | [Plan](../plans/archive/005-监控主题CRUD与生命周期计划.md) | accepted |
| 006 | [关键词规则与多语言查询扩展](archive/006-关键词规则与多语言查询扩展设计.md) | [PRD](../prd/archive/006-关键词规则与多语言查询扩展.md) | [Plan](../plans/archive/006-关键词规则与多语言查询扩展计划.md) | accepted |
| 007 | [来源连接控制面与合规健康](archive/007-来源连接控制面与合规健康设计.md) | [PRD](../prd/archive/007-来源连接控制面与合规健康.md) | [Plan](../plans/archive/007-来源连接控制面与合规健康计划.md) | accepted |
| 008 | [RSS-Atom来源连接器](archive/008-RSS-Atom来源连接器设计.md) | [PRD](../prd/archive/008-RSS-Atom来源连接器.md) | [Plan](../plans/archive/008-RSS-Atom来源连接器计划.md) | accepted |
| 009 | [Hacker-News官方来源连接器](archive/009-Hacker-News官方来源连接器设计.md) | [PRD](../prd/archive/009-Hacker-News官方来源连接器.md) | [Plan](../plans/archive/009-Hacker-News官方来源连接器计划.md) | accepted |
| 010 | [X官方搜索连接器](archive/010-X官方搜索连接器设计.md) | [PRD](../prd/archive/010-X官方搜索连接器.md) | [Plan](../plans/archive/010-X官方搜索连接器计划.md) | accepted |
| 011 | [Bing-Grounding来源适配](archive/011-Bing-Grounding来源适配设计.md) | [PRD](../prd/archive/011-Bing-Grounding来源适配.md) | [Plan](../plans/archive/011-Bing-Grounding来源适配计划.md) | accepted |
| 012 | [搜狗授权来源适配](archive/012-搜狗授权来源适配设计.md) | [PRD](../prd/archive/012-搜狗授权来源适配.md) | [Plan](../plans/archive/012-搜狗授权来源适配计划.md) | accepted |
| 013 | [Bilibili开放平台与账号监控](archive/013-Bilibili开放平台与账号监控设计.md) | [PRD](../prd/archive/013-Bilibili开放平台与账号监控.md) | [Plan](../plans/archive/013-Bilibili开放平台与账号监控计划.md) | accepted |
| 014 | [微博开放平台来源适配](archive/014-微博开放平台来源适配设计.md) | [PRD](../prd/archive/014-微博开放平台来源适配.md) | [Plan](../plans/archive/014-微博开放平台来源适配计划.md) | accepted |
| 015 | [Google可授权搜索迁移](archive/015-Google可授权搜索迁移设计.md) | [PRD](../prd/archive/015-Google可授权搜索迁移.md) | [Plan](../plans/archive/015-Google可授权搜索迁移计划.md) | accepted |
| 016 | [DuckDuckGo-Instant-Answer边界](archive/016-DuckDuckGo-Instant-Answer边界设计.md) | [PRD](../prd/archive/016-DuckDuckGo-Instant-Answer边界.md) | [Plan](../plans/archive/016-DuckDuckGo-Instant-Answer边界计划.md) | accepted |
| 017 | [定时监听手动搜索与失败重试](archive/017-定时监听手动搜索与失败重试设计.md) | [PRD](../prd/archive/017-定时监听手动搜索与失败重试.md) | [Plan](../plans/archive/017-定时监听手动搜索与失败重试计划.md) | accepted |
| 018 | [内容标准化去重时效与原始证据](archive/018-内容标准化去重时效与原始证据设计.md) | [PRD](../prd/archive/018-内容标准化去重时效与原始证据.md) | [Plan](../plans/archive/018-内容标准化去重时效与原始证据计划.md) | accepted |
| 019 | [AI真伪相关性重要性与摘要](archive/019-AI真伪相关性重要性与摘要设计.md) | [PRD](../prd/archive/019-AI真伪相关性重要性与摘要.md) | [Plan](../plans/archive/019-AI真伪相关性重要性与摘要计划.md) | accepted |
| 020 | [事件聚类生命周期热度与趋势](archive/020-事件聚类生命周期热度与趋势设计.md) | [PRD](../prd/archive/020-事件聚类生命周期热度与趋势.md) | [Plan](../plans/archive/020-事件聚类生命周期热度与趋势计划.md) | accepted |
| 021 | [内容与事件检索筛选排序](archive/021-内容与事件检索筛选排序设计.md) | [PRD](../prd/archive/021-内容与事件检索筛选排序.md) | [Plan](../plans/archive/021-内容与事件检索筛选排序计划.md) | accepted |
| 022 | [实时事件流与站内通知](archive/022-实时事件流与站内通知设计.md) | [PRD](../prd/archive/022-实时事件流与站内通知.md) | [Plan](../plans/archive/022-实时事件流与站内通知计划.md) | accepted |
| 023 | [低噪声告警与邮件交付](archive/023-低噪声告警与邮件交付设计.md) | [PRD](../prd/archive/023-低噪声告警与邮件交付.md) | [Plan](../plans/archive/023-低噪声告警与邮件交付计划.md) | accepted |
| 024 | [日报周报私有Feed与知识归档](archive/024-日报周报私有Feed与知识归档设计.md) | [PRD](../prd/archive/024-日报周报私有Feed与知识归档.md) | [Plan](../plans/archive/024-日报周报私有Feed与知识归档计划.md) | accepted |
| 025 | [配额限流保留与审计治理](archive/025-配额限流保留与审计治理设计.md) | [PRD](../prd/archive/025-配额限流保留与审计治理.md) | [Plan](../plans/archive/025-配额限流保留与审计治理计划.md) | accepted |
| 026 | [Web工作台全页面交互](archive/026-Web工作台全页面交互设计.md) | [PRD](../prd/archive/026-Web工作台全页面交互.md) | [Plan](../plans/archive/026-Web工作台全页面交互计划.md) | accepted |
| 027 | [Agent-Skill与外部API](archive/027-Agent-Skill与外部API设计.md) | [PRD](../prd/archive/027-Agent-Skill与外部API.md) | [Plan](../plans/archive/027-Agent-Skill与外部API计划.md) | accepted |
| 028 | [可观测性部署与质量门禁](archive/028-可观测性部署与质量门禁设计.md) | [PRD](../prd/archive/028-可观测性部署与质量门禁.md) | [Plan](../plans/archive/028-可观测性部署与质量门禁计划.md) | accepted |
| 029 | [来源凭据与个性化配置](archive/029-来源凭据与个性化配置设计.md) | [PRD](../prd/archive/029-来源凭据与个性化配置.md) | [Plan](../plans/archive/029-来源凭据与个性化配置计划.md) | accepted |
| 030 | [通知与订阅页面拆分](archive/030-通知与订阅页面拆分设计.md) | [PRD](../prd/archive/030-通知与订阅页面拆分.md) | [Plan](../plans/archive/030-通知与订阅页面拆分计划.md) | accepted |
| 031 | [Hacker-News热门榜单与持续观测](archive/031-Hacker-News热门榜单与持续观测设计.md) | [PRD](../prd/archive/031-Hacker-News热门榜单与持续观测.md) | [Plan](../plans/archive/031-Hacker-News热门榜单与持续观测计划.md) | accepted |
| 032 | [热点事件语义监控与出处正文](archive/032-热点事件语义监控与出处正文设计.md) | [PRD](../prd/archive/032-热点事件语义监控与出处正文.md) | [Plan](../plans/archive/032-热点事件语义监控与出处正文计划.md) | accepted |
| 033 | [X热点发现与持续热度观测](033-X热点发现与持续热度观测设计.md) | [PRD](../prd/033-X热点发现与持续热度观测.md) | [Plan](../plans/033-X热点发现与持续热度观测计划.md) | accepted |
| 034 | [X热点核心链路收敛与遗留系统清理](034-X热点核心链路收敛与遗留系统清理设计.md) | [PRD](../prd/034-X热点核心链路收敛与遗留系统清理.md) | [Plan](../plans/034-X热点核心链路收敛与遗留系统清理计划.md) | superseded |
| 035 | [多源AI热点监控首版收敛](archive/035-多源AI热点监控首版收敛设计.md) | [PRD](../prd/archive/035-多源AI热点监控首版收敛.md) | [Plan](../plans/archive/035-多源AI热点监控首版收敛计划.md) | superseded |
| 036 | [评论会话采集与采集运行时边界](036-评论会话采集与采集运行时边界设计.md) | [PRD](../prd/036-评论会话采集与采集运行时边界.md) | [Plan](../plans/036-评论会话采集与采集运行时边界计划.md) | proposed |
| 037 | [实时热点监控MVP功能基线](archive/037-实时热点监控MVP功能基线设计.md) | [PRD](../prd/archive/037-实时热点监控MVP功能基线.md) | [Plan](../plans/archive/037-实时热点监控MVP功能基线计划.md) | accepted |
| 038 | [邮件与WebSocket双通道通知收敛](archive/038-邮件与WebSocket双通道通知收敛设计.md) | [PRD](../prd/archive/038-邮件与WebSocket双通道通知收敛.md) | [Plan](../plans/archive/038-邮件与WebSocket双通道通知收敛计划.md) | accepted |
