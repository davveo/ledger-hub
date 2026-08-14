# Ledger Hub · 通用账本中台

> 状态：草案实现 v0.2（工程初始化）  
> 定位：面向任意交易相关系统的**虚拟资产账本中台**——提供统一开户、入账、出账、冻结、转账、多币种兑换、流水与对账能力；**不替代**订单、支付、库存、营销等业务系统。

**Ledger Hub = 交易世界的「资产记账内核」；业务系统负责交易，账本负责把钱 / 分 / 币记清楚。**

技术栈：**Go 1.20 + Gin + GORM + MySQL**。进程按层拆分，可独立部署。

- 技术方案原文：[通用账本中台技术方案.md](./通用账本中台技术方案.md)
- 整体架构图（可导出 PNG/PDF）：[docs/architecture.html](./docs/architecture.html)

---

## 1. 建设目标

| 目标 | 说明 |
|------|------|
| 通用账本 | 用 `asset_code` 承载积分、余额、金币等，内核同一套 |
| 标准对接 | 任意交易系统仅对接 Credit / Debit / Freeze / Capture / Release / Transfer / Exchange |
| 强可追溯 | 每笔变动有流水、业务单号、变更后余额 |
| 防重防超 | 幂等键 + 账户行锁 / 乐观锁 |
| 可对账 | 按日 / 按业务系统 / 按币种输出对账视图 |
| 多币种 | 一币种一资产一账户；跨币种走兑换凭证 |
| 快速接入 | 新业务注册资产 + 按接入规范调用，无需改账本核心 |

### 非目标

商品库存、订单状态机、支付渠道路由、券码生命周期、会员等级等纯状态权益——均不在本中台范围内。

---

## 2. 可拆分部署架构

当前是**模块化单体仓库**，运行时拆成三个无状态进程；领域层以包边界隔离，后续可再拆成独立服务（HTTP / gRPC）。

| 部署单元 | 入口 | 默认端口 | 职责 |
|----------|------|----------|------|
| **Gateway** | `cmd/gateway` | `:8088` | 鉴权（HMAC 签名）、限流、审计、反向代理 |
| **API** | `cmd/api` | `:8080` | 账本 HTTP 内核：资产 / 账户 / 记账命令 / 流水查询 |
| **Worker** | `cmd/worker` | `:8089` | 超时冻结自动释放、日终对账任务（Phase 2 起完善） |

进程内分层（均可被单独抽成服务）：

```text
iface/http (Gin)     → 可独立为 BFF / API 进程
application          → 用例编排，不依赖 Gin
domain               → 记账内核与仓储接口，零框架依赖
infrastructure       → GORM / MySQL / 日志，可替换存储
```

调用关系：

```text
订单 / 支付 / 活动 / 游戏
        │
        │  Adapter：业务事件 → 标准记账命令
        ▼
   Gateway :8088          （独立部署）
        │
        ▼
   ledger-api :8080       （独立部署）
        │
        ├── Asset / Account / Bookkeeping / Freeze
        └── Ledger Store (MySQL, 强一致写入)
   ledger-worker :8089    （独立部署，扫超时冻结 / 对账）
```

设计原则：

1. **内核极简**：只认记账命令，不认「下单」「签到」等业务语义（语义放 `biz_type`）。
2. **扩展配置化**：新资产 = 注册配置；新业务 = 新 `biz_type` + 调用方适配。
3. **写入强一致，读取可缓存**：余额缓存可失效重算，流水不可丢。账本不以 Redis 为唯一真相。

---

## 3. 目录结构

```text
ledger-hub/
├── cmd/
│   ├── api/                 # 账本 HTTP 服务
│   ├── gateway/             # API 网关
│   └── worker/              # 异步任务
├── configs/config.yaml
├── deployments/docker-compose.yaml
├── docs/architecture.html
├── internal/
│   ├── application/         # 应用层（用例）
│   ├── config/
│   ├── domain/              # 领域层（实体 / 仓储接口 / 错误码）
│   ├── gateway/             # 网关实现
│   ├── iface/http/          # 接口层 Gin
│   ├── infrastructure/      # GORM / 日志 / ID
│   └── worker/
├── pkg/
│   ├── client/              # Go SDK 雏形
│   └── sign/                # HMAC 签名
└── 通用账本中台技术方案.md
```

