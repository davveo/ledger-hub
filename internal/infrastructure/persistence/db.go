package persistence

import (
	"context"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
)

type ctxKey struct{}

func Open(cfg config.MySQLConfig) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4").AutoMigrate(
		&LedgerAsset{},
		&LedgerAccount{},
		&LedgerEntry{},
		&LedgerFreeze{},
		&LedgerIdempotency{},
		&LedgerJournal{},
		&LedgerFxRate{},
		&LedgerExchangeLeg{},
		&LedgerTenant{},
		&LedgerLimitUsage{},
		&LedgerReconcileJob{},
		&LedgerReconcileDiff{},
		&LedgerGatewayAudit{},
	)
}

type TxManager struct {
	db      *gorm.DB
	cluster *Cluster
}

func NewTxManager(db *gorm.DB) *TxManager {
	return &TxManager{db: db}
}

func NewClusterTxManager(c *Cluster) *TxManager {
	return &TxManager{db: c.Primary(), cluster: c}
}

func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	db := m.db
	if m.cluster != nil {
		if h := domain.HolderIDFrom(ctx); h != "" {
			db = m.cluster.ForHolder(h)
		} else {
			db = m.cluster.Primary()
		}
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, ctxKey{}, tx))
	})
}

func dbFrom(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}

func scanDBs(ctx context.Context, cluster *Cluster, fallback *gorm.DB) []*gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok && tx != nil {
		return []*gorm.DB{tx.WithContext(ctx)}
	}
	if cluster != nil {
		out := make([]*gorm.DB, 0, len(cluster.All()))
		for _, db := range cluster.All() {
			out = append(out, db.WithContext(ctx))
		}
		return out
	}
	return []*gorm.DB{fallback.WithContext(ctx)}
}

func routeDB(ctx context.Context, cluster *Cluster, fallback *gorm.DB, holderID string) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	if cluster != nil && holderID != "" {
		return cluster.ForHolder(holderID).WithContext(ctx)
	}
	if h := domain.HolderIDFrom(ctx); cluster != nil && h != "" {
		return cluster.ForHolder(h).WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}

type Repos struct {
	Asset       domain.AssetRepository
	Account     domain.AccountRepository
	Entry       domain.EntryRepository
	Freeze      domain.FreezeRepository
	Idempotency domain.IdempotencyRepository
	Reconcile   domain.ReconcileRepository
	Journal     domain.JournalRepository
	FxRate      domain.FxRateRepository
	ExchangeLeg domain.ExchangeLegRepository
	Tenant      domain.TenantRepository
	Limit       domain.LimitRepository
	Audit       domain.AuditRepository
}

func NewRepos(db *gorm.DB) *Repos {
	return &Repos{
		Asset:       NewAssetRepo(db),
		Account:     NewAccountRepo(db),
		Entry:       NewEntryRepo(db),
		Freeze:      NewFreezeRepo(db),
		Idempotency: NewIdempotencyRepo(db),
		Reconcile:   NewReconcileRepo(db),
		Journal:     NewJournalRepo(db),
		FxRate:      NewFxRateRepo(db),
		ExchangeLeg: NewExchangeLegRepo(db),
		Tenant:      NewTenantRepo(db),
		Limit:       NewLimitRepo(db),
		Audit:       NewAuditRepo(db),
	}
}

func NewClusterRepos(c *Cluster) *Repos {
	primary := c.Primary()
	return &Repos{
		Asset:       NewAssetRepo(primary),
		Account:     NewAccountRepo(primary).WithCluster(c),
		Entry:       NewEntryRepo(primary).WithCluster(c),
		Freeze:      NewFreezeRepo(primary).WithCluster(c),
		Idempotency: NewIdempotencyRepo(primary).WithCluster(c),
		Reconcile:   NewReconcileRepo(primary),
		Journal:     NewJournalRepo(primary).WithCluster(c),
		FxRate:      NewFxRateRepo(primary),
		ExchangeLeg: NewExchangeLegRepo(primary).WithCluster(c),
		Tenant:      NewTenantRepo(primary),
		Limit:       NewLimitRepo(primary).WithCluster(c),
		Audit:       NewAuditRepo(primary),
	}
}

func now() time.Time {
	return time.Now().UTC()
}
