# CLAUDE.local.md — hotkey-server 局部项目规范

本文件存放当前项目特有的路径、环境配置和临时协作约定。

## 工作目录

```
/Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
```

## 跨仓顺序

接口、数据模型或 Swagger 契约变更默认顺序：`hotkey-server` → `hotkey-web` → `hotkey-miniapp` → 回归。

## 角色与流程

- 角色配置：`.claude/agents/`
- 可复用流程：`.claude/skills/`
- Symphony 调度：`WORKFLOW.md`