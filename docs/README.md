# HotKey 文档索引

当前为需求/设计复核与工程底座补齐阶段。已有部分工程实现，业务闭环尚未验收；43 项逐项 PRD 中 003、004、007 已完成评审，其余仍为草案。2026-09-22 已建立 proposed 总体 Design，完成 046 公共技术前置、003/007/009 S00—S03、004/031/036/037 S00—S02、027 S00、028/029/032/034/035/038/039 S00/S01 与 002 S01 受控技术切片；首批来源和产品范围仍待评审。

总体实施顺序与进度见 [BACKLOG](../BACKLOG.md)，具体任务和阶段 checklist 见 [执行计划索引](plans/README.md)。

项目技术选型见根 [PROJECT.md](../PROJECT.md)，当前交接见 [HANDOVER.md](../HANDOVER.md)。2026-09-18 已按用户决定将当前设计/计划中的消息依赖调整为 Kafka，并补入 Redis；Web 归属 `frontend/`，独立客户端为 Flutter `hotkey-app`。该调整不代表消息执行设计或产品验收完成。

## 文档台账

统一异常、响应和状态契约由 [046 Design](design/046-全局异常与响应契约设计.md) 定义；[046 Plan](plans/046-全局异常与响应契约前置计划.md) S03 已通过 [Acceptance](acceptance/046-全局异常与响应契约验收.md)，其他 Plan 可在各自领域前置满足后进入实现。

| 编号 | 主题 | Research | PRD | Design | Plan / Acceptance |
|---|---|---|---|---|---|
| 001 | 热点事件监控平台 | [鱼皮项目功能对照调研](research/001-热点事件监控平台调研.md) · draft | [功能与非功能需求](prd/001-热点事件监控平台功能与非功能需求.md) · draft | [总体设计](design/001-热点事件监控平台总体设计.md) · proposed | [总计划](plans/001-热点事件监控平台总计划.md) · in_progress；Acceptance 未建立 |
| 002 | X 免费采集与热点监控专项 | 一手资料包含在专项设计 | 尚未建立独立专项 PRD；受 001 总体需求约束 | [专项设计](design/002-X免费采集与热点监控设计.md) · proposed，S01 范围已冻结 | [专项计划](plans/002-X免费采集与热点监控计划.md) · in_progress；S01 受控通过，0/10 AC；Acceptance 未建立 |
| 043 | 模型服务接入与模型配置 | [OpenRouter / Vercel / 本地模型调研](research/043-模型服务接入与模型配置调研.md) · draft | [模型配置 PRD](prd/043-模型服务接入与模型配置.md) · draft | [模型服务设计](design/043-模型服务接入与模型配置设计.md) · proposed | [执行计划](plans/043-模型服务接入与模型配置计划.md) · planned；Acceptance 未建立 |
| 044 | 模型平台信息采集与变更监控 | [调研](research/044-模型平台信息采集与变更监控调研.md) · draft | [PRD](prd/044-模型平台信息采集与变更监控.md) · draft | [设计](design/044-模型平台信息采集与变更监控设计.md) · proposed | [计划](plans/044-模型平台信息采集与变更监控计划.md) · planned；Acceptance 未建立 |
| 045 | 模型联网检索与线索采集 | [调研](research/045-模型联网检索与线索采集调研.md) · draft | [PRD](prd/045-模型联网检索与线索采集.md) · draft | [设计](design/045-模型联网检索与线索采集设计.md) · proposed | [计划](plans/045-模型联网检索与线索采集计划.md) · planned；Acceptance 未建立 |
| 046 | 全局异常与响应契约前置 | 当前代码与隔离探针核对见 Design | 承接既有质量需求，不新增产品 PRD | [统一设计](design/046-全局异常与响应契约设计.md) · accepted | [前置计划](plans/046-全局异常与响应契约前置计划.md) · completed；[Acceptance](acceptance/046-全局异常与响应契约验收.md) · passed |

阅读顺序：先看 001 调研与需求，再看 [逐项 PRD 索引](prd/README.md)；002 提供 X 来源专项可行性，046 定义实施共同前置。当前不以候选采集器支持范围代替产品需求，不把专项设计当成平台总体设计。

