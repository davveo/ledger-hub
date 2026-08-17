package persistence

import (
	"context"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/infrastructure/shard"
)

type Cluster struct {
	nodes []*gorm.DB
}

func OpenCluster(cfg config.MySQLConfig) (*Cluster, error) {
	dsns := []string{cfg.DSN}
	for _, extra := range cfg.Shards {
		if extra != "" && extra != cfg.DSN {
			dsns = append(dsns, extra)
		}
	}
	c := &Cluster{}
	for i, dsn := range dsns {
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
		if err != nil {
			return nil, fmt.Errorf("open shard %d: %w", i, err)
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
		c.nodes = append(c.nodes, db)
	}
	return c, nil
}

func (c *Cluster) Primary() *gorm.DB { return c.nodes[0] }

func (c *Cluster) All() []*gorm.DB { return c.nodes }

func (c *Cluster) ForHolder(holderID string) *gorm.DB {
	return c.nodes[shard.Index(holderID, len(c.nodes))]
}

func (c *Cluster) SameShard(a, b string) bool {
	n := len(c.nodes)
	return shard.Index(a, n) == shard.Index(b, n)
}

func (c *Cluster) AutoMigrate() error {
	if c == nil {
		return fmt.Errorf("cluster is nil")
	}
	for _, db := range c.nodes {
		if err := AutoMigrate(db); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) Ping(ctx context.Context) error {
	if c == nil || len(c.nodes) == 0 {
		return fmt.Errorf("cluster is nil")
	}
	for i, db := range c.nodes {
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("shard %d: %w", i, err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("shard %d ping: %w", i, err)
		}
		if err := db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
			return fmt.Errorf("shard %d select 1: %w", i, err)
		}
	}
	return nil
}
