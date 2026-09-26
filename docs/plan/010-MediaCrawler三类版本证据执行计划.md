---
layer: Plan
scope: issue
doc_no: "010"
title: MediaCrawler 三类版本证据执行计划
status: planned
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/010-MediaCrawler三类版本证据执行计划.md
prd: docs/prd/003-本人账号B站试点需求.md
design: docs/design/003-本人账号B站试点设计.md
architecture_prerequisite: "046 S03"
source_task: TASK-003-S01-T01
---

# Plan 010：MediaCrawler 三类版本证据

## 范围与需求

旧 B 站预设的 `component_version` 仍是旧值；离线回放不能证明实际运行工作树版本。本卡按 [PRD 003](../prd/003-本人账号B站试点需求.md) `FR-003-002`、`NFR-003-001` 和 `AC-003-001/002/003`，分开固定 MediaCrawler 上游基线、补丁后提交及 HotKey 适配器版本，并关联每个 Job。Plan 002 的节奏契约先确定；仅版本只读核查可并行。预计检查宿主机固定补丁工作树、`backend/app/connections/{presets,services}.py`、B 站适配器、Job 版本字段和测试；与 Plan 011 共用文件时串行修改。

## SPEC

| ID | 规格 |
|---|---|
| `SPEC-010-SEC-001` | 运行前核对上游基线、补丁后 HEAD 和已跟踪文件未改动；固定工作树不符则拒绝启动，不读取任意路径或回显资料。 |
| `SPEC-010-DATA-001` | Job 分别记录上游基线、补丁提交、实际适配器版本及来源连接版本，不能以单个 `component_version` 混记三类事实。 |
| `SPEC-010-JOB-001` | 版本校验先于子进程启动；校验失败记明确停止状态，不消耗真实采集结果名额。 |

## 验收与 Checklist

Given：固定补丁工作树和三类版本标识可读。When：分别以正确、脏工作树、错 HEAD 启动受控 Job。Then：仅符合固定版本的 Job 可以继续，版本证据在 Job 可独立追溯。

- [ ] `CHK-010-G0-001`：核对 PRD/Design、046 S03、Plan 002 与当前预设差异，列确切文件。
- [ ] `CHK-010-G1-001`：冻结三类版本字段、校验顺序和错误码。
- [ ] `CHK-010-G2-001`：先保存正确/错误 HEAD、已跟踪文件改动和版本缺失的失败验证。
- [ ] `CHK-010-G3-001`：实现并核对 `SPEC-010-SEC-001`、`SPEC-010-DATA-001`、`SPEC-010-JOB-001` 与 Job 关联。
- [ ] `CHK-010-G4-001`：运行 Ruff/mypy/pytest、真实库 Job 持久化和进程启动边界测试。
- [ ] `CHK-010-G5-001`：本人账号真实低频采集前只读核对三类实际版本，并在 Plan 012 的真实 Job 验证关联。
- [ ] `CHK-010-G6-001`：记录 `AC-003-001/002/003` 的版本部分；采集和风控结果另验。

失败时保持 B 站来源关闭，保留原 Job 和错误证据；不得自动切换工作树或补丁。受控、真实与产品证据分别登记。
