package application

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/internal/infrastructure/idgen"
	"github.com/davveo/ledger-hub/internal/observability"
)

type Jobs struct {
	books      *Bookkeeping
	recon      *ReconcileService
	expire     *ExpireEngine
	freezes    domain.FreezeRepository
	fx         *FxService
	tenants    domain.TenantRepository
	idem       domain.IdempotencyRepository
	runs       domain.OpsRunRepository
	audit      domain.OpsAuditRepository
	feeds      []domain.FxFeedPair
	tenant     string
	retain     time.Duration
	instanceID string
	maxAttempt int
	retryBase  time.Duration
}

func NewJobs(books *Bookkeeping, recon *ReconcileService, expire *ExpireEngine, freezes domain.FreezeRepository, tenant string) *Jobs {
	return &Jobs{books: books, recon: recon, expire: expire, freezes: freezes, tenant: tenant, retain: 192 * time.Hour, maxAttempt: 1, retryBase: 100 * time.Millisecond}
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

func (j *Jobs) WithInstance(id string) *Jobs {
	j.instanceID = id
	return j
}

func (j *Jobs) WithRetry(max int, base time.Duration) *Jobs {
	if max > 0 {
		j.maxAttempt = max
	}
	j.retryBase = base
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

func (j *Jobs) LastSuccess(ctx context.Context) (map[string]time.Time, error) {
	if j == nil || j.runs == nil {
		return map[string]time.Time{}, nil
	}
	return j.runs.LastSuccess(ctx)
}

func (j *Jobs) ListActions(ctx context.Context, q domain.AuditQuery) ([]*domain.OpsAudit, error) {
	if j == nil || j.audit == nil {
		return []*domain.OpsAudit{}, nil
	}
	return j.audit.List(ctx, q)
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
	max := 1
	if j != nil && j.maxAttempt > 0 {
		max = j.maxAttempt
	}
	base := time.Millisecond
	if j != nil && j.retryBase > 0 {
		base = j.retryBase
	}
	run := &domain.OpsRun{
		RunID:     idgen.New("run_"),
		Name:      name,
		TenantID:  tenant,
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	if j != nil {
		run.InstanceID = j.instanceID
	}
	var n int
	var detail string
	var err error
	start := time.Now()
	for attempt := 1; attempt <= max; attempt++ {
		run.Attempt = attempt
		n, detail, err = fn()
		if err == nil {
			break
		}
		run.LastError = err.Error()
		if attempt < max && base > 0 {
			time.Sleep(BackoffDuration(attempt-1, base, 5*time.Second))
		}
	}
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Count = n
	run.Detail = detail
	run.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		if run.Attempt >= max && max > 1 {
			run.Status = "dead"
		} else {
			run.Status = "failed"
		}
		if run.LastError == "" {
			run.LastError = err.Error()
		}
		if run.Detail == "" {
			run.Detail = err.Error()
		} else if err.Error() != "" && run.Detail != err.Error() {
			run.Detail = run.Detail + "; " + err.Error()
		}
	} else {
		run.Status = "done"
		run.LastError = ""
	}
	if j != nil && j.runs != nil {
		_ = j.runs.Save(ctx, run)
	}
	observability.ObserveWorkerJob(name, run.Status, time.Since(start))
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
			if _, err := j.recon.Enqueue(ctx, tenant, date, "", "", domain.ReconJobTypeDaily, false, nil, nil); err != nil {
				return n, "date=" + date, err
			}
			n++
		}
		d, err := j.recon.DrainQueued(ctx, 50)
		return n + d, "date=" + date, err
	})
}

func (j *Jobs) DrainReconcile(ctx context.Context) *domain.OpsRun {
	return j.track(ctx, "queued-reconcile", "", func() (int, string, error) {
		if j.recon == nil {
			return 0, "", nil
		}
		n, err := j.recon.DrainQueued(ctx, 50)
		return n, "", err
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

func (j *Jobs) ResumeSagas(ctx context.Context) *domain.OpsRun {
	return j.track(ctx, "saga", "", func() (int, string, error) {
		if j.books == nil {
			return 0, "", nil
		}
		n, err := j.books.ResumeOpenSagas(ctx, 50)
		refreshSagaMetrics(ctx, j.books)
		return n, "", err
	})
}

func refreshSagaMetrics(ctx context.Context, books *Bookkeeping) {
	if books == nil {
		return
	}
	list, err := books.ListSagas(ctx, "", "", 200)
	if err != nil {
		return
	}
	oldest := time.Duration(0)
	now := time.Now().UTC()
	n := 0
	for _, sg := range list {
		if sg.Status == domain.SagaCompleted || sg.Status == domain.SagaFailed {
			continue
		}
		n++
		age := now.Sub(sg.CreatedAt)
		if oldest == 0 || age > oldest {
			oldest = age
		}
	}
	observability.SetSagaPending(n, oldest)
}
