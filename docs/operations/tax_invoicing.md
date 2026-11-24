# 自营订单数电开票

本功能接入 PerPay 商户签名开票接口，处理经财务确认可开票的微信自营订单。支付和权益规则见[支付与权益](../domains/payments_and_entitlements.md)，部署迁移遵循[迁移执行](deployment_and_migrations.md#migration_execution)。

## 启用条件

1. 发票渠道已正式开通，销方、税号、税目、税率和开票员经财务确认。微信商户登录或 OAuth 授权本身不代表这些条件齐备。
2. PerPay 已提供 `/pay/epay/tax-invoices/{applications,query,files}`，公开 nginx 路由能命中对应服务。配置主站商户 HMAC 凭据，不能复用 EasyPay pid/MD5 密钥。
3. 业务库已应用 migration 319。服务默认关闭；关闭时保留本人历史申请查询，停止新申请、后台恢复和文件获取。
4. 财务在受信数据库管理会话为每笔订单记录资格。用户接口只接收购方资料，不能自认资格、指定金额、销方或税率。

## 部署配置

以下均为服务进程环境变量，变更后按正常发布流程生效。示例不含可用凭据。

```dotenv
TOKENROUTER_TAX_INVOICE_ENABLED=false
TOKENROUTER_TAX_INVOICE_ENDPOINT=https://star-web.top/pay/epay/tax-invoices/
TOKENROUTER_TAX_INVOICE_MERCHANT_ID=<主站商户ID>
TOKENROUTER_TAX_INVOICE_API_ID=<主站HMAC API ID>
TOKENROUTER_TAX_INVOICE_API_SECRET=<服务端密钥>
TOKENROUTER_TAX_INVOICE_EPAY_PID=<实际收款pid>
TOKENROUTER_TAX_INVOICE_PROVIDER_INSTANCE_ID=<本业务支付实例ID>
```

开启时缺失或无效配置使服务初始化失败。密钥仅由后端读取，不进入前端构建、订单快照或日志。当前通道支持微信订单；支付宝订单不会进入申请流程。

## 财务确认与历史订单

1. 核对真实自营交易、开票时点、实际商品服务、合同或消费依据。余额充值成功不等于消费完成；按消费或月结开票需要独立的消费归集模型，本功能不实现该资格计算。
2. 核对 `payment_orders` 的归属、`COMPLETED`、`paid_at`、退款字段、实际 CNY `pay_amount`、回调确认的 `payment_trade_no`、EasyPay 实例和 pid。发票金额固定为整笔实际人民币收款，不采用页面 USD 余额。
3. 由受信运维/财务在数据库记录 `payment_tax_invoice_eligibility`：`order_id`、可追溯的 `reference`、真实 `approved_by`。以下参数必须由实际核验资料填写，不可批量给所有充值授权。

```sql
INSERT INTO payment_tax_invoice_eligibility (order_id, reference, approved_by)
VALUES ($1, $2, $3);
```

资格撤销写 `revoked_at`；已提交申请保留原审核快照和原请求，不删除或改写历史购方。该表没有用户写入接口。未来如增加管理界面，必须保持受信权限和审核凭据要求。

新 EasyPay 订单保存明确 CNY 币种。历史订单若缺少 `provider_snapshot.currency` 会被拒绝，不能将界面兼容默认值当作已核验事实；核实原始支付记录后，才能按单独审批流程补齐历史快照。本次迁移不自动回填。

## 申请与恢复

| 接口 | 返回或效果 |
| --- | --- |
| `GET /api/v1/payment/orders/:id/tax-invoice` | 登录用户所属订单的资格和本地发票状态，不调用上游 |
| `POST /api/v1/payment/orders/:id/tax-invoice` | 接受 `buyer_type/title/tax_id/email`，持久保存原申请后提交 |
| `GET /api/v1/payment/orders/:id/tax-invoice/file` | 仅启用状态且已开票时获取新的临时 HTTPS 文件链接 |

个人税号为空，企业需要税号；抬头与邮箱必填。申请创建后购方、订单、金额、渠道和资格快照不可修改。重复提交相同资料返回原记录，冲突资料被拒绝。

后台每 30 秒处理当前入口、主站商户、支付 pid 和支付实例下最多 20 笔到期 UNKNOWN/PROCESSING/REVERSING，以数据库租约避免并发覆盖。旧绑定申请保留在库，恢复原绑定后继续处理，不占用当前绑定的批次。未收到平台发票 ID 时，恢复复用原 requestId 和原正文；已收到 ID 后只查询原记录。超时、503、响应身份/金额不一致和未知失败码保留不确定状态。只有 ISSUED 且带发票号才显示已开票。

订单页按需加载详情；自动轮询只查询原申请。提交结果不明且尚无持久记录时，用户可明确重试原购方快照；确定输入拒绝后，只有查询确认没有申请才开放修正。页面每 30 秒最多轮询 20 次，切到后台或关闭弹窗停止轮询。临时下载链接过期后重新获取。

## 退款与红冲边界

UNKNOWN/PROCESSING/ISSUED/REVERSING 与退款在同一订单行上互斥。已核验的 FAILED（未发送、明确参数/权限拒绝或微信 ISSUE_FAILED）和 REVERSED 允许退款；传输失败不能转为 FAILED 来解锁。

当前不发起红冲。PerPay 签名 `/query` 读取平台本地状态，不等于即时向税局核验；平台文件接口会再核查已开票状态。外部红冲需在平台受控刷新并同步业务记录后才能解除业务退款锁，当前没有业务侧自动红冲同步闭环。不得直接删申请或手工将未知结果改成 FAILED。

## 验收范围

本地 TLS 夹具、PostgreSQL 事务/竞态、前端类型和测试仅证明程序行为。上线前还必须验证正式渠道授权、正确税目/税率、公开代理路由，以及一笔真实已确认可开票订单的票号、金额、购方和最终票面文件。未完成这些验证时保持配置关闭。
