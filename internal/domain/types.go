package domain

import "time"

type Command string

const (
	CmdCredit   Command = "Credit"
	CmdDebit    Command = "Debit"
	CmdFreeze   Command = "Freeze"
	CmdCapture  Command = "Capture"
	CmdRelease  Command = "Release"
	CmdTransfer Command = "Transfer"
	CmdExchange Command = "Exchange"
	CmdReverse  Command = "Reverse"
)

type Direction string

const (
	DirIN  Direction = "IN"
	DirOUT Direction = "OUT"
)

type HolderType string

const (
	HolderUser          HolderType = "user"
	HolderMerchant      HolderType = "merchant"
	HolderSystemSubject HolderType = "system_subject"
)

type AccountStatus string

const (
	AccountActive   AccountStatus = "active"
	AccountDisabled AccountStatus = "disabled"
)

type AssetStatus string

const (
	AssetActive   AssetStatus = "active"
	AssetDisabled AssetStatus = "disabled"
)

type FreezeStatus string

const (
	FreezeFrozen   FreezeStatus = "frozen"
	FreezeCaptured FreezeStatus = "captured"
	FreezeReleased FreezeStatus = "released"
)

type Holder struct {
	Type HolderType `json:"type"`
	ID   string     `json:"id"`
}

type Asset struct {
	TenantID         string      `json:"tenant_id"`
	AssetCode        string      `json:"asset_code"`
	Name             string      `json:"name"`
	AssetClass       string      `json:"asset_class"`
	CurrencyCode     string      `json:"currency_code"`
	Precision        int         `json:"precision"`
	HolderTypes      []string    `json:"holder_types,omitempty"`
	FreezeSupported  bool        `json:"freeze_supported"`
	OverdraftAllowed bool        `json:"overdraft_allowed"`
	Status           AssetStatus `json:"status"`
	Ext              string      `json:"ext,omitempty"`
}

type Account struct {
	AccountID  string        `json:"account_id"`
	TenantID   string        `json:"tenant_id"`
	HolderType HolderType    `json:"holder_type"`
	HolderID   string        `json:"holder_id"`
	AssetCode  string        `json:"asset_code"`
	Available  int64         `json:"available"`
	Frozen     int64         `json:"frozen"`
	Version    int64         `json:"version"`
	Status     AccountStatus `json:"status"`
}

func (a *Account) Total() int64 {
	return a.Available + a.Frozen
}

