# Telegram 和 Discord 登录配置

TokenRouter 的「系统设置 → 身份认证」提供 Telegram、Discord 独立配置卡片。用户可在登录页使用这两个入口，也可在个人资料页绑定或解绑账号。

## 需要准备的内容

1. Telegram：BotFather 中一个机器人的 **Login Widget Client ID 和 Client Secret**。这里使用 Telegram 登录专用凭据，不能用 Bot API Token 代替 Client Secret。
2. Discord：Developer Portal 中一个应用的 **OAuth2 Client ID 和 Client Secret**。不需要 Bot Token 或服务器管理员权限。
3. 当前站点的公网 HTTPS 地址。后台卡片显示完整回调地址，可直接复制；密钥只填写到后台，不放入前端构建变量或聊天记录。

回调地址留空时，后端采用发起授权请求的公网 API 来源。一般同域部署的地址为：

```text
https://你的现有站点域名/api/v1/auth/oauth/telegram/callback
https://你的现有站点域名/api/v1/auth/oauth/discord/callback
```

平台登记的是这两个后端地址。`/auth/telegram/callback` 和 `/auth/discord/callback` 是浏览器完成页面，不填进平台的回调配置。

## Telegram

1. 打开 Telegram 的 [BotFather](https://t.me/BotFather)，进入机器人管理 mini app，选择机器人并打开 **Login Widget**。
2. 在 **Redirect URIs** 填写后台卡片显示的完整回调地址，在 **Trusted Origins** 填写网站来源（例如 `https://shop.example`）。域名、路径和尾部斜杠须与实际请求一致。填写后离开并重新打开 Login Widget，确认两项均已保存。
3. 取得登录用 Client ID 和 Client Secret，签名算法保留 **RS256**。
4. 在 TokenRouter 的 Telegram 卡片填写凭据，启用后保存。回调地址可留空沿用当前站点；使用独立 API 域名或特殊反向代理路径时，填写与平台登记完全一致的后端地址。
5. 在未登录窗口点击 Telegram 登录。首次使用会进入创建本地账号或验证已有账号的流程；已有绑定则进入登录，开启的本地 TOTP 仍须完成。

授权范围为 `openid profile`。TokenRouter 使用授权码、S256 PKCE 和 nonce，并校验 ID Token 的签名、签发者、受众、有效期和稳定 `sub`。不请求电话号码或发送消息权限。完整平台说明见 [Telegram Login](https://core.telegram.org/bots/telegram-login)。

## Discord

1. 打开 [Discord Developer Portal](https://discord.com/developers/applications)，创建或选择应用。
2. 在 **OAuth2** 页面取得 Client ID 和 Client Secret，将后台显示的 Discord 完整回调地址加入 **Redirects** 并保存。
3. 将凭据填写到 TokenRouter 的 Discord 卡片，启用后保存。
4. 从未登录窗口发起 Discord 登录，检查授权页面只请求账号资料和邮箱。个人资料页可单独验证绑定和解绑。

授权范围固定为 `identify email`，使用授权码和 S256 PKCE。账号归属以 Discord 用户 ID 为准，已验证邮箱只作为注册填写建议，不自动合并同邮箱账号。完整协议见 [Discord OAuth2](https://docs.discord.com/developers/topics/oauth2)。

## 部署与验收

1. 前后端一同更新，启动时执行迁移 `322_social_login_providers.sql`。先备份数据库；产生新身份后，回退旧二进制不保证能识别这些身份。
2. HTTPS 反向代理须保留公网 Host，并按站点既有可信代理规则传递 HTTPS 信息。Cookie 的域和路径需覆盖开始授权、回调与 pending 请求；拆分域名时应通过同一浏览器 API 来源代理这些路径。
3. 后端须能访问 `oauth.telegram.org`、`discord.com` 的 token、JWKS 或用户接口。浏览器须能访问两个平台的授权页。平台授权页面要求 HTTPS，本机开发例外不代表平台一定接受该回调。
4. 验证首次注册、已有绑定登录、登录后绑定、身份已属他人时拒绝绑定、TOTP、解绑最后一个登录方式时拒绝，以及退出后旧回调不能继续完成。
5. 验证后台保存后密钥输入框清空、显示“已配置”，再次留空保存仍可登录。关闭渠道后隐藏新登录入口，已有绑定记录继续显示。

启用前缺凭据会拒绝保存；授权失败不会发放本地登录令牌。Telegram 不提供邮箱，若站点启用了注册邮箱验证，需要先配置可用 SMTP。两种渠道均沿用站点的注册开关、邀请与邮箱政策；当前没有独立的 Telegram/Discord 首次绑定赠送配置。

## 常见故障

- `redirect_uri` 不匹配：对比平台登记地址和后台卡片显示值，检查 `https`、域名、端口、路径与尾部斜杠。
- 授权返回提示会话过期：使用同一浏览器窗口重新发起，检查 Cookie、反向代理 Host 和 HTTPS 识别；授权状态有效期为 10 分钟。
- Telegram 身份验证失败：检查 Client ID、Login Widget 专用 Secret、RS256 设置、服务器时钟和 JWKS 出站访问。
- Discord `invalid_issuer`：授权回调应含 `iss=https://discord.com`，代理不得移除 query 参数。
- 新账号需要邮箱验证但收不到邮件：先验证站点 SMTP 实际投递，再重试注册。外部账号绑定已有用户时必须提供本地账号证明。
