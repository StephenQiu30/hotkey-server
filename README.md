# hotkey-server

`hotkey-server` 是 HotKey 内容创作者热点选题工具的 FastAPI 后端仓库。

本仓是跨仓规范主源，也是 Swagger/OpenAPI 契约事实源。`hotkey-web` 和 `hotkey-miniapp` 必须通过 `@umijs/openapi` 从本仓 OpenAPI 文档生成 API 客户端，不手写后端 API 类型。

## P0 职责

1. 基础账号体系：Web 邮箱/密码登录，小程序平台登录凭证换取后端会话。
2. 热点数据：公开源采集、标准化、去重、聚合和榜单排行。
3. AI 能力：热点快速理解、结构化摘要、风险点和内容选题生成。
4. 用户动作：收藏、关注、搜索、通知状态。
5. 接口契约：维护 Swagger/OpenAPI，并作为 Web 与小程序生成客户端的唯一事实源。

## 跨仓协作顺序

默认顺序：

```text
server -> web -> miniapp -> 回归
```

接口相关变更必须先在本仓完成测试、OpenAPI 更新和契约审查，再同步到 `hotkey-web` 与 `hotkey-miniapp`。

## 技术栈

- Python
- FastAPI
- PostgreSQL
- SQLAlchemy 2.0
- Swagger/OpenAPI
- OpenAI 兼容模型接口

## 规范文件

- [AGENTS.md](./AGENTS.md)：跨仓主规范源。
- [AGENTS.local.md](./AGENTS.local.md)：当前仓库局部补充规则。

## M0 验证

运行仓库治理基线测试：

```bash
python3 -m unittest discover -s tests -p 'test_repository_governance.py'
```

该测试用于确认本仓声明了后端职责、跨仓主规范源、Swagger/OpenAPI 契约源和生成客户端规则。

## OpenAPI 契约导出

Web 与小程序默认通过运行中的 Swagger/OpenAPI 文档接入，也可以导出静态契约文件用于 `@umijs/openapi` 生成客户端：

```bash
npm run openapi:export
```

默认输出为 `docs/openapi.json`。
