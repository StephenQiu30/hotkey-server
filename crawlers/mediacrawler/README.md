# MediaCrawler（本机，P2-1）

- 上游：`https://github.com/NanmiCoder/MediaCrawler`，固定提交 `380b426000aac3d612837ed72c99808347dc94c9`；许可为非商业学习许可，仅用于个人研究。
- 位置：`~/Desktop/StephenQiu/MediaCrawler`，本地分支 `hotkey-safe` = 固定提交 + `hotkey-safe.patch`。
- 安装：`git clone` 后 `git checkout 380b4260 && git switch -c hotkey-safe && git am <本目录>/hotkey-safe.patch && uv sync --frozen`。

## 安全补丁（使用者本人账号，不触发风控）

- 不注入 `stealth.min.js`；出现滑块/验证码时直接停止，不自动破解。
- 调试端口只绑定 `127.0.0.1`（上游为 `0.0.0.0`，会把已登录浏览器暴露给局域网）。
- 不连接日常 Chrome：每个平台一个独立用户资料（`browser_data/cdp_<平台>_user_data_dir`），首次扫码登录后保留。
- 低频：请求间隔 8 秒、并发 1、每次最多 10 条内容、每条最多 20 条一级评论、不采楼中楼。

## 登录状态

| 平台 | 状态 |
|---|---|
| B 站 | 已扫码登录（2026-09-26），试采 19 条视频、272 条评论 |
| 微博、小红书、抖音 | 待扫码 |
