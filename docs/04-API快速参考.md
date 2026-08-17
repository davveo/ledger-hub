# Ledger Hub API 快速参考

## 1. 接入地址与鉴权

生产接入统一经过 Gateway，默认本地地址为：

```text
http://127.0.0.1:8088
```

API 前缀：

```text
/api/v1/ledger
```

请求头：

| Header | 说明 |
|---|---|
| `X-Client-Id` | 管理员分配的客户端 ID |
| `X-Timestamp` | Unix 秒时间戳 |
| `X-Signature` | HMAC-SHA256 十六进制签名 |
| `X-Tenant-Id` | 当前租户，未传时使用服务默认租户 |
| `X-Request-Id` | 推荐传入，用于链路排查 |
| `Content-Type` | 写请求使用 `application/json` |

当前签名算法：

```text
hex(HMAC-SHA256(secret, client_id + timestamp + raw_body))
```

请求时间默认允许与服务端相差 300 秒。写请求体中的 `source_system` 必须与 `X-Client-Id` 一致。

Go 项目可使用 `pkg/client` 自动完成签名和请求。

## 2. 通用规则

### 2.1 金额

- 所有金额均为资产最小单位整数。
- JSON 使用字符串，避免精度丢失。
- 精度为 2 的资产中，`"10000"` 表示 `100.00`。

### 2.2 幂等

推荐业务号格式：

```text
{source_system}:{biz_type_short}:{natural_id}
```

例如：

```text
order:freeze:O20260817001
pay:credit:P20260817001
wallet:fx:F20260817001
```

同一动作重试必须保持 command、`source_system`、`biz_no` 和参数不变。成功重放返回首次结果，并带 `idempotent_replay=true`。

### 2.3 响应结构

成功：

```json
{
  "code": 0,
  "data": {}
}
```

失败：

```json
{
  "code": 42201,
  "message": "余额不足"
}
```

## 3. 记账命令

### 3.1 Credit 入账

```http
POST /api/v1/ledger/commands/credit
```

```json
{
  "source_system": "campaign",
  "biz_type": "signin",
  "biz_no": "campaign:signin:u123:20260817",
  "holder": {"type": "user", "id": "u123"},
  "asset_code": "POINT",
  "amount": "100"
}
```

### 3.2 Debit 扣款

```http
POST /api/v1/ledger/commands/debit
```

请求结构与 Credit 相同。余额不足返回 `42201`。

### 3.3 Freeze 冻结

```http
POST /api/v1/ledger/commands/freeze
```

```json
{
  "source_system": "order",
  "biz_type": "order_freeze",
  "biz_no": "order:freeze:O20260817001",
  "holder": {"type": "user", "id": "u123"},
  "asset_code": "POINT",
  "amount": "50",
  "expire_at": "2026-08-17T12:30:00+08:00"
}
```

保存响应中的 `freeze_id`。

### 3.4 Capture 确认冻结

```http
POST /api/v1/ledger/commands/capture
```

```json
{
  "source_system": "order",
  "biz_type": "order_capture",
  "biz_no": "order:capture:O20260817001",
  "related_biz_no": "order:freeze:O20260817001",
  "freeze_id": "fz_xxx",
  "asset_code": "POINT",
  "amount": "30"
}
```

省略 `amount` 表示整单确认；传入金额表示部分确认。

### 3.5 Release 释放冻结

```http
POST /api/v1/ledger/commands/release
```

```json
{
  "source_system": "order",
  "biz_type": "order_release",
  "biz_no": "order:release:O20260817001",
  "related_biz_no": "order:freeze:O20260817001",
  "freeze_id": "fz_xxx",
  "asset_code": "POINT"
}
```

### 3.6 Transfer 同资产转账

```http
POST /api/v1/ledger/commands/transfer
```

```json
{
  "source_system": "wallet",
  "biz_type": "user_transfer",
  "biz_no": "wallet:transfer:T20260817001",
  "holder": {"type": "user", "id": "u123"},
  "to_holder": {"type": "user", "id": "u456"},
  "asset_code": "POINT",
  "amount": "100"
}
```

### 3.7 Exchange 跨资产兑换

```http
POST /api/v1/ledger/commands/exchange
```

```json
{
  "source_system": "wallet",
  "biz_type": "fx_convert",
  "biz_no": "wallet:fx:F20260817001",
  "holder": {"type": "user", "id": "u123"},
  "from": {"asset_code": "BALANCE_CNY", "amount": "10000"},
  "to": {"asset_code": "BALANCE_USD", "amount": "1400"},
  "fx": {
    "rate": "0.14000000",
    "base_asset": "BALANCE_CNY",
    "quote_asset": "BALANCE_USD",
    "rate_source": "pricing"
  },
  "fee": {"asset_code": "BALANCE_CNY", "amount": "10"},
  "tolerance": "0"
}
```

`to.amount` 可省略，由账本按汇率计算。也可使用 `fx.rate_id` 引用已登记汇率。

### 3.8 Reverse 冲正

```http
POST /api/v1/ledger/commands/reverse
```

```json
{
  "source_system": "wallet",
  "biz_type": "ops_reverse",
  "biz_no": "wallet:reverse:R20260817001",
  "related_biz_no": "wallet:fx:F20260817001",
  "holder": {"type": "user", "id": "u123"},
  "asset_code": "BALANCE_CNY"
}
```

