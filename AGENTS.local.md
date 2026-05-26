# hotkey-server 局部规范

本文件补充 `AGENTS.md` 在当前仓库的局部执行规则。

## 当前阶段

当前仓库已经进入 Go 后端全面重建阶段。旧 FastAPI、Python、SQLAlchemy、旧 Docker/Compose、旧 SQL 初始化和旧 OpenSpec 实现内容不再作为事实源。

## 保留范围

当前阶段只保留以下长期有价值内容：

1. 开源治理文件：`README.md`、`LICENSE`、`CONTRIBUTING.md`、`AGENTS.md`、`AGENTS.local.md`。
2. 文档事实源：`docs/` 下的 PRD、Plan、工程设计、验收入口、运维入口和模板。
3. 任务工具配置：`.codex/` 与 `openspec/config.yaml`。
4. CI 入口：`.github/workflows/ci.yml`。

## 清理规则

1. 不保留旧实现代码、旧测试、旧运行时配置、旧构建脚本和一次性过程文件。
2. 不新增临时草稿、缓存、导出产物或中间状态文件到 Git。
3. 新 Go 代码必须从对应 PRD/Plan 任务开始创建，并同步 OpenAPI 影响。
4. 每次提交前必须确认 PRD 与 Plan 编号连续、路径一致、没有旧 FastAPI 事实源残留。

## 实现入口

Go 实现应从 `docs/product/prd/2-Go服务基础工程与OpenAPIPRD.md` 与 `docs/plans/2-Go服务基础工程与OpenAPI实现计划.md` 开始。