### 逐项需求交付台账

003—042 共 40 个编号已登记为独立需求交付项，当前各有 PRD 与执行计划；003/004/007/009/027/028/029/031/032/034/035/036/037/038/039/042 为 in_progress 且已有 accepted Design，其余为 planned；尚未建立对应 Acceptance。来源依据统一引用 001 Research，002 专项仅作为 X 可行性输入。Design 准备、SPEC 和每阶段 checklist 已列入对应 Plan。另新增 043 的 Research / PRD / proposed Design / Plan 完整链，连同 044/045 两类信息采集入口，逐项需求合计 43 项。

| 编号 | PRD 主题 | 来源需求 | 优先级 | PRD 状态 | Plan |
|---|---|---|---|---|---|
| 003 | [监控主题管理](prd/003-监控主题管理.md) | FR-001-001 | P0 | accepted | [设计](design/003-监控主题管理设计.md) · accepted；[计划](plans/003-监控主题管理计划.md) · in_progress |
| 004 | [平台与连接管理](prd/004-平台与连接管理.md) | FR-001-002 | P0 | accepted | [设计](design/004-平台与连接管理设计.md) · accepted；[计划](plans/004-平台与连接管理计划.md) · in_progress（S00—S02 与 S03 版本屏障先行片，0/6 AC） |
| 005 | [关键词主动发现](prd/005-关键词主动发现.md) | FR-001-003 | P0 | draft | [计划](plans/005-关键词主动发现计划.md) · planned |
| 006 | [指定用户作品追踪](prd/006-指定用户作品追踪.md) | FR-001-004 | P0 | draft | [计划](plans/006-指定用户作品追踪计划.md) · planned |
| 007 | [作品资料与上下文](prd/007-作品资料与上下文.md) | FR-001-005 | P0 | accepted | [设计](design/007-作品资料与上下文设计.md) · accepted（S00—S03）；[计划](plans/007-作品资料与上下文计划.md) · in_progress（S00—S02，0/6 AC） |
| 008 | [评论与回复采集](prd/008-评论与回复采集.md) | FR-001-006 | P0 | draft | [计划](plans/008-评论与回复采集计划.md) · planned |
| 009 | [采集任务控制](prd/009-采集任务控制.md) | FR-001-007 | P0 | draft | [设计](design/009-采集任务控制设计.md) · accepted；[计划](plans/009-采集任务控制计划.md) · in_progress |
| 010 | [增量更新与历史回补](prd/010-增量更新与历史回补.md) | FR-001-008 | P0 | draft | [计划](plans/010-增量更新与历史回补计划.md) · planned |
| 011 | [事件识别与归并](prd/011-事件识别与归并.md) | FR-001-009 | P0 | draft | [计划](plans/011-事件识别与归并计划.md) · planned |
| 012 | [事件详情与发展时间线](prd/012-事件详情与发展时间线.md) | FR-001-010 | P0 | draft | [计划](plans/012-事件详情与发展时间线计划.md) · planned |
| 013 | [相关性与热度排序](prd/013-相关性与热度排序.md) | FR-001-011 | P0 | draft | [计划](plans/013-相关性与热度排序计划.md) · planned |
| 014 | [历史内容检索与筛选](prd/014-历史内容检索与筛选.md) | FR-001-012 | P0 | draft | [计划](plans/014-历史内容检索与筛选计划.md) · planned |
| 015 | [讨论分析与趋势](prd/015-讨论分析与趋势.md) | FR-001-013 | P0 | draft | [计划](plans/015-讨论分析与趋势计划.md) · planned |
| 016 | [证据引用与分析修订](prd/016-证据引用与分析修订.md) | FR-001-014 | P0 | draft | [计划](plans/016-证据引用与分析修订计划.md) · planned |
| 017 | [变化提醒与通知记录](prd/017-变化提醒与通知记录.md) | FR-001-015 | P0 | draft | [计划](plans/017-变化提醒与通知记录计划.md) · planned |
| 018 | [基础报告与数据导出](prd/018-基础报告与数据导出.md) | FR-001-016 | P0 | draft | [计划](plans/018-基础报告与数据导出计划.md) · planned |
| 019 | [人工整理与反馈](prd/019-人工整理与反馈.md) | FR-001-017 | P0 | draft | [计划](plans/019-人工整理与反馈计划.md) · planned |
| 020 | [数据与使用设置](prd/020-数据与使用设置.md) | FR-001-018 | P0 | draft | [计划](plans/020-数据与使用设置计划.md) · planned |
| 021 | [热榜与订阅补充发现](prd/021-热榜与订阅补充发现.md) | FR-001-019 | P1 | draft | [计划](plans/021-热榜与订阅补充发现计划.md) · planned |
| 022 | [自动分析增强](prd/022-自动分析增强.md) | FR-001-020 | P1 | draft | [计划](plans/022-自动分析增强计划.md) · planned |
| 023 | [图片和视频上下文补充](prd/023-图片和视频上下文补充.md) | FR-001-021 | P1 | draft | [计划](plans/023-图片和视频上下文补充计划.md) · planned |
| 024 | [离线可达提醒](prd/024-离线可达提醒.md) | FR-001-022 | P1 | draft | [计划](plans/024-离线可达提醒计划.md) · planned |
| 025 | [基于资料的问答与高级比较](prd/025-基于资料的问答与高级比较.md) | FR-001-023 | P2 | draft | [计划](plans/025-基于资料的问答与高级比较计划.md) · planned |
| 026 | [团队协作](prd/026-团队协作.md) | FR-001-024 | P2 | draft | [计划](plans/026-团队协作计划.md) · planned |
| 027 | [数据正确性](prd/027-数据正确性.md) · [Design](design/027-数据正确性设计.md) accepted | NFR-001-001 | P0 | draft | [计划](plans/027-数据正确性计划.md) · in_progress（S00 完成，0/6 AC） |
| 028 | [可追溯与可复现](prd/028-可追溯与可复现.md) · [Design](design/028-可追溯与可复现设计.md) accepted | NFR-001-002 | P0 | draft | [计划](plans/028-可追溯与可复现计划.md) · in_progress（S00/S01 完成，0/6 AC） |
| 029 | [时效与数据新鲜度](prd/029-时效与数据新鲜度.md) · [Design](design/029-时效与数据新鲜度设计.md) accepted | NFR-001-003 | P0 | draft | [计划](plans/029-时效与数据新鲜度计划.md) · in_progress（S00/S01 完成，0/6 AC） |
| 030 | [交互与检索性能](prd/030-交互与检索性能.md) | NFR-001-004 | P0 | draft | [计划](plans/030-交互与检索性能计划.md) · planned |
| 031 | [可靠执行与幂等](prd/031-可靠执行与幂等.md) · [Design](design/031-可靠执行与幂等设计.md) accepted | NFR-001-005 | P0 | draft | [计划](plans/031-可靠执行与幂等计划.md) · in_progress（S00—S02 完成，0/6 AC） |
| 032 | [备份与恢复](prd/032-备份与恢复.md) · [Design](design/032-备份与恢复设计.md) accepted | NFR-001-006 | P0 | draft | [计划](plans/032-备份与恢复计划.md) · in_progress（S00/S01 完成，0/6 AC） |
| 033 | [故障隔离与降级](prd/033-故障隔离与降级.md) | NFR-001-007 | P0 | draft | [计划](plans/033-故障隔离与降级计划.md) · planned |
| 034 | [凭据与应用安全](prd/034-凭据与应用安全.md) · [Design](design/034-凭据与应用安全设计.md) accepted | NFR-001-008 | P0 | draft | [计划](plans/034-凭据与应用安全计划.md) · in_progress（S00/S01 完成，0/6 AC） |
| 035 | [权限与数据隔离](prd/035-权限与数据隔离.md) · [Design](design/035-权限与数据隔离设计.md) accepted | NFR-001-009 | P0 | draft | [计划](plans/035-权限与数据隔离计划.md) · in_progress（S00/S01 完成，S02 当前资源通过，完整切片待接入，0/6 AC） |
| 036 | [数据访问与生命周期](prd/036-数据访问与生命周期.md) · [Design](design/036-数据访问与生命周期设计.md) accepted | NFR-001-010 | P0 | draft | [计划](plans/036-数据访问与生命周期计划.md) · in_progress（S00—S02 完成，0/6 AC） |
| 037 | [费用与资源约束](prd/037-费用与资源约束.md) · [Design](design/037-费用与资源约束设计.md) accepted | NFR-001-011 | P0 | draft | [计划](plans/037-费用与资源约束计划.md) · in_progress（S00—S02 完成，0/6 AC） |
| 038 | [可维护与可替换](prd/038-可维护与可替换.md) · [Design](design/038-可维护与可替换设计.md) accepted | NFR-001-012 | P0 | draft | [计划](plans/038-可维护与可替换计划.md) · in_progress（S00/S01 完成，0/6 AC） |
| 039 | [可观测与可运维](prd/039-可观测与可运维.md) · [Design](design/039-可观测与可运维设计.md) accepted | NFR-001-013 | P0 | draft | [计划](plans/039-可观测与可运维计划.md) · in_progress（S00/S01 完成，0/6 AC） |
| 040 | [可用性与可访问性](prd/040-可用性与可访问性.md) | NFR-001-014 | P0 | draft | [计划](plans/040-可用性与可访问性计划.md) · planned |
| 041 | [分析有效性与不确定性](prd/041-分析有效性与不确定性.md) | NFR-001-015 | P0 | draft | [计划](plans/041-分析有效性与不确定性计划.md) · planned |
| 042 | [容量与部署可重复性](prd/042-容量与部署可重复性.md) · [Design](design/042-容量与部署可重复性设计.md) accepted | NFR-001-016 | P0 | draft | [计划](plans/042-容量与部署可重复性计划.md) · in_progress |
| 043 | [模型服务接入与模型配置](prd/043-模型服务接入与模型配置.md) | FR-001-025 | P1 | draft | [计划](plans/043-模型服务接入与模型配置计划.md) · planned |
| 044 | [模型平台信息采集与变更监控](prd/044-模型平台信息采集与变更监控.md) | FR-001-026 | P1 | draft | [计划](plans/044-模型平台信息采集与变更监控计划.md) · planned |
| 045 | [模型联网检索与线索采集](prd/045-模型联网检索与线索采集.md) | FR-001-027 | P1 | draft | [计划](plans/045-模型联网检索与线索采集计划.md) · planned |

