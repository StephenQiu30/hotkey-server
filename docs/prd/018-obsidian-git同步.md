# 018 Obsidian Git 同步

## 功能目标

通过用户连接的 Git 仓库，把 HotKey 生成的事件笔记、日报周报索引和主题目录同步到用户的 Obsidian Vault 指定目录。

## 用户故事

1. 作为用户，我可以连接 GitHub 或其他支持的 Git provider。
2. 作为用户，我可以选择仓库、分支和 Vault 子目录。
3. 作为用户，我希望每个事件都生成结构清晰的 Markdown 笔记。
4. 作为用户，我可以看到最近同步状态、提交链接和失败原因。

## 范围

1. Git 凭据授权和撤权。
2. 仓库、分支、目录校验。
3. Markdown 模板和文件路径生成。
4. 提交、冲突检测、重试和幂等。
5. 事件笔记、日报索引和周报索引同步。

## 非范围

1. Obsidian 插件。
2. 双向编辑同步。
3. 复杂 Git merge UI。

## 状态模型

Git 连接状态：`connected`、`expired`、`permission_limited`、`revoked`。

同步任务状态：`queued`、`rendering`、`committing`、`conflicted`、`completed`、`failed`。

文件状态：`created`、`updated`、`unchanged`、`skipped`。

## 契约影响

OpenAPI 必须支持 Git 连接、仓库选择、目录校验、同步状态、最近提交链接和失败原因。事件和报告接口必须返回对应 Obsidian 文件路径和 commit URL。

## SDD 要求

1. 先定义 Markdown 文件模板和 frontmatter schema。
2. 先定义路径命名、非法字符处理和重复标题策略。
3. 先定义 Git 提交幂等键和冲突处理规则。
4. 先定义 Git provider 抽象。

## TDD 要求

1. 先写 Markdown 渲染快照测试。
2. 先写路径生成和非法字符测试。
3. 先写重复同步不产生重复文件测试。
4. 先写 Git 冲突和权限不足测试。
5. 先写撤权后同步任务停止测试。

## E2E 验收门禁

1. 用户连接测试 Git 仓库并完成目录校验。
2. 事件摘要生成后提交 Markdown 到指定目录。
3. Web 显示 commit 链接。
4. Git 权限失效时同步失败状态用户可见。
5. 冲突时不覆盖用户已有内容，并给出可恢复状态。

## 数据与权限

1. Git token 加密存储。
2. 提交内容不包含平台授权凭据。
3. 用户撤权后不得继续提交。

## 依赖 PRD

1. `017-ai事件摘要与时间线.md`
2. `021-审计撤权与删除.md`
