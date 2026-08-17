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
	if err := prepareReconcileJobUnique(db); err != nil {
		return err
	}
	err := db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4").AutoMigrate(
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
		&LedgerLimitAlert{},
		&LedgerOpsAudit{},
		&LedgerOpsRun{},
		&LedgerTransferSaga{},
		&LedgerGatewayNonce{},
		&LedgerJobLease{},
		&LedgerConfigRevision{},
		&LedgerMQInbox{},
		&LedgerSchemaMigration{},
		&LedgerReconcileDiffEvent{},
	)
	if err != nil {
		return err
	}
	for _, idx := range []string{"idx_ledger_freeze_biz_no", "uni_ledger_freeze_biz_no"} {
		_ = db.Exec("ALTER TABLE ledger_freeze DROP INDEX " + idx).Error
	}
	return nil
}

// prepareReconcileJobUnique backfills job_type/version and makes existing rows unique
// so AutoMigrate can create uk_recon_job on databases that already have duplicate jobs.
func prepareReconcileJobUnique(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&LedgerReconcileJob{}) {
		return nil
	}
	if db.Migrator().HasIndex(&LedgerReconcileJob{}, "uk_recon_job") {
		return nil
	}
	if !db.Migrator().HasColumn(&LedgerReconcileJob{}, "job_type") {
		if err := db.Exec("ALTER TABLE ledger_reconcile_job ADD COLUMN job_type varchar(32) NOT NULL DEFAULT 'daily'").Error; err != nil {
			return err
		}
	}
	if !db.Migrator().HasColumn(&LedgerReconcileJob{}, "version") {
		if err := db.Exec("ALTER TABLE ledger_reconcile_job ADD COLUMN version int NOT NULL DEFAULT 1").Error; err != nil {
			return err
		}
	}
	var rows []LedgerReconcileJob
	if err := db.Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return err
	}
	assigned := assignReconcileJobVersions(rows)
	for i, row := range assigned {
		if rows[i].JobType == row.JobType && rows[i].Version == row.Version {
			continue
		}
		if err := db.Model(&LedgerReconcileJob{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
			"job_type": row.JobType,
			"version":  row.Version,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

type reconJobKey struct {
	tenant, date, source, asset, jobType string
}

func assignReconcileJobVersions(rows []LedgerReconcileJob) []LedgerReconcileJob {
	out := make([]LedgerReconcileJob, len(rows))
	copy(out, rows)
	counters := map[reconJobKey]int{}
	for i := range out {
		jt := out[i].JobType
		if jt == "" {
			jt = "daily"
		}
		k := reconJobKey{out[i].TenantID, out[i].BizDate, out[i].SourceSystem, out[i].AssetCode, jt}
		counters[k]++
		out[i].JobType = jt
		out[i].Version = counters[k]
	}
	return out
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
	Alert       domain.AlertRepository
	OpsAudit    domain.OpsAuditRepository
	OpsRun      domain.OpsRunRepository
	Saga        domain.SagaRepository
	Nonce       domain.NonceRepository
	Lease       domain.JobLeaseRepository
	ConfigRev   domain.ConfigRevisionRepository
	Inbox       domain.InboxRepository
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
		Alert:       NewAlertRepo(db),
		OpsAudit:    NewOpsAuditRepo(db),
		OpsRun:      NewOpsRunRepo(db),
		Saga:        NewSagaRepo(db),
		Nonce:       NewNonceRepo(db),
		Lease:       NewLeaseRepo(db),
		ConfigRev:   NewConfigRevisionRepo(db),
		Inbox:       NewInboxRepo(db),
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
		Alert:       NewAlertRepo(primary),
		OpsAudit:    NewOpsAuditRepo(primary),
		OpsRun:      NewOpsRunRepo(primary),
		Saga:        NewSagaRepo(primary),
		Nonce:       NewNonceRepo(primary),
		Lease:       NewLeaseRepo(primary),
		ConfigRev:   NewConfigRevisionRepo(primary),
		Inbox:       NewInboxRepo(primary),
	}
}

func now() time.Time {
	return time.Now().UTC()
}