046 为公共技术前置：[Design](design/046-全局异常与响应契约设计.md) · accepted；[Plan](plans/046-全局异常与响应契约前置计划.md) · completed；[Acceptance](acceptance/046-全局异常与响应契约验收.md) · passed。不新增产品 PRD，不改变 43 项需求及优先级分母。

新增独立交付项从下一个未占用编号 **047** 登记；001 后续总体 Design、Plan、Acceptance 继续使用 001，003—046 后续同主题文档复用各自编号。文档规范见 [TEMPLATE.md](TEMPLATE.md)。只有真实实施与验证发生后，才建立 Acceptance。

## 本轮编号纠正

2026-09-17，按用户对空基线编号的纠正意见，修正两个尚未提交的草案：

| 已废止草案的旧编号 | 当前归属编号 | 处理 |
|---|---|---|
| 011 | 001 | 平台功能与非功能需求；文件名、doc_no、BR/FR/NFR/DEC/RSK/OPEN/AC 及交叉引用同步调整 |
| 010 | 002 | X 专项设计；文件名、doc_no、DEC/RSK/OPEN/AC 及交叉引用同步调整 |

此处只记录编号纠错，不保留重复正文或兼容文件。两个旧草案编号来自错误地沿用已清理历史，均在新台账冻结前废止；纠正不改变需求含义。随后从 003 连续分配逐项 PRD，当前 010 是“增量更新与历史回补”，011 是“事件识别与归并”，与上述废止草案无继承关系。今后的编号以本台账为准，不再重新解释已登记含义。001 的 Research 与 PRD 是同一平台交付项，不另占一个流水号。
