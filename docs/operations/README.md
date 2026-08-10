---
layer: Operations
scope: shared
doc_no: "000"
title: Operations 索引
status: active
version: v2.3
owner: HotKey Team
canonical_path: docs/operations/README.md
---

# Operations 索引

本目录保存可重复执行的运维流程；`active` 表示已经随功能验收，`planned` 只表示实现必须满足的运维契约，不得用于生产。命令中的示例值必须先替换并校验；任何恢复演练都先使用独立 Compose 项目和新卷，禁止直接覆盖生产卷。

| 编号 | 手册 | 覆盖范围 | 状态 |
| --- | --- | --- | --- |
| 001 | [部署、升级与回滚](001-部署升级与回滚.md) | 预检、发布、readiness、兼容回滚 | active |
| 002 | [备份、恢复与密钥轮换](002-备份恢复与密钥轮换.md) | PostgreSQL、MinIO、Vault、轮换演练 | active |
| 003 | [来源授权与故障处置](003-来源授权与故障处置.md) | 七类已实现来源的准入、探测、停用与恢复 | active |
| 004 | [可观测性、SLO 与容量基线](004-可观测性SLO与容量基线.md) | 指标词典、阈值、排障路径、容量复核 | active |
| 005 | [032 非空库升级与证据对账](005-032非空库升级与证据对账.md) | canonical Schema 非空升级、批次回填、重入、三存储对账与切流回滚 | active |
| 006 | [自动决策质量、模型成本与来源权利](006-自动决策质量、模型成本与来源权利.md) | 时间隔离质量门禁、AI 成本/降级、七类 Connector 单动作 Rights | active |

部署入口与完整质量命令见根目录 [README](../../README.md)。
