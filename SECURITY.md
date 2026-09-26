# 安全策略

HotKey 正在开发中，尚无经过完整产品与公网部署验收的正式版本。当前维护 `main`；历史提交和未维护分支不提供安全更新承诺。

## 私密报告漏洞

请优先使用 GitHub 的 [Private Vulnerability Reporting](https://github.com/StephenQiu30/hotkey-server/security/advisories/new)。如果入口不可用，可创建**不含漏洞细节**的 Issue，请维护者提供私密联系方式。不要在公开 Issue、Pull Request 或 Discussion 中发布可利用细节。

报告时请提供受影响提交或版本、影响范围、复现前置条件、最小化且已脱敏的步骤及可能的缓解措施。不要提交真实密码、Token、Cookie、账号会话、连接字符串、私有地址或用户内容。

## 部署与数据边界

- 默认 Compose 端口只绑定本机回环地址；面向公网部署需要单独配置 HTTPS、随机凭据、来源限制和备份恢复，并验证生产环境。
- 只采集公开或获授权内容；来源的访问条件、限流、失败状态与数据保留需分别验证。代码适配器、模拟测试和健康检查不表示某平台已获授权或可用。
- `.env`、来源凭据、浏览器会话和真实证据不应进入 Git、日志或公开反馈。密钥泄露时请先撤销或轮换。

具体部署、身份与数据库限制见 [后端说明](backend/README.md) 和 [项目约束](PROJECT.md)。
