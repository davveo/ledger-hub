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
	TenantID         string
	AssetCode        string
	Name             string
	AssetClass       string
	CurrencyCode     string
	Precision        int
	HolderTypes      []string
	FreezeSupported  bool
	OverdraftAllowed bool
	Status           AssetStatus
	Ext              string
}

type Account struct {
	AccountID  string
	TenantID   string
	HolderType HolderType
	HolderID   string
	AssetCode  string
	Available  int64
	Frozen     int64
	Version    int64
	Status     AccountStatus
}

func (a *Account) Total() int64 {
	return a.Available + a.Frozen
}

type LedgerEntry struct {
	EntryID        string
	AccountID      string
	TenantID       string
	AssetCode      string
	HolderType     HolderType
	HolderID       string
	Direction      Direction
	Amount         int64
	AvailableAfter int64
	FrozenAfter    int64
	Command        Command
	SourceSystem   string
	BizType        string
	BizNo          string
	JournalID      string
	FreezeID       string
	RelatedBizNo   string
	CreatedAt      time.Time
}

type FreezeOrder struct {
	FreezeID  string
	BizNo     string
	TenantID  string
	AccountID string
	AssetCode string
	Amount    int64
	Status    FreezeStatus
	ExpireAt  *time.Time
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
	Ext          map[string]interface{}
}

type ACLRule struct {
	SourceSystem string
	Commands     []string
	Assets       []string
}

const (
	DiffExtra          = "extra"
	DiffMissing        = "missing"
	DiffAmountMismatch = "amount_mismatch"
	DiffAssetMismatch  = "asset_mismatch"
	DiffBalanceTieOut  = "balance_tie_out"
	DiffFreezeTieOut   = "freeze_tie_out"

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
	JobID        string
	TenantID     string
	Date         string
	SourceSystem string
	AssetCode    string
	Status       string
	Summary      *ReconcileSummary
	Note         string
	CreatedAt    time.Time
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
	InAmount        string `json:"in_amount"`
	OutAmount       string `json:"out_amount"`
}

type ReconcileDiff struct {
	DiffID       string
	JobID        string
	Kind         string
	BizNo        string
	Command      Command
	AssetCode    string
	BizAmount    int64
	LedgerAmount int64
	AccountID    string
	Status       string
	Note         string
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
