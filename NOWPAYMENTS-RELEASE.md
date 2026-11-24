# NOWPayments 发版说明（给构建线）

## 要合并的提交

`acbbc7da940c86b041e5f3891b4e66f8dff14d24` — feat(payment): add NOWPayments cryptocurrency provider（已在 main 上）。

## 为什么不会覆盖构建线的更新

- 该提交是 2133d19c0 的直系后代，构建线的树正是基于 2133d19c0，合并是干净的快进式合并。
- 只动 25 个支付相关文件（backend/internal/payment/**、前端支付设置与方式标签、支付文档），无新增 migration，不碰社交登录、监控、profit 等构建线文件；与构建线自有迁移 317/318/322 零冲突。

## 步骤

1. 在构建线的树里 pull main（或 cherry-pick acbbc7d）。
2. 按原流程构建并发版（沿用 deploy-quiescent 即可）。

## 发版之后（自动，无需手动配置）

- NOWPayments 后台侧已就绪：KYC Verified、生产 API key 有效、IPN secret 已生成、回调地址已设为站内 webhook、USDT-TRC20 收款钱包已配（Main wallet）。
- 站内实例配置已在服务器预置一键脚本 `/opt/tokenrouter/nowpayments-setup.sh`（凭据在 `/opt/tokenrouter/nowpayments-creds.txt`，600 权限）。Muse 侧有自动巡检盯新二进制上线，上线后自动建实例（USD / usdttrc20 / qrcode）并做小额实测，无需构建线处理。

## 验证口径

新二进制含字符串 `api.nowpayments.io`；实例建好后支付方式出现 nowpayments，一笔小额 USDT 充值全程跑通（下单出地址 → 付款 → IPN 回调 → 余额到账）即算完成。
