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
| `X-Sign-Version` | `v2`（SDK 默认）。缺省且无 nonce 时按 V1 |
| `X-Nonce` | V2 必填，时间窗内不可重复 |
| `X-Key-Version` | 密钥版本，缺省 `1` |
| `X-Tenant-Id` | 当前租户；未传时使用 `gateway.default_tenant` |
| `X-Request-Id` | 推荐传入，用于链路排查；Gateway 会转发给上游 |
| `traceparent` | W3C 追踪；缺省时服务端生成并继续传播 |
| `Lang` / `X-Lang` / `Accept-Language` | 错误 `message` 语言。`en` / `en-US` 英文，其余默认中文。`code` 与 `error` 不随语言变化 |
| `Content-Type` | 写请求使用 `application/json` |

运营控制台只用 `X-Console-Token`，**禁止** `?console_token=`。

**V2**（推荐，`pkg/client` 默认）：

```text
hex(HMAC-SHA256(secret, CanonicalV2))

CanonicalV2 =
v2\n
client_id\n
METHOD\n
path\n
canonical_query\n
tenant_id\n
timestamp\n
nonce\n
sha256_hex(body)
```

**V1**（迁移兼容）：

```text
hex(HMAC-SHA256(secret, client_id + timestamp + raw_body))
```

请求时间默认允许与服务端相差 300 秒。写请求体中的 `source_system` 必须与 `X-Client-Id` 一致。client 配置了 `tenants` 时不可访问列表外租户。nonce 重复返回 `40102`。

Go 项目可使用 `pkg/client` 自动完成 V2 签名和请求。

## 2. 通用规则

### 2.1 金额

- 所有金额均为资产最小单位整数。
- JSON 使用字符串，避免精度丢失。
- 精度为 2 的资产中，`"1049900"` 表示 `10499.00`。
- `amount` / `from.amount` / `to.amount` / `fee.amount` / `tolerance` 解析失败返回 `40010`–`40014`（`error` 如 `AMOUNT_NOT_INTEGER`），不会变成 0。
- 查询 `from`/`to` 须为 RFC3339，且 `from < to`，最大跨度 366 天。

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

`POST /reconcile/jobs` 与 `POST .../jobs/{id}/rerun` 成功时 HTTP **202**（`code` 仍为 0）。金额字段一律最小单位整数字符串。分页查询 `limit` 默认 50、最大 200，响应含 `items` 与总数（若该接口提供）。

Go SDK：`pkg/client` 的 `Credit`/`Debit`/… 接受 `Command`；推荐 `Exec()`。`WithTimeout`、`WithRequestID`；对 HTTP 502/503 有限重试。

### 2.4 探针、指标与密钥

进程根路径（无 `/api/v1/ledger` 前缀）：

```http
GET /livez      # 存活，200
GET /readyz     # API/Worker ping MySQL 主库与各分片，失败 503；Gateway 短超时探测上游并缓存约 5s
GET /healthz    # /livez 别名
GET /metrics    # Prometheus（API / Gateway / Worker）
```

生产密钥在 yaml 之后用环境变量覆盖，日志不打印密钥值：

- `LEDGER_GATEWAY_CONSOLE_TOKEN`
- `LEDGER_GATEWAY_CLIENT_<CLIENTID>_SECRET`（CLIENTID 大写，连字符改下划线）

`app.env` 为 `prod` / `production` 时：空 token、`dev-console-token`、空或 `dev-` 前缀 client secret 会拒绝启动。Schema 用 `make migrate` / `bin/ledger-migrate`；生产请设 `mysql.auto_migrate: false`。演进顺序：**expand**（新列可空）→ **migrate**（回填）→ **contract**（加约束）。

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

跨分片时经 `pending_settlement` 持久化 Saga（`pending` → `out_done` → `in_done` → `completed`），Worker 续跑，失败则补偿。

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

运营（经 Gateway 须带 `X-Console-Token`）：

