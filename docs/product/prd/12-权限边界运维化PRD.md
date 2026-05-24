---
layer: PRD
doc_no: "12"
audience:
  - PM
  - Dev
  - QA
  - Ops
feature_area: security-ops
purpose: "将 RBAC/权限异常运行边界转为可执行运维文档与排障动作。"
canonical_path: "docs/product/prd/12-权限边界运维化PRD.md"
status: draft
version: "0.1.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/08-安全增强-RBAC与权限治理PRD.md"
  - "docs/plans/15-安全增强与RBAC计划.md"
  - "server/app/api/deps.py"
outputs:
  - "可复核的角色边界定义与操作手册"
  - "权限异常排障流程与验证清单"
triggers:
  - "企业级 P0 进入交付期"
  - "角色/权限相关缺陷率不清晰"
downstream:
  - "docs/plans/20-权限边界与运维手册计划.md"
---

# 权限边界运维化 PRD（C1）

## 1. 背景

RBAC 已完成最小代码实现后，当前缺少可落地的运维文档和现场排障闭环。
该文档将角色边界从“功能实现”转为“可执行运维资产”。

## 2. 目标

- 明确 `admin`/`viewer`/`readonly` 的 API 与页面权限边界。
- 定义 403、token 失效、角色错误等常见故障的处理动作。
- 建立与部署配置、环境变量、回滚对应关系。

## 3. 非目标

- 不扩展新的角色模型；
- 不新增复杂组织级权限树；
- 不替代企业级 IAM。

## 4. 功能定义

- 角色边界定义
  - 列出每类路由的访问权限。

- 运维排障
  - 403 与鉴权错误快速确认；
  - token 失效恢复动作；
  - 权限回滚/调整步骤。

- 验证要求
  - 管理员可按文档独立完成一次权限校验流程。

## 5. 验收

- 完成 `docs/plans/20-权限边界与运维手册计划.md`。
- 更新 `docs/验收差距清单.md` 并标注 C1 文档复核状态。
- 文档中包含真实运行命令、日志查看、应急恢复路径。

## 6. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 0.1.0 | 新建 C1 权限运维化 PRD |
