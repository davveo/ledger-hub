package persistence

import (
	"encoding/json"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

type LedgerAsset struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	TenantID         string    `gorm:"size:64;uniqueIndex:uk_tenant_asset;not null"`
	AssetCode        string    `gorm:"size:64;uniqueIndex:uk_tenant_asset;not null"`
	Name             string    `gorm:"size:128;not null"`
	AssetClass       string    `gorm:"size:32;not null"`
	CurrencyCode     string    `gorm:"size:16;not null"`
	Precision        int       `gorm:"not null"`
	HolderTypes      string    `gorm:"type:text"`
	FreezeSupported  bool      `gorm:"not null"`
	OverdraftAllowed bool      `gorm:"not null"`
	Status           string    `gorm:"size:16;not null"`
	Ext              string    `gorm:"type:text"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (LedgerAsset) TableName() string { return "ledger_asset" }

func (m *LedgerAsset) toDomain() *domain.Asset {
	var holders []string
	_ = json.Unmarshal([]byte(m.HolderTypes), &holders)
	return &domain.Asset{
		TenantID:         m.TenantID,
		AssetCode:        m.AssetCode,
		Name:             m.Name,
		AssetClass:       m.AssetClass,
		CurrencyCode:     m.CurrencyCode,
		Precision:        m.Precision,
		HolderTypes:      holders,
		FreezeSupported:  m.FreezeSupported,
		OverdraftAllowed: m.OverdraftAllowed,
		Status:           domain.AssetStatus(m.Status),
		Ext:              m.Ext,
	}
}

func assetFromDomain(a *domain.Asset) *LedgerAsset {
	holders, _ := json.Marshal(a.HolderTypes)
	return &LedgerAsset{
		TenantID:         a.TenantID,
		AssetCode:        a.AssetCode,
		Name:             a.Name,
		AssetClass:       a.AssetClass,
		CurrencyCode:     a.CurrencyCode,
		Precision:        a.Precision,
		HolderTypes:      string(holders),
		FreezeSupported:  a.FreezeSupported,
		OverdraftAllowed: a.OverdraftAllowed,
		Status:           string(a.Status),
		Ext:              a.Ext,
	}
}

type LedgerAccount struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	AccountID  string `gorm:"size:64;uniqueIndex;not null"`
	TenantID   string `gorm:"size:64;uniqueIndex:uk_holder_asset;not null"`
	HolderType string `gorm:"size:32;uniqueIndex:uk_holder_asset;not null"`
	HolderID   string `gorm:"size:64;uniqueIndex:uk_holder_asset;not null"`
	AssetCode  string `gorm:"size:64;uniqueIndex:uk_holder_asset;not null"`
	Available  int64  `gorm:"not null"`
	Frozen     int64  `gorm:"not null"`
	Version    int64  `gorm:"not null"`
	Status     string `gorm:"size:16;not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (LedgerAccount) TableName() string { return "ledger_account" }

func (m *LedgerAccount) toDomain() *domain.Account {
	return &domain.Account{
		AccountID:  m.AccountID,
		TenantID:   m.TenantID,
		HolderType: domain.HolderType(m.HolderType),
		HolderID:   m.HolderID,
		AssetCode:  m.AssetCode,
		Available:  m.Available,
		Frozen:     m.Frozen,
		Version:    m.Version,
		Status:     domain.AccountStatus(m.Status),
	}
}

type LedgerEntry struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement"`
	EntryID        string `gorm:"size:64;uniqueIndex;not null"`
	AccountID      string `gorm:"size:64;index;not null"`
	TenantID       string    `gorm:"size:64;index:idx_entry_recon,priority:1;not null"`
	AssetCode      string    `gorm:"size:64;index:idx_entry_recon,priority:3;not null"`
	HolderType     string `gorm:"size:32;not null"`
	HolderID       string `gorm:"size:64;not null"`
	Direction      string `gorm:"size:8;not null"`
	Amount         int64  `gorm:"not null"`
	AvailableAfter int64  `gorm:"not null"`
	FrozenAfter    int64  `gorm:"not null"`
	Command        string `gorm:"size:32;not null"`
	SourceSystem   string    `gorm:"size:64;index:idx_entry_recon,priority:2;not null"`
	BizType        string `gorm:"size:64"`
	BizNo          string `gorm:"size:128;index;not null"`
	JournalID      string `gorm:"size:64;index"`
	FreezeID       string `gorm:"size:64;index"`
	RelatedBizNo   string `gorm:"size:128"`
	CreatedAt      time.Time `gorm:"index:idx_entry_recon,priority:4;index"`
}