```http
GET  /api/v1/ledger/ops/jobs
POST /api/v1/ledger/ops/jobs/{name}
GET  /api/v1/ledger/ops/sagas?status=
POST /api/v1/ledger/ops/sagas/{id}/retry
POST /api/v1/ledger/ops/sagas/{id}/compensate
POST /api/v1/ledger/ops/reload
GET  /api/v1/ledger/ops/config/revisions?limit=
GET  /api/v1/ledger/ops/audits?limit=
GET  /api/v1/ledger/ops/actions?limit=
GET  /api/v1/ledger/ops/alerts?limit=
GET  /api/v1/ledger/openapi.yaml
```

## 7. 对账

```http
POST /api/v1/ledger/reconcile/jobs          # 202 入队
GET  /api/v1/ledger/reconcile/jobs?limit=20
GET  /api/v1/ledger/reconcile/jobs/{job_id}
POST /api/v1/ledger/reconcile/jobs/{job_id}/rerun
GET  /api/v1/ledger/reconcile/reports/{date}?source_system=order&asset_code=POINT
GET  /api/v1/ledger/reconcile/files
GET  /api/v1/ledger/reconcile/files/{name}
GET  /api/v1/ledger/reconcile/diffs
POST /api/v1/ledger/reconcile/diffs/{diff_id}/resolve
POST /api/v1/ledger/reconcile/diffs/{diff_id}/assign
GET  /api/v1/ledger/reconcile/diffs/{diff_id}/events
```

成功时 HTTP **202**，`data` 含 `job_id`、`status`（通常 `queued`）、`version`。Worker 领取后 `GET .../jobs/{id}` 可见 `phase`/`summary`。

