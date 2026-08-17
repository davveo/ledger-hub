package persistence

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/davveo/ledger-hub/migrations"
)

type MigrationRecord struct {
	Version   string
	Name      string
	AppliedAt time.Time
	Success   bool
	Error     string
}

func RunMigrations(ctx context.Context, cluster *Cluster, log *zap.Logger) error {
	if cluster == nil {
		return fmt.Errorf("cluster is nil")
	}
	if log == nil {
		log = zap.NewNop()
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	for i, db := range cluster.All() {
		if err := db.WithContext(ctx).AutoMigrate(&LedgerSchemaMigration{}); err != nil {
			return fmt.Errorf("shard %d schema_migration table: %w", i, err)
		}
		log.Info("migrate shard", zap.Int("shard", i))
		if err := AutoMigrate(db); err != nil {
			return fmt.Errorf("shard %d automigrate: %w", i, err)
		}
		for _, name := range files {
			version := strings.TrimSuffix(name, ".sql")
			if applied, _ := migrationApplied(db, version); applied {
				continue
			}
			body, err := fs.ReadFile(migrations.FS, name)
			if err != nil {
				return err
			}
			rec := LedgerSchemaMigration{
				Version:   version,
				Name:      name,
				AppliedAt: time.Now().UTC(),
				Success:   true,
			}
			sql := strings.TrimSpace(string(body))
			if err := execSQL(db.WithContext(ctx), sql); err != nil {
				rec.Success = false
				rec.Error = err.Error()
				_ = db.Create(&rec).Error
				return fmt.Errorf("migration %s shard %d: %w", name, i, err)
			}
			if err := db.Create(&rec).Error; err != nil {
				return err
			}
			log.Info("applied migration", zap.String("version", version), zap.Int("shard", i))
		}
	}
	return nil
}

func migrationApplied(db *gorm.DB, version string) (bool, error) {
	var n int64
	err := db.Model(&LedgerSchemaMigration{}).Where("version = ? AND success = ?", version, true).Count(&n).Error
	return n > 0, err
}

func execSQL(db *gorm.DB, sql string) error {
	parts := splitSQL(sql)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "--") {
			continue
		}
		if err := db.Exec(p).Error; err != nil {
			if isIgnorableSchemaErr(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func splitSQL(sql string) []string {
	var out []string
	var b strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if strings.HasSuffix(trim, ";") {
			out = append(out, b.String())
			b.Reset()
		}
	}
	if rest := strings.TrimSpace(b.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}

func isIgnorableSchemaErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "Duplicate key name") ||
		strings.Contains(s, "Duplicate column name") ||
		strings.Contains(s, "already exists") ||
		strings.Contains(s, "1061") ||
		strings.Contains(s, "1060") ||
		strings.Contains(s, "1050")
}
