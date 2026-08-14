package application

import (
	"context"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

type Limiter struct {
	rules []domain.LimitRule
	store domain.LimitRepository
}

func NewLimiter(rules []domain.LimitRule, store domain.LimitRepository) *Limiter {
	return &Limiter{rules: rules, store: store}
}

func (l *Limiter) Check(ctx context.Context, req domain.CommandRequest) error {
	if l == nil || len(l.rules) == 0 || l.store == nil {
		return nil
	}
	amt := req.Amount
	if amt <= 0 {
		amt = req.ToAmount
	}
	asset := req.AssetCode
	date := time.Now().UTC().Format("2006-01-02")
	for _, r := range l.rules {
		if r.SourceSystem != "" && r.SourceSystem != "*" && r.SourceSystem != req.SourceSystem {
			continue
		}
		if r.Command != "" && r.Command != "*" && r.Command != req.Command {
			continue
		}
		if r.AssetCode != "" && r.AssetCode != "*" && r.AssetCode != asset {
			continue
		}
		if r.MaxAmount > 0 && amt > r.MaxAmount {
			return domain.ErrRateLimited
		}
		sum, count, err := l.store.AddUsage(ctx, req.TenantID, req.SourceSystem, req.Holder.ID, asset, req.Command, date, amt)
		if err != nil {
			return err
		}
		if r.DailyAmount > 0 && sum > r.DailyAmount {
			return domain.ErrRateLimited
		}
		if r.DailyCount > 0 && count > r.DailyCount {
			return domain.ErrRateLimited
		}
	}
	return nil
}