---

## 4. 快速开始

### 4.1 环境

- Go 1.20+
- Docker（用于本地 MySQL 8）

### 4.2 启动依赖与服务

```bash
make docker-up          # MySQL 8，库名 ledger_hub，root/root
make tidy
make api                # 另开终端：make gateway / make worker
```

或一次编译：

```bash
make build
./bin/ledger-api -config configs/config.yaml
./bin/ledger-gateway -config configs/config.yaml
./bin/ledger-worker -config configs/config.yaml
```

健康检查：

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8088/healthz
curl http://127.0.0.1:8089/healthz
```

开发环境可直连 `:8080`；生产流量走 Gateway `:8088`（写请求需签名）。

### 4.3 最小记账示例

```bash
# 1. 注册资产
curl -s -X POST http://127.0.0.1:8080/api/v1/ledger/assets \
  -H 'Content-Type: application/json' \
  -d '{"asset_code":"POINT","name":"积分","asset_class":"points","currency_code":"POINT","precision":0,"freeze_supported":true}'

# 2. 入账（懒开户）
curl -s -X POST http://127.0.0.1:8080/api/v1/ledger/commands/credit \
  -H 'Content-Type: application/json' \
  -d '{
    "source_system":"campaign",
    "biz_type":"signin",
    "biz_no":"campaign:signin:u123:20260814",
    "holder":{"type":"user","id":"u_123"},
    "asset_code":"POINT",
    "amount":"100"
  }'

