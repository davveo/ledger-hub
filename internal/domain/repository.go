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
	CountByTenant(ctx context.Context, tenantID, assetCode string) (int64, error)
	ListRecentByTenant(ctx context.Context, tenantID, assetCode string, limit int) ([]*Account, error)
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
	ListQueuedJobs(ctx context.Context, limit int) ([]*ReconcileJob, error)
	LatestJob(ctx context.Context, tenantID, date, sourceSystem, assetCode string) (*ReconcileJob, error)
	FindJobByKey(ctx context.Context, tenantID, date, sourceSystem, assetCode, jobType string) (*ReconcileJob, error)
	CreateDiffs(ctx context.Context, diffs []*ReconcileDiff) error
	ListDiffs(ctx context.Context, jobID string) ([]*ReconcileDiff, error)
	ListOpenDiffs(ctx context.Context, tenantID string, limit int) ([]*ReconcileDiff, error)
	GetDiff(ctx context.Context, diffID string) (*ReconcileDiff, error)
	UpdateDiff(ctx context.Context, d *ReconcileDiff) error
	ResolveDiff(ctx context.Context, diffID, note, operator string) error
	CreateDiffEvent(ctx context.Context, ev *ReconcileDiffEvent) error
	ListDiffEvents(ctx context.Context, diffID string) ([]*ReconcileDiffEvent, error)
}

type IdempotencyRepository interface {
	Get(ctx context.Context, tenantID, sourceSystem, bizNo string, cmd Command) (*IdempotencyRecord, error)
	Create(ctx context.Context, rec *IdempotencyRecord) error
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}

type AuditRepository interface {
	Create(ctx context.Context, a *GatewayAudit) error
	List(ctx context.Context, q AuditQuery) ([]*GatewayAudit, error)
}

type AlertRepository interface {
	Create(ctx context.Context, a *LimitAlert) error
	List(ctx context.Context, tenantID string, limit int) ([]*LimitAlert, error)
}

type OpsAuditRepository interface {
	Create(ctx context.Context, a *OpsAudit) error
	List(ctx context.Context, q AuditQuery) ([]*OpsAudit, error)
}

type OpsRunRepository interface {
	Save(ctx context.Context, run *OpsRun) error
	List(ctx context.Context, limit int) ([]*OpsRun, error)
	LastSuccess(ctx context.Context) (map[string]time.Time, error)
}

type JobLeaseRepository interface {
	Acquire(ctx context.Context, jobName, holder string, ttl time.Duration) (bool, error)
	Renew(ctx context.Context, jobName, holder string, ttl time.Duration) (bool, error)
}

type ConfigRevisionRepository interface {
	Create(ctx context.Context, r *ConfigRevision) error
	List(ctx context.Context, limit int) ([]*ConfigRevision, error)
	Latest(ctx context.Context) (*ConfigRevision, error)
}

type InboxRepository interface {
	InsertIfAbsent(ctx context.Context, m *InboxMessage) (inserted bool, err error)
	Get(ctx context.Context, messageID string) (*InboxMessage, error)
	Update(ctx context.Context, m *InboxMessage) error
	List(ctx context.Context, status string, limit int) ([]*InboxMessage, error)
	ListDue(ctx context.Context, now time.Time, limit int) ([]*InboxMessage, error)
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

type SagaRepository interface {
	Create(ctx context.Context, s *TransferSaga) error
	Update(ctx context.Context, s *TransferSaga) error
	Get(ctx context.Context, sagaID string) (*TransferSaga, error)
	GetByBizNo(ctx context.Context, tenantID, sourceSystem, bizNo string) (*TransferSaga, error)
	ListOpen(ctx context.Context, tenantID string, limit int) ([]*TransferSaga, error)
	List(ctx context.Context, tenantID, status string, limit int) ([]*TransferSaga, error)
}

type NonceRepository interface {
	Consume(ctx context.Context, clientID, nonce string, ttl time.Duration) error
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}
