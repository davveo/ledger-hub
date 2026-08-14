# Ledger Hub 后续能力清单

对照 [技术方案](./通用账本中台技术方案.md) 与当前代码（Phase 1–3 已落地）。下列项是**还能加、且值得加**的能力，不是已交付功能的复述。

状态约定：`- [ ]` 未做 · 优先级：P0 内核完整性 / P1 可运营可接入 / P2 规模与工程化。

---

## 已有能力（不再列入）

- Credit / Debit / Freeze / Capture / Release / Transfer / Exchange
- 幂等、ACL、日终 L1–L4 对账、超时释放冻结
- 汇率快照、Journal、系统科目 `fx_clearing` / `fx_fee_income`
- 租户、限额、按 `holder_id` 分片、过期引擎雏形、运营台雏形
- Gateway HMAC、Connector 订单/支付样板、Compose 一键启动

---

## P0 · 记账内核补全

- [x] **冲正命令 `Reverse`**
  - 常量 `JournalReverse` 已有，无 `CmdReverse`、无 HTTP、无整笔回滚。
  - 按技术方案 11.5：新 `biz_no` + `related_biz_no` 指向原单；保留原流水。
  - Exchange 必须整笔 Journal 回滚（from / to / fee / clearing），禁止只改一侧。
- [x] **Credit / Debit / Freeze 也落 Journal**
  - 目前仅 Transfer / Exchange 写 `ledger_journal`；单边入账无法按凭证对账。
  - 建议 `journal_type=posting`，可选挂 `system:point_issuance` / `pending_settlement`。
- [x] **过期入系统科目**
  - 现实现只 `Debit` 用户，未贷记 `point_sink`，过期后资产总量对不上。
  - 改为 Transfer 到 `system_subject:point_sink`，并进日终 POINT 对账。
- [x] **账户 / 资产状态真正生效**
  - `AccountDisabled`、`AssetDisabled`、`holder_types` 已建模，开户与记账未校验。
  - 停用账户禁止变动；资产未允许的 holder_type 拒绝懒开户。
- [x] **部分 Capture**
  - 冻结单只能整单确认。订单部分履约需要 `capture_amount <= freeze.amount`，余量继续冻或自动 Release。
- [x] **跨分片 Transfer 的标准路径**
  - 现跨片直接 `42204`。技术方案要求经系统科目轧差（两段记账 + 同一业务 `related_biz_no`），而不是永远禁止。

---

## P0 · 对账与汇率

- [x] **L4 按兑换腿核对金额 / 费率 / `rate_id`**
  - 当前只检查「有用户 OUT + 用户 IN + 至少两种 asset」，不核 from/to/fee 与快照。
  - 应用 `ledger_exchange_leg` + `ledger_fx_rate` 做兑换等式校验。
- [x] **对账产物落盘**
  - CSV 只塞在 JSON `files` 里。补：`recon_*` / `diff_*` / `balance_tie_out_*` / `fx_journal_*` 可下载或写对象存储。
- [x] **Worker 对账覆盖全部租户 / 资产**
  - `runDailyReconcile` 只用 `default_tenant` 且 `source_system/asset` 为空，多租户会漏跑。
- [x] **L5 支付渠道对账（可选）**
  - 支付成功金额 ↔ `Credit(BALANCE_*)`，按币种分文件；Connector 侧导出渠道清单。
- [x] **汇率 Feed**
  - `rate_source=feed` 已预留。Worker 定时拉牌价写入 `ledger_fx_rate`（账本仍不负责行情对错）。

---

## P1 · 查询与接入体验

- [ ] **批量查账户（多币种钱包）**
  - `GET /accounts?holder_id=` 一次返回该 holder 下全部 `asset_code`，避免 N 次请求。
- [ ] **流水 / 冻结分页与按 holder 列冻结单**
  - 流水硬编码 `Limit(200)`；无 `GET /freezes?holder_id=`。
- [ ] **OpenAPI + 类型化 SDK**
  - `pkg/client` 目前是 `map[string]interface{}`。补：生成 spec、强类型 Command、查询账户/流水/Journal。
- [ ] **样板资产种子**
  - README 写了 POINT / BALANCE_CNY / USD / HKD / COIN / GROWTH，启动不自动注册。
- [ ] **Connector：退款 / MQ**
  - 仅 HTTP 事件；缺 `refund → Credit(related_biz_no)`，也未订阅订单/支付 MQ。
- [ ] **Gateway 鉴权加固**
  - GET 完全免签（含对账报表）；无时间窗防重放；`audit()` 空实现；限流是全局秒桶不是按 client。
  - `/console` 未走网关。补：timestamp 偏差、审计落库、按 client RPS、运营台鉴权。

---

## P1 · 运营与风控

- [ ] **运营台做成可用的控制台**
  - 现页只能登记汇率、触发对账、JSON 概览。缺：资产 CRUD、租户启停、账户/流水检索、差异工单关闭、限额规则展示与告警。
- [ ] **ACL / 限额热加载与租户隔离**
  - 规则只在 yaml，改配置要重启；限额不按租户；超限无告警只有 `42901`。
- [ ] **账户停用 / 资产下线 API**
  - 只有注册与查询，没有 `disabled` 操作入口。
- [ ] **幂等记录清理**
  - `ledger_idempotency` 只增不删，需 TTL 或归档策略（保留期与对账窗口对齐）。

---

## P2 · 可靠运行与规模

- [ ] **健康检查探活依赖**
  - `/healthz` 不探 MySQL；编排会把「进程活着」当成「可记账」。
- [ ] **可观测性**
  - 无 Prometheus 指标（命令耗时、幂等命中、不足额、滑点拒绝、对账差异数）、无 tracing、无结构化审计表。
- [ ] **余额只读缓存**
  - 技术方案：写入删缓存，不以 Redis 为唯一真相。热点查询可加，必须失效策略。
- [ ] **流水表按时间分区**
  - 日终 `ListByRange` 与过期 FIFO 会随流水膨胀。按月分区 + 归档。
- [ ] **分片运维**
  - `GetByID` / 对账扫全分片；无扩片、迁移、一致性校验工具。
- [ ] **热点账户**
  - 大 V / 系统科目行锁排队。后期可分段子账户或系统科目本地副本（方案第 12 章）。
- [ ] **CI 与集成测试**
  - 无 GitHub Actions；测试全是内存 fake。补：compose 起库后跑命令/对账/兑换用例。
- [ ] **密钥与配置**
  - Gateway secret 明文 yaml。补：环境变量 / secret 挂载；生产关闭 AutoMigrate。
- [ ] **gRPC / mTLS**（方案 5 节可选）
  - 内核稳定后再加内网 RPC；浏览器不持有 secret。

---

## 建议实施顺序

1. Reverse + 过期入 `point_sink` + 状态校验（账能冲、过期能平）
2. L4 金额核验 + 对账文件 + Worker 扫全租户
3. 批量账户 / 分页 / 类型化 SDK / OpenAPI
4. Gateway 时间窗与审计、运营台可用化
5. 指标、健康检查、CI 集成测试
6. 缓存、分区、跨片轧差、汇率 Feed

明确不做（保持非目标）：库存、订单状态机、支付渠道路由、券码生命周期、会员等级。
