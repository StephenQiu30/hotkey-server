---
layer: Operations
scope: shared
doc_no: "000"
title: Operations 索引
status: active
version: v1.0
owner: HotKey Team
canonical_path: docs/operations/README.md
---

# Operations 索引

本目录保存 001–005 新基线所需的可重复运行流程。当前手册均为 `planned`：它们定义实施和验收必须满足的运维契约，但在同编号功能完成恢复演练并留下 Acceptance 证据前，不得作为生产操作依据。

| 编号 | 手册 | 覆盖范围 | 状态 |
|---:|---|---|---|
| 001 | [部署、升级与回滚](001-部署升级与回滚.md) | 预检、兼容顺序、健康判定、灰度与回滚 | planned |
| 002 | [备份、恢复与重建](002-备份恢复与重建.md) | PostgreSQL、MinIO、Vault、Redis 派生状态与恢复演练 | planned |
| 003 | [来源授权、预算与故障处置](003-来源授权预算与故障处置.md) | Capability、Rights、Credential、Budget、限流与停用 | planned |
| 004 | [可观测性、SLO 与事件响应](004-可观测性SLO与事件响应.md) | 指标、日志、Trace、告警、分级响应与容量 | planned |
| 005 | [保留、删除与撤权处置](005-保留删除与撤权处置.md) | Dry Run、审批、读取阻断、投影/对象删除与对账 | planned |
| 006 | [密钥轮换与泄漏响应](006-密钥轮换与泄漏响应.md) | 新凭据预检、兼容窗口、撤销、回退与泄漏处置 | planned |

生产入口仍以根 README 和已验收 Compose 配置为准。任何示例值都必须先替换并校验；恢复演练只允许在隔离环境和新存储上执行，不得覆盖生产事实。
