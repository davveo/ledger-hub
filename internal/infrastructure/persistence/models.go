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
	BizNo     string `gorm:"size:128;uniqueIndex;not null"`
	TenantID  string `gorm:"size:64;not null"`
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
	BizNo        string `gorm:"size:128;index;not null"`
	JournalType  string `gorm:"size:32;not null"`
	Status       string `gorm:"size:16;not null"`
	EntriesCount int    `gorm:"not null"`
	FxRateID     string `gorm:"size:64"`
	Ext          string `gorm:"type:text"`
	CreatedAt    time.Time
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
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (LedgerReconcileDiff) TableName() string { return "ledger_reconcile_diff" }