func (LedgerEntry) TableName() string { return "ledger_entry" }

func (m *LedgerEntry) toDomain() *domain.LedgerEntry {
	return &domain.LedgerEntry{
		EntryID:        m.EntryID,
		AccountID:      m.AccountID,
		TenantID:       m.TenantID,
		AssetCode:      m.AssetCode,
		HolderType:     domain.HolderType(m.HolderType),
		HolderID:       m.HolderID,
		Direction:      domain.Direction(m.Direction),
		Amount:         m.Amount,
		AvailableAfter: m.AvailableAfter,
		FrozenAfter:    m.FrozenAfter,
		Command:        domain.Command(m.Command),
		SourceSystem:   m.SourceSystem,
		BizType:        m.BizType,
		BizNo:          m.BizNo,
		JournalID:      m.JournalID,
		FreezeID:       m.FreezeID,
		RelatedBizNo:   m.RelatedBizNo,
		CreatedAt:      m.CreatedAt,
	}
}

func entryFromDomain(e *domain.LedgerEntry) *LedgerEntry {
	return &LedgerEntry{
		EntryID:        e.EntryID,
		AccountID:      e.AccountID,
		TenantID:       e.TenantID,
		AssetCode:      e.AssetCode,
		HolderType:     string(e.HolderType),
		HolderID:       e.HolderID,
		Direction:      string(e.Direction),
		Amount:         e.Amount,
		AvailableAfter: e.AvailableAfter,
		FrozenAfter:    e.FrozenAfter,
		Command:        string(e.Command),
		SourceSystem:   e.SourceSystem,
		BizType:        e.BizType,
		BizNo:          e.BizNo,
		JournalID:      e.JournalID,
		FreezeID:       e.FreezeID,
		RelatedBizNo:   e.RelatedBizNo,
		CreatedAt:      e.CreatedAt,
	}
}

type LedgerFreeze struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	FreezeID  string `gorm:"size:64;uniqueIndex;not null"`
	BizNo     string `gorm:"size:128;uniqueIndex:uk_freeze_biz;not null"`
	TenantID  string `gorm:"size:64;uniqueIndex:uk_freeze_biz;not null"`
	AccountID string `gorm:"size:64;index;not null"`
	AssetCode string `gorm:"size:64;not null"`
	Amount    int64  `gorm:"not null"`
	Status    string `gorm:"size:16;index;not null"`
	ExpireAt  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (LedgerFreeze) TableName() string { return "ledger_freeze" }

func (m *LedgerFreeze) toDomain() *domain.FreezeOrder {
	return &domain.FreezeOrder{
		FreezeID:  m.FreezeID,
		BizNo:     m.BizNo,
		TenantID:  m.TenantID,
		AccountID: m.AccountID,
		AssetCode: m.AssetCode,
		Amount:    m.Amount,
		Status:    domain.FreezeStatus(m.Status),
		ExpireAt:  m.ExpireAt,
	}
}

type LedgerIdempotency struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	TenantID     string `gorm:"size:64;uniqueIndex:uk_idem;not null"`
	SourceSystem string `gorm:"size:64;uniqueIndex:uk_idem;not null"`
	BizNo        string `gorm:"size:128;uniqueIndex:uk_idem;not null"`
	Command      string `gorm:"size:32;uniqueIndex:uk_idem;not null"`
	RequestHash  string `gorm:"size:64;not null"`
	ResponseJSON string `gorm:"type:text;not null"`
	CreatedAt    time.Time
}

func (LedgerIdempotency) TableName() string { return "ledger_idempotency" }

type LedgerJournal struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	JournalID    string `gorm:"size:64;uniqueIndex;not null"`
	TenantID     string `gorm:"size:64;index;not null"`
	BizNo        string `gorm:"size:128;index;not null"`
	JournalType  string `gorm:"size:32;not null"`
	Status       string `gorm:"size:16;not null"`
	EntriesCount int    `gorm:"not null"`
	FxRateID     string `gorm:"size:64"`
	Ext          string `gorm:"type:text"`
	CreatedAt    time.Time
}

func (m *LedgerJournal) toDomain() *domain.Journal {
	return &domain.Journal{
		JournalID:    m.JournalID,
		TenantID:     m.TenantID,
		BizNo:        m.BizNo,
		JournalType:  m.JournalType,
		Status:       m.Status,
		EntriesCount: m.EntriesCount,
		FxRateID:     m.FxRateID,
		Ext:          m.Ext,
	}
}

func (LedgerJournal) TableName() string { return "ledger_journal" }

