package application

import (
	"context"
	"sync"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type Limiter struct {
	mu     sync.RWMutex
	rules  []domain.LimitRule
	store  domain.LimitRepository
	persist domain.AlertRepository
	alerts []domain.LimitAlert
}

func NewLimiter(rules []domain.LimitRule, store domain.LimitRepository) *Limiter {
	return &Limiter{rules: append([]domain.LimitRule(nil), rules...), store: store}
}

func (l *Limiter) WithAlerts(store domain.AlertRepository) *Limiter {
	if l != nil {
		l.persist = store
	}
	return l
}

func (l *Limiter) Replace(rules []domain.LimitRule) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.rules = append([]domain.LimitRule(nil), rules...)
	l.mu.Unlock()
}

func (l *Limiter) Rules() []domain.LimitRule {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]domain.LimitRule(nil), l.rules...)
}

func (l *Limiter) Alerts() []domain.LimitAlert {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]domain.LimitAlert, len(l.alerts))
	copy(out, l.alerts)
	return out
}

func (l *Limiter) ListAlerts(ctx context.Context, tenantID string, limit int) []domain.LimitAlert {
	if l != nil && l.persist != nil {
		list, err := l.persist.List(ctx, tenantID, limit)
		if err == nil && len(list) > 0 {
			out := make([]domain.LimitAlert, 0, len(list))
			for _, a := range list {
				out = append(out, *a)
			}
			return out
		}
	}
	return l.Alerts()
}

func (l *Limiter) Check(ctx context.Context, req domain.CommandRequest) error {
	if l == nil || l.store == nil {
		return nil
	}
	l.mu.RLock()
	rules := append([]domain.LimitRule(nil), l.rules...)
	l.mu.RUnlock()
	if len(rules) == 0 {
		return nil
	}
	amt := req.Amount
	if amt <= 0 {
		amt = req.ToAmount
	}
	asset := req.AssetCode
	date := time.Now().UTC().Format("2006-01-02")
	for _, r := range rules {
		if r.TenantID != "" && r.TenantID != req.TenantID {
			continue
		}
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
			l.note(ctx, req, "超过单笔限额")
			return domain.ErrRateLimited
		}
		sum, count, err := l.store.AddUsage(ctx, req.TenantID, req.SourceSystem, req.Holder.ID, asset, req.Command, date, amt)
		if err != nil {
			return err
		}
		if r.DailyAmount > 0 && sum > r.DailyAmount {
			l.note(ctx, req, "超过日累计金额")
			return domain.ErrRateLimited
		}
		if r.DailyCount > 0 && count > r.DailyCount {
			l.note(ctx, req, "超过日累计笔数")
			return domain.ErrRateLimited
		}
	}
	return nil
}

func (l *Limiter) note(ctx context.Context, req domain.CommandRequest, reason string) {
	alert := domain.LimitAlert{
		AlertID:      idgen.New("alt_"),
		At:           time.Now().UTC(),
		TenantID:     req.TenantID,
		SourceSystem: req.SourceSystem,
		HolderID:     req.Holder.ID,
		AssetCode:    req.AssetCode,
		Command:      req.Command,
		Reason:       reason,
	}
	l.mu.Lock()
	l.alerts = append(l.alerts, alert)
	if len(l.alerts) > 50 {
		l.alerts = l.alerts[len(l.alerts)-50:]
	}
	persist := l.persist
	l.mu.Unlock()
	if persist != nil {
		_ = persist.Create(ctx, &alert)
	}
}