同一 `(tenant_id, date, source_system, asset_code, job_type, version)` 已有 queued/running/done 时复用，不建重复任务。`"force_new_version": true` 或 `POST .../rerun` 升版本。

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
GET  /connector/mq/inbox?status=
POST /connector/mq/inbox/{message_id}/replay
```

订单事件：

- `created` → Freeze
- `paid` → Capture
- `cancelled` → Release

支付事件：

- `paid` → Credit
- `refund` → Credit 并关联原业务号

Connector 用于 Adapter Contract。事件带 `schema_version`（默认 1）。HTTP/文件先入 `ledger_mq_inbox` 再处理；失败指数退避，超过次数 `dead`，可用 replay。`connector.kafka.brokers` 非空时启动 Kafka 消费者。

## 9. 错误码

失败响应固定为：

```json
{"code":40010,"error":"AMOUNT_NOT_INTEGER","message":"金额必须为最小单位整数"}
```

- `code`：数字错误码，与语言无关。HTTP 状态 = `code / 100`（如 40010→400，40102→401，50300→503）。
- `error`：稳定英文键，**调用方应按此分支**。
- `message`：按请求头 `Lang`、`X-Lang` 或 `Accept-Language` 返回中文或英文；缺省中文。

| code | error | HTTP | 含义 |
|---:|---|---:|---|
| 0 | | 200 | 成功 |
| 40001 | INVALID_PARAM | 400 | 通用参数错误 |
| 40002 | JSON_INVALID | 400 | JSON 或必填字段 |
| 40010 | AMOUNT_NOT_INTEGER | 400 | 金额不是最小单位整数 |
| 40011–40014 | AMOUNT_FROM/TO/FEE/TOLERANCE_INVALID | 400 | 对应金额字段非法 |
| 40020–40025 | TIME_* / DATE_NOT_ISO | 400 | 时间或日期非法 |
| 40031 | ASSET_DISABLED | 400 | 资产未启用 |
| 40032 | ASSET_FREEZE_UNSUPPORTED | 400 | 资产不支持冻结 |
| 40033 | HOLDER_TYPE_NOT_ALLOWED | 400 | 持有者类型不允许 |
| 40034 | ACCOUNT_DISABLED | 400 | 账户已停用 |
| 40036 | CAPTURE_EXCEEDS_FREEZE | 400 | Capture 超过剩余冻结 |
| 40037 | TRANSFER_NEEDS_TO_HOLDER | 400 | Transfer 缺 to_holder |
| 40038 | TRANSFER_CROSS_CURRENCY | 400 | Transfer 禁止跨币种 |
| 40039–40043 | EXCHANGE_* / FX_* | 400 | 兑换或汇率 |
| 40044–40045 | REVERSE_* | 400 | 冲正参数 |
| 40046 | UNKNOWN_JOB | 400 | 未知作业 |
| 40047–40048 | SAGA_* | 400/500 | Saga 状态 |
| 40049 | UNKNOWN_TENANT | 400 | 未知租户 |
| 40100 | UNAUTHORIZED | 401 | 通用鉴权失败 |
| 40102 | REPLAY | 401 | nonce 重放 |
| 40110 | CONSOLE_TOKEN_IN_QUERY | 401 | console_token 出现在 query |
| 40111 | UNKNOWN_CLIENT | 401 | 未知 client_id |
| 40112 | MISSING_SIGNATURE | 401 | 缺少签名 |
| 40113–40114 | TIMESTAMP_* | 401 | 时间戳无效或超窗 |
| 40115 | UNKNOWN_KEY_VERSION | 401 | 未知密钥版本 |
| 40116 | SIGN_VERSION_UNSUPPORTED | 401 | 不支持的签名版本 |
| 40117 | NONCE_REQUIRED | 401 | V2 缺 nonce |
| 40118 | SIGNATURE_MISMATCH | 401 | 签名不匹配 |
| 40301 | FORBIDDEN | 403 | ACL 无权 |
| 40310 | SOURCE_SYSTEM_MISMATCH | 403 | source_system ≠ client_id |
| 40311 | TENANT_HEADER_MISMATCH | 403 | Header 与 body 租户不一致 |
| 40312 | TENANT_NOT_ALLOWED | 403 | client 无权访问该租户 |
| 40313 | TENANT_DISABLED | 403 | 租户已停用 |
| 40401 | NOT_FOUND | 404 | 不存在；跨租户同样 404 |
| 40901 | IDEMPOTENCY_CONFLICT | 409 | 幂等键相同参数不同 |
| 42201 | INSUFFICIENT_BALANCE | 422 | 余额不足 |
| 42202 | FREEZE_STATE_INVALID | 422 | 冻结单非 frozen |
| 42203 | SLIPPAGE_EXCEEDED | 422 | 兑换滑点超限 |
| 42204 | CROSS_SHARD_FORBIDDEN | 422 | 跨分片路径异常 |
| 42901 | RATE_LIMITED | 429 | 限额或 RPS |
| 50000 | INTERNAL_ERROR | 500 | 内部错误 |
| 50010 | OPTIMISTIC_LOCK | 500 | 账户乐观锁冲突，可重试 |
| 50011–50013 | SAGA_FAILED / COMPENSATED / FREEZE_LEDGER_MISMATCH | 500 | 一致性异常 |
| 50101 | NOT_IMPLEMENTED | 501 | 能力未装配 |
| 50200 | UPSTREAM_ERROR | 502 | Connector 调账本失败 |
| 50300 | NOT_READY | 503 | /readyz 未就绪 |

HTTP `401` 表示 Gateway 鉴权失败，检查 client ID、时间戳、签名、nonce、密钥版本和租户授权。

## 10. 接入验收

- 相同请求重复 3 次只记账一次。
- 相同业务号修改金额后返回幂等冲突。
- 并发扣款不会产生负余额，除非资产明确允许透支。
- Freeze、Capture、Release 状态转换正确。
- 余额不足、滑点和限额错误可被业务服务识别。
- Journal 可查到完整分录。
- 请求日志可通过 Request ID 和 biz_no 追踪。
- Gateway secret 不进入浏览器、App 包、前端日志或客户端配置。

