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
}

type EntryRepository interface {
	Create(ctx context.Context, e *LedgerEntry) error
	ListByBizNo(ctx context.Context, tenantID, bizNo string) ([]*LedgerEntry, error)
	ListByHolder(ctx context.Context, tenantID string, holder Holder, assetCode string, from, to *time.Time) ([]*LedgerEntry, error)
}

type FreezeRepository interface {
	Create(ctx context.Context, f *FreezeOrder) error
	GetByID(ctx context.Context, freezeID string) (*FreezeOrder, error)
	GetByBizNo(ctx context.Context, tenantID, bizNo string) (*FreezeOrder, error)
	UpdateStatus(ctx context.Context, freezeID string, from, to FreezeStatus) error
	ListExpired(ctx context.Context, now time.Time, limit int) ([]*FreezeOrder, error)
}

type IdempotencyRepository interface {
	Get(ctx context.Context, tenantID, sourceSystem, bizNo string, cmd Command) (*IdempotencyRecord, error)
	Create(ctx context.Context, rec *IdempotencyRecord) error
}