# 3. 查询账户
curl -s 'http://127.0.0.1:8080/api/v1/ledger/accounts?holder_type=user&holder_id=u_123&asset_code=POINT'
```

同一 `biz_no` 再调一次 Credit，余额只加一次（幂等回放）。

---

## 5. 标准记账命令

所有对接系统只允许使用以下命令（金额均为**最小单位整数**，JSON 用字符串避免精度丢失）。

| 命令 | 含义 | 路由 | 阶段 |
|------|------|------|------|
| `Credit` | 入账 available↑ | `POST /api/v1/ledger/commands/credit` | Phase 1 |
| `Debit` | 出账 available↓ | `POST /api/v1/ledger/commands/debit` | Phase 1 |
| `Freeze` | 预占 available↓ frozen↑ | `POST /api/v1/ledger/commands/freeze` | Phase 2（已实现内核） |
| `Capture` | 确认预占 frozen↓ | `POST /api/v1/ledger/commands/capture` | Phase 2（已实现内核） |
| `Release` | 释放预占 frozen↓ available↑ | `POST /api/v1/ledger/commands/release` | Phase 2（已实现内核） |
| `Transfer` | 同币种转账 | `POST /api/v1/ledger/commands/transfer` | Phase 2（已实现内核） |
| `Exchange` | 跨币种兑换（同一 Journal） | `POST /api/v1/ledger/commands/exchange` | Phase 3 |

聚合入口：`POST /api/v1/ledger/commands`，body 带 `"command":"Credit"`。

余额恒等式：`total = available + frozen`。

**Transfer 禁止跨币种**；跨币种必须用 `Exchange`，以便保留汇率、双边金额并对账。

---

## 6. HTTP API

统一前缀：`/api/v1/ledger`  
鉴权：应用 `client_id` + HMAC 签名（经 Gateway）；仅服务端调用。

### 资产与账户

```http
POST /api/v1/ledger/assets
GET  /api/v1/ledger/assets/{asset_code}
POST /api/v1/ledger/accounts/open
GET  /api/v1/ledger/accounts?holder_type=user&holder_id=u_1&asset_code=POINT
GET  /api/v1/ledger/accounts/{account_id}
```

### 流水 / 冻结 / 对账

```http
GET  /api/v1/ledger/entries?biz_no=order:O001
GET  /api/v1/ledger/entries?holder_id=u_1&asset_code=POINT
GET  /api/v1/ledger/freezes/{freeze_id}
GET  /api/v1/ledger/freezes?biz_no=order:O001
POST /api/v1/ledger/reconcile/jobs
GET  /api/v1/ledger/reconcile/jobs/{id}
GET  /api/v1/ledger/reconcile/reports/{date}?source_system=order
```

成功响应：`{"code":0,"data":{...}}`。幂等重放时 `idempotent_replay=true`，返回首次结果。

### 错误码

| code | 含义 |
|------|------|
| 0 | 成功 |
| 40001 | 参数错误 / 资产未注册 |
| 40401 | 账户 / 冻结单不存在 |
| 40901 | 幂等冲突但命令参数不一致 |
| 42201 | 余额不足 |
| 42202 | 冻结单状态不允许 Capture/Release |
| 42901 | 触发限额 |
| 50101 | 能力尚未交付（如 Exchange） |

---

## 7. 接入规范（Adapter Contract）

Ledger Hub **不订阅**各业务私有事件格式；由对接方或独立 Connector 翻译成标准命令。

`biz_no` 建议：`{source_system}:{biz_type_short}:{natural_id}`

```text
order:freeze:O20260814001
pay:credit:P988331
campaign:signin:u123:20260814
```

- 同一业务动作重试，`biz_no` 不变
- 逆向业务使用新号，并用 `related_biz_no` 指向原单
- 禁止随机 UUID 当唯一业务键（除非业务侧已持久化）
- 取消必须 `Release` 对应 freeze，禁止补偿式瞎加
- 记账 API 只能业务后端调

SDK 雏形：`pkg/client`（超时、签名、命令封装）。

---

## 8. 数据模型（落库）

GORM AutoMigrate 创建以下表（金额一律 BIGINT 最小单位）：

| 表 | 说明 |
|----|------|
| `ledger_asset` | 资产定义（精度、可否冻结、可否透支） |
| `ledger_account` | 持有者 × 资产的账户，`unique(tenant, holder, asset)` |
| `ledger_entry` | 单笔流水（真相来源），含变更后余额快照 |
| `ledger_freeze` | 冻结单：frozen / captured / released |
| `ledger_idempotency` | tenant + source + biz_no + command → 首次响应 |
| `ledger_journal` | 复式凭证（兑换 / 转账预留） |
| `ledger_fx_rate` | 汇率快照（Phase 3） |

样板资产：`POINT`、`BALANCE_CNY`、`BALANCE_USD`、`BALANCE_HKD`、`COIN`、`GROWTH`。

---

## 9. 分阶段交付

| 阶段 | 范围 | 本仓库现状 |
|------|------|------------|
| **Phase 1 MVP** | 资产注册、懒开户、Credit/Debit、流水、幂等、鉴权 | 已落地 |
| **Phase 2** | Freeze 三态、Transfer、权限矩阵、日终对账、样板接入 | 冻结/转账内核已实现；对账报表占位 |
| **Phase 3** | Exchange、汇率快照、Journal、过期引擎、分库、运营台 | 表结构预留，命令返回 50101 |

---

## 10. 配置

见 `configs/config.yaml`。环境变量前缀 `LEDGER_`，层级用 `_` 替换，例如 `LEDGER_MYSQL_DSN`。

Gateway 写请求头：

- `X-Client-Id`
- `X-Timestamp`（unix 秒）
- `X-Signature` = `HMAC-SHA256(secret, client_id + timestamp + body)` hex

---

## 11. 安全与对账要点

- 真资金 / 可兑换余额：**不要**只做 Redis 加减
- 补账或冲正一律走命令，禁止直接改余额字段
- 多币种对账按 `asset_code` 分文件，禁止折本币混加作为唯一结果
- 兑换冲正必须整笔 Journal 回滚

完整对账分层、兑换拆解、风险对策见技术方案第 10–15 章。
