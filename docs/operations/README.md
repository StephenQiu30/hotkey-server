---
layer: Operations
doc_no: "000"
audience: [Dev, QA, Ops]
feature_area: 项目运行与发布
purpose: 定义 HotKey Server 发布、运行、回滚和故障手册的归档规则
canonical_path: docs/operations/README.md
status: review
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/README.md
outputs:
  - Operations 编写规范
triggers:
  - 新增发布、部署、回滚或运行流程
  - 运行方式或依赖恢复流程变化
downstream: []
---

# 运维文档规范

Operations 保存可重复执行的协作和运行流程，包括 Git/PR、发布、部署、备份、恢复、回滚、告警和故障处置。

## 必需内容

- 适用范围、前置权限和依赖
- 可复制命令与预期信号
- 失败判断、停止条件和回滚步骤
- 数据、密钥、日志和审计边界
- 验证方式和最后演练日期

## 收录边界

- 不放产品需求、架构设计或测试报告主体
- 不记录单次发布流水；只记录可重复流程
- 运行手册不得包含真实密钥、Token 或个人环境绝对路径
- 当前尚未设计部署拓扑，因此只建立规范入口，不虚构部署手册