type LedgerFxRate struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	RateID      string `gorm:"size:64;uniqueIndex;not null"`
	TenantID    string `gorm:"size:64;index;not null"`
	BaseAsset   string `gorm:"size:64;not null"`
	QuoteAsset  string `gorm:"size:64;not null"`
	Rate        string `gorm:"size:32;not null"`
	RateSource  string `gorm:"size:16;not null"`
	ValidFrom   *time.Time
	ValidTo     *time.Time
	QuotedAt    time.Time
	CreatedBy   string `gorm:"size:64"`
	CreatedAt   time.Time
}

func (LedgerFxRate) TableName() string { return "ledger_fx_rate" }

type LedgerReconcileJob struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	JobID        string `gorm:"size:64;uniqueIndex;not null"`
	TenantID     string `gorm:"size:64;index:idx_recon_latest,priority:1;not null"`
	BizDate      string `gorm:"size:16;index:idx_recon_latest,priority:2;not null"`
	SourceSystem string `gorm:"size:64;index:idx_recon_latest,priority:3"`
	AssetCode    string `gorm:"size:64;index:idx_recon_latest,priority:4"`
	Status       string `gorm:"size:16;not null"`
	SummaryJSON  string `gorm:"type:text"`
	Note         string `gorm:"type:text"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (LedgerReconcileJob) TableName() string { return "ledger_reconcile_job" }

type LedgerReconcileDiff struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	DiffID       string `gorm:"size:64;uniqueIndex;not null"`
	JobID        string `gorm:"size:64;index;not null"`
	Kind         string `gorm:"size:32;not null"`
	BizNo        string `gorm:"size:128;index"`
	Command      string `gorm:"size:32"`
	AssetCode    string `gorm:"size:64"`
	BizAmount    int64
	LedgerAmount int64
	AccountID    string `gorm:"size:64;index"`
	Status       string `gorm:"size:16;index;not null"`
	Note         string `gorm:"type:text"`
	ResolvedBy   string `gorm:"size:64"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (LedgerReconcileDiff) TableName() string { return "ledger_reconcile_diff" }

type LedgerExchangeLeg struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	ExchangeID string `gorm:"size:64;uniqueIndex;not null"`
	JournalID  string `gorm:"size:64;index;not null"`
	BizNo      string `gorm:"size:128;index;not null"`
	TenantID   string `gorm:"size:64;index;not null"`
	HolderType string `gorm:"size:32;not null"`
	HolderID   string `gorm:"size:64;index;not null"`
	FromAsset  string `gorm:"size:64;not null"`
	FromAmount int64  `gorm:"not null"`
	ToAsset    string `gorm:"size:64;not null"`
	ToAmount   int64  `gorm:"not null"`
	FeeAsset   string `gorm:"size:64"`
	FeeAmount  int64
	RateID     string `gorm:"size:64"`
	Rate       string `gorm:"size:32"`
	Status     string `gorm:"size:16;not null"`
	CreatedAt  time.Time
}

func (LedgerExchangeLeg) TableName() string { return "ledger_exchange_leg" }

func (m *LedgerExchangeLeg) toDomain() *domain.ExchangeLeg {
	return &domain.ExchangeLeg{
		ExchangeID: m.ExchangeID,
		JournalID:  m.JournalID,
		BizNo:      m.BizNo,
		TenantID:   m.TenantID,
		HolderType: domain.HolderType(m.HolderType),
		HolderID:   m.HolderID,
		FromAsset:  m.FromAsset,
		FromAmount: m.FromAmount,
		ToAsset:    m.ToAsset,
		ToAmount:   m.ToAmount,
		FeeAsset:   m.FeeAsset,
		FeeAmount:  m.FeeAmount,
		RateID:     m.RateID,
		Rate:       m.Rate,
		Status:     m.Status,
	}
}

