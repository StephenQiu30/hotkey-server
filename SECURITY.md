# 安全策略

HotKey 会处理账号、来源配置、第三方凭据、原始证据和 AI Provider 密钥。请负责任地报告安全问题，避免给自托管实例和上游平台带来风险。

## 支持范围

| 版本 | 安全更新 |
|------|----------|
| `main` / 最新发布版本 | 支持 |
| 历史提交与未维护分支 | 不支持 |

1.0 之前接口和部署方式可能变化，安全修复优先进入 `main`。

## 私密报告漏洞

请使用 GitHub 的 [Private Vulnerability Reporting](https://github.com/StephenQiu30/hotkey-server/security/advisories/new) 提交报告，不要创建公开 Issue、Pull Request 或 Discussion。

如果私密报告入口不可用，请仅创建一个不含漏洞细节的 Issue，请求维护者提供私密联系方式。不要粘贴 Token、密钥、个人数据、完整请求、数据库内容或可直接利用的复现代码。

报告最好包含：

- 受影响的版本或提交
- 漏洞类型、影响范围和所需前置条件
- 最小化复现步骤或概念验证
- 可能的缓解措施
- 是否已在真实系统或第三方服务上测试

## 响应目标

- 3 个工作日内确认收到报告。
- 14 天内完成初步评估并同步处理计划。
- 修复发布前与报告者协调披露时间。

复杂问题可能需要更长时间，我们会尽量保持进度透明。

## 重点关注领域

- 认证、会话、权限提升和刷新 Cookie
- SSRF、DNS Rebinding 与来源连接器访问边界
- MinIO 证据、Vault 路径和多租户数据隔离
- SQL 注入、越权查询和任务重放
- SMTP、Feed Token、AI Provider 与环境变量密钥泄漏
- 日志、错误响应、OpenAPI 或指标中的敏感信息
- 依赖与构建供应链风险

## 安全使用建议

- 为 PostgreSQL、Redis、MinIO 和 SMTP 使用专用低权限账号。
- 每个环境生成不同的 JWT 与 HMAC 密钥，并启用 HTTPS 与安全 Cookie。
- 不要把 `.env`、数据库 dump、Vault 或对象存储内容提交到 Git。
- 对外暴露前配置网络边界、备份、监控和升级流程。
- 只接入官方 API、公开 RSS/Atom 或明确授权的数据源。

感谢你帮助保护 HotKey 社区。
