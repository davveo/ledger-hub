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
	)
}

type TxManager struct {
	db *gorm.DB
}

func NewTxManager(db *gorm.DB) *TxManager {
	return &TxManager{db: db}
}

func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, ctxKey{}, tx))
	})
}

func dbFrom(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}

type Repos struct {
	Asset       domain.AssetRepository
	Account     domain.AccountRepository
	Entry       domain.EntryRepository
	Freeze      domain.FreezeRepository
	Idempotency domain.IdempotencyRepository
}

func NewRepos(db *gorm.DB) *Repos {
	return &Repos{
		Asset:       NewAssetRepo(db),
		Account:     NewAccountRepo(db),
		Entry:       NewEntryRepo(db),
		Freeze:      NewFreezeRepo(db),
		Idempotency: NewIdempotencyRepo(db),
	}
}

func now() time.Time {
	return time.Now().UTC()
}