type LedgerTenant struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	TenantID  string `gorm:"size:64;uniqueIndex;not null"`
	Name      string `gorm:"size:128;not null"`
	Status    string `gorm:"size:16;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (LedgerTenant) TableName() string { return "ledger_tenant" }

func (m *LedgerTenant) toDomain() *domain.Tenant {
	return &domain.Tenant{TenantID: m.TenantID, Name: m.Name, Status: m.Status}
}

type LedgerLimitUsage struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	TenantID     string `gorm:"size:64;uniqueIndex:uk_limit;not null"`
	SourceSystem string `gorm:"size:64;uniqueIndex:uk_limit;not null"`
	HolderID     string `gorm:"size:64;uniqueIndex:uk_limit;not null"`
	AssetCode    string `gorm:"size:64;uniqueIndex:uk_limit;not null"`
	Command      string `gorm:"size:32;uniqueIndex:uk_limit;not null"`
	BizDate      string `gorm:"size:16;uniqueIndex:uk_limit;not null"`
	Amount       int64  `gorm:"not null"`
	Count        int    `gorm:"not null"`
	UpdatedAt    time.Time
}

func (LedgerLimitUsage) TableName() string { return "ledger_limit_usage" }

func (m *LedgerFxRate) toDomain() *domain.FxRate {
	return &domain.FxRate{
		RateID:     m.RateID,
		TenantID:   m.TenantID,
		BaseAsset:  m.BaseAsset,
		QuoteAsset: m.QuoteAsset,
		Rate:       m.Rate,
		RateSource: m.RateSource,
		ValidFrom:  m.ValidFrom,
		ValidTo:    m.ValidTo,
		QuotedAt:   m.QuotedAt,
		CreatedBy:  m.CreatedBy,
	}
}

type LedgerGatewayAudit struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	AuditID    string `gorm:"size:64;uniqueIndex;not null"`
	ClientID   string `gorm:"size:64;index;not null"`
	TenantID   string `gorm:"size:64;index"`
	Method     string `gorm:"size:16;not null"`
	Path       string `gorm:"size:255;not null"`
	Status     int    `gorm:"not null"`
	RemoteAddr string `gorm:"size:64"`
	RequestID  string `gorm:"size:64;index"`
	CreatedAt  time.Time `gorm:"index"`
}

func (LedgerGatewayAudit) TableName() string { return "ledger_gateway_audit" }

type LedgerLimitAlert struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	AlertID      string `gorm:"size:64;uniqueIndex;not null"`
	TenantID     string `gorm:"size:64;index;not null"`
	SourceSystem string `gorm:"size:64"`
	HolderID     string `gorm:"size:64;index"`
	AssetCode    string `gorm:"size:64"`
	Command      string `gorm:"size:32"`
	Reason       string `gorm:"size:128"`
	CreatedAt    time.Time `gorm:"index"`
}

func (LedgerLimitAlert) TableName() string { return "ledger_limit_alert" }

type LedgerOpsAudit struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	AuditID   string `gorm:"size:64;uniqueIndex;not null"`
	Operator  string `gorm:"size:64;index;not null"`
	Action    string `gorm:"size:32;index;not null"`
	TenantID  string `gorm:"size:64;index"`
	Target    string `gorm:"size:128"`
	Detail    string `gorm:"type:text"`
	CreatedAt time.Time `gorm:"index"`
}

func (LedgerOpsAudit) TableName() string { return "ledger_ops_audit" }

type LedgerOpsRun struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	RunID      string `gorm:"size:64;uniqueIndex;not null"`
	Name       string `gorm:"size:32;index;not null"`
	TenantID   string `gorm:"size:64;index"`
	Status     string `gorm:"size:16;not null"`
	Detail     string `gorm:"type:text"`
	Count      int
	StartedAt  time.Time
	FinishedAt *time.Time
}

func (LedgerOpsRun) TableName() string { return "ledger_ops_run" }

type LedgerTransferSaga struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	SagaID       string `gorm:"size:64;uniqueIndex;not null"`
	TenantID     string `gorm:"size:64;uniqueIndex:uk_saga_biz;not null"`
	SourceSystem string `gorm:"size:64;uniqueIndex:uk_saga_biz;not null"`
	BizNo        string `gorm:"size:128;uniqueIndex:uk_saga_biz;not null"`
	FromType     string `gorm:"size:32;not null"`
	FromID       string `gorm:"size:64;not null"`
	ToType       string `gorm:"size:32;not null"`
	ToID         string `gorm:"size:64;not null"`
	AssetCode    string `gorm:"size:64;not null"`
	Amount       int64  `gorm:"not null"`
	Status       string `gorm:"size:16;index;not null"`
	OutBizNo     string `gorm:"size:160"`
	InBizNo      string `gorm:"size:160"`
	RollbackNo   string `gorm:"size:160"`
	ResultJSON   string `gorm:"type:text"`
	LastError    string `gorm:"type:text"`
	RetryCount   int
	CreatedAt    time.Time
	UpdatedAt    time.Time `gorm:"index"`
}

func (LedgerTransferSaga) TableName() string { return "ledger_transfer_saga" }

type LedgerGatewayNonce struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	ClientID  string `gorm:"size:64;uniqueIndex:uk_gw_nonce;not null"`
	Nonce     string `gorm:"size:64;uniqueIndex:uk_gw_nonce;not null"`
	CreatedAt time.Time `gorm:"index"`
}

func (LedgerGatewayNonce) TableName() string { return "ledger_gateway_nonce" }
