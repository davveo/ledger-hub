package application

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
)

type Jobs struct {
	books   *Bookkeeping
	recon   *ReconcileService
	expire  *ExpireEngine
	freezes domain.FreezeRepository
	fx      *FxService
	tenants domain.TenantRepository
	idem    domain.IdempotencyRepository
	runs    domain.OpsRunRepository
	audit   domain.OpsAuditRepository
	feeds   []domain.FxFeedPair
	tenant  string
	retain  time.Duration
}

func NewJobs(books *Bookkeeping, recon *ReconcileService, expire *ExpireEngine, freezes domain.FreezeRepository, tenant string) *Jobs {
	return &Jobs{books: books, recon: recon, expire: expire, freezes: freezes, tenant: tenant, retain: 192 * time.Hour}
}

func (j *Jobs) WithFx(fx *FxService, feeds []domain.FxFeedPair) *Jobs {
	j.fx = fx
	j.feeds = feeds
	return j
}

func (j *Jobs) WithTenants(t domain.TenantRepository) *Jobs {
	j.tenants = t
	return j
}

func (j *Jobs) WithIdempotency(idem domain.IdempotencyRepository, retain time.Duration) *Jobs {
	j.idem = idem
	if retain > 0 {
		j.retain = retain
	}
	return j
}

func (j *Jobs) WithRuns(runs domain.OpsRunRepository) *Jobs {
	j.runs = runs
	return j
}

func (j *Jobs) WithAudit(a domain.OpsAuditRepository) *Jobs {
	j.audit = a
	return j
}

func (j *Jobs) Record(ctx context.Context, operator, action, tenant, target, detail string) {
	if j == nil || j.audit == nil || action == "" {
		return
	}
	_ = j.audit.Create(ctx, &domain.OpsAudit{
		Operator: operator,
		Action:   action,
		TenantID: tenant,
		Target:   target,
		Detail:   detail,
	})
}

func (j *Jobs) ListRuns(ctx context.Context, limit int) ([]*domain.OpsRun, error) {
	if j == nil || j.runs == nil {
		return []*domain.OpsRun{}, nil
	}
	return j.runs.List(ctx, limit)
}

func (j *Jobs) ListActions(ctx context.Context, tenantID string, limit int) ([]*domain.OpsAudit, error) {
	if j == nil || j.audit == nil {
		return []*domain.OpsAudit{}, nil
	}
	return j.audit.List(ctx, tenantID, limit)
}

func (j *Jobs) tenantIDs(ctx context.Context) []string {
	if j.tenants != nil {
		list, err := j.tenants.List(ctx)
		if err == nil && len(list) > 0 {
			var ids []string
			for _, t := range list {
				if t.Status == "" || t.Status == "active" {
					ids = append(ids, t.TenantID)
				}
			}
			if len(ids) > 0 {
				return ids
			}
		}
	}
	if j.tenant != "" {
		return []string{j.tenant}
	}
	return nil
}

func (j *Jobs) track(ctx context.Context, name, tenant string, fn func() (int, string, error)) *domain.OpsRun {
	run := &domain.OpsRun{
		RunID:     idgen.New("run_"),
		Name:      name,
		TenantID:  tenant,
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	n, detail, err := fn()
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Count = n
	run.Detail = detail
	if err != nil {
		run.Status = "failed"
		if run.Detail == "" {
			run.Detail = err.Error()
		} else {
			run.Detail = run.Detail + "; " + err.Error()
		}
	} else {
		run.Status = "done"
	}
	if j != nil && j.runs != nil {
		_ = j.runs.Save(ctx, run)
	}
	return run
}

func (j *Jobs) ReleaseExpired(ctx context.Context) *domain.OpsRun {
	return j.track(ctx, "release_expired", "", func() (int, string, error) {
		if j.freezes == nil || j.books == nil {
			return 0, "", nil
		}
		list, err := j.freezes.ListExpired(ctx, time.Now().UTC(), 100)
		if err != nil {
			return 0, "", err
		}
		okN := 0
		for _, fz := range list {
			_, err := j.books.Execute(ctx, domain.CommandRequest{
				Command:      domain.CmdRelease,
				TenantID:     fz.TenantID,
				SourceSystem: "worker",
				BizType:      "freeze_expire",
				BizNo:        "worker:expire:" + fz.FreezeID,
				FreezeID:     fz.FreezeID,
				AssetCode:    fz.AssetCode,
			})
			if err != nil {
				continue
			}
			okN++
		}
		return okN, fmt.Sprintf("expired=%d released=%d", len(list), okN), nil
	})
}

func (j *Jobs) Reconcile(ctx context.Context, date string) *domain.OpsRun {
	if date == "" {
		date = time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	}
	return j.track(ctx, "reconcile", "", func() (int, string, error) {
		if j.recon == nil {
			return 0, "", domain.ErrNotImplemented
		}
		n := 0
		for _, tenant := range j.tenantIDs(ctx) {
			if _, err := j.recon.Trigger(ctx, tenant, date, "", "", nil, nil); err != nil {
				return n, "date=" + date, err
			}
			n++
		}
		return n, "date=" + date, nil
	})
}

func (j *Jobs) Expire(ctx context.Context) *domain.OpsRun {
	return j.track(ctx, "expire", "", func() (int, string, error) {
		if j.expire == nil {
			return 0, "", nil
		}
		n := 0
		now := time.Now().UTC()
		for _, tenant := range j.tenantIDs(ctx) {
			c, err := j.expire.Run(ctx, tenant, now)
			if err != nil {
				return n, "", err
			}
			n += c
		}
		return n, "", nil
	})
}

func (j *Jobs) ExpirePreview(ctx context.Context, tenantID string, now time.Time) ([]domain.ExpirePreview, error) {
	if j == nil || j.expire == nil {
		return nil, nil
	}
	if tenantID == "" {
		tenantID = j.tenant
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return j.expire.Preview(ctx, tenantID, now)
}

func (j *Jobs) FxFeed(ctx context.Context) *domain.OpsRun {
	return j.track(ctx, "fx_feed", "", func() (int, string, error) {
		if j.fx == nil || len(j.feeds) == 0 {
			return 0, "no feed", nil
		}
		n := 0
		now := time.Now().UTC()
		for _, p := range j.feeds {
			if p.BaseAsset == "" || p.QuoteAsset == "" || p.Rate == "" {
				continue
			}
			tenant := p.TenantID
			if tenant == "" {
				tenant = j.tenant
			}
			err := j.fx.Save(ctx, &domain.FxRate{
				RateID:     "feed:" + tenant + ":" + p.BaseAsset + ":" + p.QuoteAsset,
				TenantID:   tenant,
				BaseAsset:  p.BaseAsset,
				QuoteAsset: p.QuoteAsset,
				Rate:       p.Rate,
				RateSource: "feed",
				QuotedAt:   now,
				CreatedBy:  "worker",
			})
			if err != nil {
				return n, "", err
			}
			n++
		}
		return n, strconv.Itoa(n) + " pairs", nil
	})
}

func (j *Jobs) PurgeIdempotency(ctx context.Context) *domain.OpsRun {
	return j.track(ctx, "idempotency", "", func() (int, string, error) {
		if j.idem == nil {
			return 0, "", nil
		}
		n, err := j.idem.DeleteBefore(ctx, time.Now().UTC().Add(-j.retain))
		return int(n), "", err
	})
}