type LedgerEntry struct {
	EntryID        string     `json:"entry_id"`
	AccountID      string     `json:"account_id"`
	TenantID       string     `json:"tenant_id"`
	AssetCode      string     `json:"asset_code"`
	HolderType     HolderType `json:"holder_type"`
	HolderID       string     `json:"holder_id"`
	Direction      Direction  `json:"direction"`
	Amount         int64      `json:"amount"`
	AvailableAfter int64      `json:"available_after"`
	FrozenAfter    int64      `json:"frozen_after"`
	Command        Command    `json:"command"`
	SourceSystem   string     `json:"source_system"`
	BizType        string     `json:"biz_type,omitempty"`
	BizNo          string     `json:"biz_no"`
	JournalID      string     `json:"journal_id,omitempty"`
	FreezeID       string     `json:"freeze_id,omitempty"`
	RelatedBizNo   string     `json:"related_biz_no,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type FreezeOrder struct {
	FreezeID  string       `json:"freeze_id"`
	BizNo     string       `json:"biz_no"`
	TenantID  string       `json:"tenant_id"`
	AccountID string       `json:"account_id"`
	AssetCode string       `json:"asset_code"`
	Amount    int64        `json:"amount"`
	Status    FreezeStatus `json:"status"`
	ExpireAt  *time.Time   `json:"expire_at,omitempty"`
}

type IdempotencyRecord struct {
	TenantID     string
	SourceSystem string
	BizNo        string
	Command      Command
	RequestHash  string
	ResponseJSON string
}

type CommandRequest struct {
	Command      Command
	RequestID    string
	TenantID     string
	SourceSystem string
	BizType      string
	BizNo        string
	Holder       Holder
	AssetCode    string
	Amount       int64
	FreezeID     string
	RelatedBizNo string
	ToHolder     *Holder
	ToAssetCode  string
	ToAmount     int64
	ExpireAt     *time.Time
	FeeAsset     string
	FeeAmount    int64
	Fx           *FxQuote
	Tolerance    int64
	Ext          map[string]interface{}
}

type FxQuote struct {
	RateID     string
	BaseAsset  string
	QuoteAsset string
	Rate       string
	RateSource string
	QuotedAt   time.Time
}

type FxRate struct {
	RateID     string     `json:"rate_id"`
	TenantID   string     `json:"tenant_id"`
	BaseAsset  string     `json:"base_asset"`
	QuoteAsset string     `json:"quote_asset"`
	Rate       string     `json:"rate"`
	RateSource string     `json:"rate_source"`
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidTo    *time.Time `json:"valid_to,omitempty"`
	QuotedAt   time.Time  `json:"quoted_at"`
	CreatedBy  string     `json:"created_by,omitempty"`
}

type Journal struct {
	JournalID    string `json:"journal_id"`
	TenantID     string `json:"tenant_id"`
	BizNo        string `json:"biz_no"`
	JournalType  string `json:"journal_type"`
	Status       string `json:"status"`
	EntriesCount int    `json:"entries_count"`
	FxRateID     string `json:"fx_rate_id,omitempty"`
	Ext          string `json:"ext,omitempty"`
}

type ExchangeLeg struct {
	ExchangeID string
	JournalID  string
	BizNo      string
	TenantID   string
	HolderType HolderType
	HolderID   string
	FromAsset  string
	FromAmount int64
	ToAsset    string
	ToAmount   int64
	FeeAsset   string
	FeeAmount  int64
	RateID     string
	Rate       string
	Status     string
}

type Tenant struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

type LimitRule struct {
	TenantID     string  `json:"tenant_id,omitempty"`
	SourceSystem string  `json:"source_system"`
	AssetCode    string  `json:"asset_code"`
	Command      Command `json:"command"`
	MaxAmount    int64   `json:"max_amount"`
	DailyAmount  int64   `json:"daily_amount"`
	DailyCount   int     `json:"daily_count"`
}

type LimitAlert struct {
	AlertID      string    `json:"alert_id,omitempty"`
	At           time.Time `json:"at"`
	TenantID     string    `json:"tenant_id"`
	SourceSystem string    `json:"source_system"`
	HolderID     string    `json:"holder_id"`
	AssetCode    string    `json:"asset_code"`
	Command      Command   `json:"command"`
	Reason       string    `json:"reason"`
}

type OpsAudit struct {
	AuditID   string    `json:"audit_id"`
	Operator  string    `json:"operator"`
	Action    string    `json:"action"`
	TenantID  string    `json:"tenant_id,omitempty"`
	Target    string    `json:"target,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type OpsRun struct {
	RunID      string     `json:"run_id"`
	Name       string     `json:"name"`
	TenantID   string     `json:"tenant_id,omitempty"`
	Status     string     `json:"status"`
	Detail     string     `json:"detail,omitempty"`
	Count      int        `json:"count"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type ExpirePreview struct {
	TenantID   string `json:"tenant_id"`
	HolderType string `json:"holder_type"`
	HolderID   string `json:"holder_id"`
	AccountID  string `json:"account_id"`
	AssetCode  string `json:"asset_code"`
	Amount     int64  `json:"amount"`
	Policy     string `json:"policy"`
}

type FxFeedPair struct {
	TenantID   string `json:"tenant_id,omitempty"`
	BaseAsset  string `json:"base_asset"`
	QuoteAsset string `json:"quote_asset"`
	Rate       string `json:"rate"`
}

type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func (p Page) Clamp(def, max int) Page {
	if def <= 0 {
		def = 50
	}
	if max <= 0 {
		max = 200
	}
	if p.Limit <= 0 {
		p.Limit = def
	}
	if p.Limit > max {
		p.Limit = max
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

const (
	JournalTransfer = "transfer"
	JournalExchange = "exchange"
	JournalReverse  = "reverse"
	JournalPosting  = "posting"

	SystemFxFee              = "fx_fee_income"
	SystemFxClearing         = "fx_clearing"
	SystemPointSink          = "point_sink"
	SystemPointIssuance      = "point_issuance"
	SystemPendingSettlement  = "pending_settlement"
)

type ACLRule struct {
	TenantID     string   `json:"tenant_id,omitempty"`
	SourceSystem string   `json:"source_system"`
	Commands     []string `json:"commands"`
	Assets       []string `json:"assets"`
}

type GatewayAudit struct {
	AuditID    string    `json:"audit_id"`
	ClientID   string    `json:"client_id"`
	TenantID   string    `json:"tenant_id,omitempty"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

const (
	SagaPending      = "pending"
	SagaOutDone      = "out_done"
	SagaInDone       = "in_done"
	SagaCompensating = "compensating"
	SagaCompleted    = "completed"
	SagaFailed       = "failed"
)

type TransferSaga struct {
	SagaID       string     `json:"saga_id"`
	TenantID     string     `json:"tenant_id"`
	SourceSystem string     `json:"source_system"`
	BizNo        string     `json:"biz_no"`
	FromType     HolderType `json:"from_type"`
	FromID       string     `json:"from_id"`
	ToType       HolderType `json:"to_type"`
	ToID         string     `json:"to_id"`
	AssetCode    string     `json:"asset_code"`
	Amount       int64      `json:"amount"`
	Status       string     `json:"status"`
	OutBizNo     string     `json:"out_biz_no,omitempty"`
	InBizNo      string     `json:"in_biz_no,omitempty"`
	RollbackNo   string     `json:"rollback_biz_no,omitempty"`
	ResultJSON   string     `json:"-"`
	LastError    string     `json:"last_error,omitempty"`
	RetryCount   int        `json:"retry_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

const (
	DiffExtra          = "extra"
	DiffMissing        = "missing"
	DiffAmountMismatch = "amount_mismatch"
	DiffAssetMismatch  = "asset_mismatch"
	DiffBalanceTieOut  = "balance_tie_out"
	DiffFreezeTieOut   = "freeze_tie_out"
	DiffFxIncomplete   = "fx_incomplete"
	DiffChannelMismatch   = "channel_mismatch"
	DiffCrossShardInFlight = "cross_shard_inflight"

	ReconJobRunning  = "running"
	ReconJobDone     = "done"
	ReconJobFailed   = "failed"
	DiffStatusOpen     = "open"
	DiffStatusResolved = "resolved"
)

type BizLine struct {
	BizNo     string
	Command   Command
	AssetCode string
	Amount    int64
}

type ReconcileJob struct {
	JobID        string            `json:"job_id"`
	TenantID     string            `json:"tenant_id"`
	Date         string            `json:"date"`
	SourceSystem string            `json:"source_system,omitempty"`
	AssetCode    string            `json:"asset_code,omitempty"`
	Status       string            `json:"status"`
	Summary      *ReconcileSummary `json:"summary,omitempty"`
	Note         string            `json:"note,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

type ReconcileSummary struct {
	LedgerCount     int    `json:"ledger_count"`
	BizCount        int    `json:"biz_count"`
	Matched         int    `json:"matched"`
	Extra           int    `json:"extra"`
	Missing         int    `json:"missing"`
	AmountMismatch  int    `json:"amount_mismatch"`
	AssetMismatch   int    `json:"asset_mismatch"`
	BalanceTieOut   int    `json:"balance_tie_out"`
	FreezeTieOut    int    `json:"freeze_tie_out"`
	FxIncomplete    int    `json:"fx_incomplete"`
	ChannelMismatch int    `json:"channel_mismatch"`
	InAmount        string `json:"in_amount"`
	OutAmount       string `json:"out_amount"`
}

type ReconcileDiff struct {
	DiffID       string  `json:"diff_id"`
	JobID        string  `json:"job_id"`
	Kind         string  `json:"kind"`
	BizNo        string  `json:"biz_no,omitempty"`
	Command      Command `json:"command,omitempty"`
	AssetCode    string  `json:"asset_code,omitempty"`
	BizAmount    int64   `json:"biz_amount"`
	LedgerAmount int64   `json:"ledger_amount"`
	AccountID    string  `json:"account_id,omitempty"`
	Status       string  `json:"status"`
	Note         string  `json:"note,omitempty"`
	ResolvedBy   string  `json:"resolved_by,omitempty"`
}

type CommandResult struct {
	Accepted         bool     `json:"accepted"`
	IdempotentReplay bool     `json:"idempotent_replay"`
	FreezeID         string   `json:"freeze_id,omitempty"`
	JournalID        string   `json:"journal_id,omitempty"`
	EntryIDs         []string `json:"entry_ids"`
	Account          *Balance `json:"account,omitempty"`
	ToAccount        *Balance `json:"to_account,omitempty"`
}

type Balance struct {
	AccountID string `json:"account_id"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}
