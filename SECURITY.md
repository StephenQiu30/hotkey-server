# 安全策略

HotKey 会处理账号与会话、来源配置、第三方凭据、原始证据、报告内容和 AI Provider 密钥。请负责任地报告影响前端、后端或部署链路的安全问题。

## 支持范围

| 版本 | 安全更新 |
| --- | --- |
| `main` / 最新发布版本 | 支持 |
| 历史提交与未维护分支 | 不支持 |

## 私密报告漏洞

请使用 GitHub 的 [Private Vulnerability Reporting](https://github.com/StephenQiu30/hotkey-server/security/advisories/new) 私密提交报告，不要创建包含漏洞细节的公开 Issue、Pull Request 或 Discussion。若私密入口不可用，只创建不含漏洞细节的 Issue 请求维护者提供私密联系方式。

报告最好包含受影响的版本或提交、页面或服务、漏洞类型、影响范围、前置条件、最小化复现步骤和可能的缓解措施。不要提交真实 Token、Cookie、密钥、邮箱、个人数据、完整数据库内容、第三方系统数据或可直接利用的攻击载荷。

## 响应目标

- 3 个工作日内确认收到报告。
- 14 天内完成初步评估并同步处理计划。
- 修复发布前与报告者协调披露时间。

## 重点关注领域

- 认证、会话、权限提升、刷新 Cookie、CSRF 与越权操作
- XSS、Markdown 渲染、开放重定向、不安全链接和客户端敏感信息泄漏
- SSRF、DNS Rebinding、来源连接器访问边界与第三方平台合规
- SQL 注入、任务重放、MinIO/Vault 路径和多租户数据隔离
- SMTP、Feed Token、AI Provider、环境变量、日志、错误响应、OpenAPI 与指标中的敏感信息
- Next.js rewrites、跨源 Cookie、CORS、反向代理、依赖与构建供应链风险

## 安全使用建议

- 生产环境使用 HTTPS、Secure Cookie、精确 CORS Origin 与受限网络边界。
- PostgreSQL、Redis、MinIO、SMTP 使用专用低权限账号，每个环境生成不同的 JWT 与 HMAC 密钥。
- 不把后端密钥放入 `NEXT_PUBLIC_*`，不把 `.env`、数据库 dump、Vault 或对象存储内容提交到 Git。
- 对外暴露前完成备份恢复、监控、升级与回滚演练，并持续使用受支持的 Go、Node.js 与依赖版本。
