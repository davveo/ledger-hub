package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davveo/ledger-hub/internal/domain"
)

type LeaseRepo struct{ db *gorm.DB }

func NewLeaseRepo(db *gorm.DB) *LeaseRepo { return &LeaseRepo{db: db} }

func (r *LeaseRepo) Acquire(ctx context.Context, jobName, holder string, ttl time.Duration) (bool, error) {
	return r.claim(ctx, jobName, holder, ttl, false)
}

func (r *LeaseRepo) Renew(ctx context.Context, jobName, holder string, ttl time.Duration) (bool, error) {
	return r.claim(ctx, jobName, holder, ttl, true)
}

func (r *LeaseRepo) claim(ctx context.Context, jobName, holder string, ttl time.Duration, renewOnly bool) (bool, error) {
	if jobName == "" || holder == "" {
		return false, domain.ErrInvalidParam
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := time.Now().UTC()
	exp := now.Add(ttl)
	held := false
	err := dbFrom(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		var row LedgerJobLease
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_name = ?", jobName).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if renewOnly {
				return nil
			}
			held = true
			return tx.Create(&LedgerJobLease{JobName: jobName, Holder: holder, ExpiresAt: exp, UpdatedAt: now}).Error
		}
		if err != nil {
			return err
		}
		if row.Holder != holder && row.ExpiresAt.After(now) {
			return nil
		}
		if renewOnly && row.Holder != holder {
			return nil
		}
		held = true
		return tx.Model(&LedgerJobLease{}).Where("job_name = ?", jobName).Updates(map[string]interface{}{
			"holder":     holder,
			"expires_at": exp,
			"updated_at": now,
		}).Error
	})
	return held, err
}
