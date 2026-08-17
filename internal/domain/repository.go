package domain

import (
	"context"
	"time"
)

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type AssetRepository interface {
	Save(ctx context.Context, a *Asset) error
	Get(ctx context.Context, tenantID, assetCode string) (*Asset, error)
	List(ctx context.Context, tenantID string) ([]*Asset, error)
}

type AccountRepository interface {
	GetByID(ctx context.Context, accountID string) (*Account, error)
	Get(ctx context.Context, tenantID string, holder Holder, assetCode string) (*Account, error)
	GetForUpdate(ctx context.Context, tenantID string, holder Holder, assetCode string) (*Account, error)
	Create(ctx context.Context, a *Account) error
	UpdateBalances(ctx context.Context, a *Account) error
	UpdateStatus(ctx context.Context, a *Account) error
	ListByTenant(ctx context.Context, tenantID, assetCode string) ([]*Account, error)
	ListByHolder(ctx context.Context, tenantID string, holder Holder, assetCode string) ([]*Account, error)
}

type EntryRepository interface {
	Create(ctx context.Context, e *LedgerEntry) error
	ListByBizNo(ctx context.Context, tenantID, bizNo string) ([]*LedgerEntry, error)
	ListByHolder(ctx context.Context, tenantID string, holder Holder, assetCode string, from, to *time.Time, page Page) ([]*LedgerEntry, error)
	ListByRange(ctx context.Context, tenantID, sourceSystem, assetCode string, from, to time.Time) ([]*LedgerEntry, error)
	ListByAccount(ctx context.Context, accountID string) ([]*LedgerEntry, error)
	ListByJournal(ctx context.Context, journalID string) ([]*LedgerEntry, error)
}

type FreezeRepository interface {
	Create(ctx context.Context, f *FreezeOrder) error
	GetByID(ctx context.Context, freezeID string) (*FreezeOrder, error)
	GetByBizNo(ctx context.Context, tenantID, bizNo string) (*FreezeOrder, error)
	UpdateStatus(ctx context.Context, freezeID string, from, to FreezeStatus) error
	Update(ctx context.Context, f *FreezeOrder) error
	ListExpired(ctx context.Context, now time.Time, limit int) ([]*FreezeOrder, error)
	ListFrozen(ctx context.Context, tenantID, assetCode string) ([]*FreezeOrder, error)
	ListByHolder(ctx context.Context, tenantID string, holder Holder, assetCode, status string, page Page) ([]*FreezeOrder, error)
}

type ReconcileRepository interface {
	CreateJob(ctx context.Context, job *ReconcileJob) error
	UpdateJob(ctx context.Context, job *ReconcileJob) error
	GetJob(ctx context.Context, jobID string) (*ReconcileJob, error)
	ListJobs(ctx context.Context, tenantID string, limit int) ([]*ReconcileJob, error)
	LatestJob(ctx context.Context, tenantID, date, sourceSystem, assetCode string) (*ReconcileJob, error)
	CreateDiffs(ctx context.Context, diffs []*ReconcileDiff) error
	ListDiffs(ctx context.Context, jobID string) ([]*ReconcileDiff, error)
	ListOpenDiffs(ctx context.Context, tenantID string, limit int) ([]*ReconcileDiff, error)
	ResolveDiff(ctx context.Context, diffID, note, operator string) error
}

type IdempotencyRepository interface {
	Get(ctx context.Context, tenantID, sourceSystem, bizNo string, cmd Command) (*IdempotencyRecord, error)
	Create(ctx context.Context, rec *IdempotencyRecord) error
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}

type AuditRepository interface {
	Create(ctx context.Context, a *GatewayAudit) error
	List(ctx context.Context, limit int) ([]*GatewayAudit, error)
}

type AlertRepository interface {
	Create(ctx context.Context, a *LimitAlert) error
	List(ctx context.Context, tenantID string, limit int) ([]*LimitAlert, error)
}

type OpsAuditRepository interface {
	Create(ctx context.Context, a *OpsAudit) error
	List(ctx context.Context, tenantID string, limit int) ([]*OpsAudit, error)
}

type OpsRunRepository interface {
	Save(ctx context.Context, run *OpsRun) error
	List(ctx context.Context, limit int) ([]*OpsRun, error)
}

type JournalRepository interface {
	Create(ctx context.Context, j *Journal) error
	Get(ctx context.Context, journalID string) (*Journal, error)
	ListByRange(ctx context.Context, tenantID, journalType string, from, to time.Time) ([]*Journal, error)
}

type FxRateRepository interface {
	Save(ctx context.Context, r *FxRate) error
	Get(ctx context.Context, rateID string) (*FxRate, error)
	Find(ctx context.Context, tenantID, base, quote string, at time.Time) (*FxRate, error)
	List(ctx context.Context, tenantID string) ([]*FxRate, error)
}

type ExchangeLegRepository interface {
	Create(ctx context.Context, leg *ExchangeLeg) error
	GetByBizNo(ctx context.Context, tenantID, bizNo string) (*ExchangeLeg, error)
	ListByRange(ctx context.Context, tenantID string, from, to time.Time) ([]*ExchangeLeg, error)
}

type TenantRepository interface {
	Save(ctx context.Context, t *Tenant) error
	Get(ctx context.Context, tenantID string) (*Tenant, error)
	List(ctx context.Context) ([]*Tenant, error)
}

type LimitRepository interface {
	AddUsage(ctx context.Context, tenantID, source, holderID, asset string, cmd Command, date string, amount int64) (sum int64, count int, err error)
}