冲正必须使用新的 `biz_no`，`related_biz_no` 指向原业务。Exchange 等多分录 Journal 会整笔冲正。

## 4. 账户与流水查询

```http
POST /api/v1/ledger/accounts/open
GET  /api/v1/ledger/accounts?holder_type=user&holder_id=u123
GET  /api/v1/ledger/accounts?holder_type=user&holder_id=u123&asset_code=POINT
GET  /api/v1/ledger/accounts/{account_id}
GET  /api/v1/ledger/accounts/{account_id}/entries
GET  /api/v1/ledger/entries?holder_type=user&holder_id=u123&asset_code=POINT&limit=50&offset=0
GET  /api/v1/ledger/entries?biz_no=order:freeze:O20260817001
GET  /api/v1/ledger/journals/{journal_id}
```

流水时间参数使用 RFC3339：

```text
from=2026-08-17T00:00:00%2B08:00
to=2026-08-18T00:00:00%2B08:00
```

## 5. 冻结查询

```http
GET /api/v1/ledger/freezes/{freeze_id}
GET /api/v1/ledger/freezes?biz_no=order:freeze:O20260817001
GET /api/v1/ledger/freezes?holder_type=user&holder_id=u123&asset_code=POINT&status=frozen
GET /api/v1/ledger/freezes?expired=1&limit=50
```

冻结状态：`frozen`、`captured`、`released`。

## 6. 资产、租户和汇率

```http
POST /api/v1/ledger/assets
GET  /api/v1/ledger/assets
GET  /api/v1/ledger/assets/{asset_code}
POST /api/v1/ledger/assets/{asset_code}/disable
POST /api/v1/ledger/assets/{asset_code}/enable

POST /api/v1/ledger/tenants
GET  /api/v1/ledger/tenants
GET  /api/v1/ledger/tenants/{tenant_id}
POST /api/v1/ledger/tenants/{tenant_id}/disable
POST /api/v1/ledger/tenants/{tenant_id}/enable

POST /api/v1/ledger/fx/rates
GET  /api/v1/ledger/fx/rates
GET  /api/v1/ledger/fx/rates/{rate_id}
```

这些接口属于管理能力，不应开放给普通业务客户端。

## 7. 对账

```http
POST /api/v1/ledger/reconcile/jobs
GET  /api/v1/ledger/reconcile/jobs?limit=20
GET  /api/v1/ledger/reconcile/jobs/{job_id}
GET  /api/v1/ledger/reconcile/reports/{date}?source_system=order&asset_code=POINT
GET  /api/v1/ledger/reconcile/files
GET  /api/v1/ledger/reconcile/files/{name}
GET  /api/v1/ledger/reconcile/diffs
POST /api/v1/ledger/reconcile/diffs/{diff_id}/resolve
```

发起对账：

```json
{
  "date": "2026-08-17",
  "source_system": "order",
  "asset_code": "POINT",
  "biz_lines": [
    {
      "biz_no": "order:freeze:O1",
      "command": "Freeze",
      "asset_code": "POINT",
      "amount": "50"
    }
  ]
}
```

关闭差异：

```json
{"note": "已通过标准命令补账并复核"}
```

## 8. Connector 样板

默认地址：`http://127.0.0.1:8090`。

```http
POST /connector/order/events
POST /connector/pay/events
POST /connector/mq/events
```

订单事件：

- `created` → Freeze
- `paid` → Capture
- `cancelled` → Release

支付事件：

- `paid` → Credit
- `refund` → Credit 并关联原业务号

Connector 用于演示 Adapter Contract，不等同于生产 MQ 中间件。

## 9. 错误码

| code | HTTP | 含义 | 调用方处理 |
|---:|---:|---|---|
| 0 | 200 | 成功 | 使用响应数据 |
| 40001 | 400 | 参数错误/资产未注册 | 修正参数，不要盲重试 |
| 40301 | 403 | ACL 无权限 | 检查客户端权限配置 |
| 40401 | 404 | 账户或冻结单不存在 | 核对租户、ID 和业务号 |
| 40901 | 409 | 幂等键相同但参数不同 | 停止重试并调查业务号 |
| 42201 | 422 | 余额不足 | 引导充值或更换方式 |
| 42202 | 422 | 冻结状态不允许操作 | 查询冻结单最新状态 |
| 42203 | 422 | 兑换滑点超限 | 刷新报价并重新确认 |
| 42204 | 422 | 跨分片路径异常 | 记录业务号并联系平台 |
| 42901 | 429 | 触发业务限额 | 稍后重试或走风控流程 |
| 50000 | 500 | 内部错误 | 保持原 biz_no 安全重试 |
| 50101 | 501 | 能力未实现 | 不应重试 |

HTTP `401` 表示 Gateway 鉴权失败，检查 client ID、时间戳、签名和密钥。

## 10. 接入验收

- 相同请求重复 3 次只记账一次。
- 相同业务号修改金额后返回幂等冲突。
- 并发扣款不会产生负余额，除非资产明确允许透支。
- Freeze、Capture、Release 状态转换正确。
- 余额不足、滑点和限额错误可被业务服务识别。
- Journal 可查到完整分录。
- 请求日志可通过 Request ID 和 biz_no 追踪。
- Gateway secret 不进入浏览器、App 包、前端日志或客户端配置。

