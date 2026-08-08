---
layer: Design
scope: shared
doc_no: "005"
title: 监控主题CRUD与生命周期设计
status: accepted
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/design/archive/005-监控主题CRUD与生命周期设计.md
prd: docs/prd/archive/005-监控主题CRUD与生命周期.md
plan: docs/plans/archive/005-监控主题CRUD与生命周期计划.md
---

# 监控主题CRUD与生命周期设计

## 现状

监控创建、草稿替换、预览、发布、暂停、恢复、归档、还原和删除接口已存在，前端热点监控页已接入主要动作。

## 目标

用户可以像管理关键词任务一样创建、启停和删除监控，同时保留版本、来源、规则和历史事件的可追溯性。

## 非目标

本条不跨越相邻编号实现其业务细节，也不建立第二套身份、任务、数据、API 或 UI 事实源。

## 核心决策

- Monitor 是稳定身份，MonitorConfigVersion 是不可变配置；编辑只改草稿，发布产生新版本。
- 状态固定为 draft、active、paused、archived；删除仅隐藏控制面记录，不级联删除历史内容、事件和报告。
- 发布前必须预览命中样本、估算来源与频率成本并校验至少一条有效 include 规则。
- 列表直接提供启停动作，详情提供版本、来源、规则、阈值和最近运行状态。

## 数据、接口与交互

- 监控列表、创建、编辑和删除
- 草稿预览与发布
- 启用、暂停、归档和恢复
- 版本历史与运行摘要

所有新增跨端字段先进入 `backend/db/schema.sql` 和后端 DTO/Swaggo 注解，再生成 `docs/openapi/swagger.json` 与前端 Client。页面同时提供正常、加载、空、错误和权限不足状态。

## 安全、失败与降级

把暂停误实现为删除会破坏历史归因；调度器必须只读取已发布且 active 的版本。

失败必须被分类、可观测且不破坏历史事实；可选外部依赖不可用时，核心读取链路继续工作。

## 设计验收边界

- 发布版本在后续编辑中不可变
- 暂停后不再产生新计划采集，恢复后从新窗口继续
- 重复动作幂等且冲突返回明确业务码
