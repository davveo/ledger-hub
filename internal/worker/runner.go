package worker

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
)

type Runner struct {
	cfg     config.WorkerConfig
	log     *zap.Logger
	books   *application.Bookkeeping
	recon   *application.ReconcileService
	expire  *application.ExpireEngine
	fx      *application.FxService
	tenants domain.TenantRepository
	tenant  string
	freezes domain.FreezeRepository
}

func New(cfg config.WorkerConfig, log *zap.Logger, books *application.Bookkeeping, recon *application.ReconcileService, freezes domain.FreezeRepository, tenant string) *Runner {
	return &Runner{cfg: cfg, log: log, books: books, recon: recon, freezes: freezes, tenant: tenant}
}

func (r *Runner) WithExpire(e *application.ExpireEngine) *Runner {
	r.expire = e
	return r
}

func (r *Runner) WithTenants(t domain.TenantRepository) *Runner {
	r.tenants = t
	return r
}

func (r *Runner) WithFxFeed(fx *application.FxService) *Runner {
	r.fx = fx
	return r
}

func (r *Runner) Engine() *gin.Engine {
	e := gin.New()
	e.Use(gin.Recovery())
	e.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ledger-worker"})
	})
	return e
}

func (r *Runner) Start(ctx context.Context) {
	expireEvery := r.cfg.FreezeExpireInterval
	if expireEvery <= 0 {
		expireEvery = 30 * time.Second
	}
	reconEvery := r.cfg.ReconcileInterval
	if reconEvery <= 0 {
		reconEvery = time.Hour
	}
	assetEvery := r.cfg.AssetExpireInterval
	if assetEvery <= 0 {
		assetEvery = 24 * time.Hour
	}
	feedEvery := r.cfg.FxFeedInterval
	if feedEvery <= 0 {
		feedEvery = time.Hour
	}
	expireTick := time.NewTicker(expireEvery)
	reconTick := time.NewTicker(reconEvery)
	assetTick := time.NewTicker(assetEvery)
	feedTick := time.NewTicker(feedEvery)
	go func() {
		defer expireTick.Stop()
		defer reconTick.Stop()
		defer assetTick.Stop()
		defer feedTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-expireTick.C:
				r.releaseExpired(ctx)
			case <-reconTick.C:
				r.runDailyReconcile(ctx)
			case <-assetTick.C:
				r.runAssetExpire(ctx)
			case <-feedTick.C:
				r.runFxFeed(ctx)
			}
		}
	}()
}

func (r *Runner) tenantIDs(ctx context.Context) []string {
	if r.tenants != nil {
		list, err := r.tenants.List(ctx)
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
	if r.tenant != "" {
		return []string{r.tenant}
	}
	return nil
}

func (r *Runner) releaseExpired(ctx context.Context) {
	list, err := r.freezes.ListExpired(ctx, time.Now().UTC(), 100)
	if err != nil {
		r.log.Warn("list expired freeze failed", zap.Error(err))
		return
	}
	for _, fz := range list {
		_, err := r.books.Execute(ctx, domain.CommandRequest{
			Command:      domain.CmdRelease,
			TenantID:     fz.TenantID,
			SourceSystem: "worker",
			BizType:      "freeze_expire",
			BizNo:        "worker:expire:" + fz.FreezeID,
			FreezeID:     fz.FreezeID,
			AssetCode:    fz.AssetCode,
		})
		if err != nil {
			r.log.Warn("auto release freeze failed", zap.String("freeze_id", fz.FreezeID), zap.Error(err))
			continue
		}
		r.log.Info("auto released expired freeze", zap.String("freeze_id", fz.FreezeID))
	}
}

func (r *Runner) runDailyReconcile(ctx context.Context) {
	date := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	for _, tenant := range r.tenantIDs(ctx) {
		rep, err := r.recon.Trigger(ctx, tenant, date, "", "", nil, nil)
		if err != nil {
			r.log.Warn("daily reconcile failed", zap.String("tenant", tenant), zap.String("date", date), zap.Error(err))
			continue
		}
		r.log.Info("daily reconcile done", zap.String("tenant", tenant), zap.String("job_id", rep.Job.JobID), zap.String("date", date))
	}
}

func (r *Runner) runAssetExpire(ctx context.Context) {
	if r.expire == nil {
		return
	}
	for _, tenant := range r.tenantIDs(ctx) {
		n, err := r.expire.Run(ctx, tenant, time.Now().UTC())
		if err != nil {
			r.log.Warn("asset expire failed", zap.String("tenant", tenant), zap.Error(err))
			continue
		}
		if n > 0 {
			r.log.Info("asset expire posted", zap.String("tenant", tenant), zap.Int("accounts", n))
		}
	}
}

func (r *Runner) runFxFeed(ctx context.Context) {
	if r.fx == nil || len(r.cfg.FxFeed) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, p := range r.cfg.FxFeed {
		if p.BaseAsset == "" || p.QuoteAsset == "" || p.Rate == "" {
			continue
		}
		tenant := p.TenantID
		if tenant == "" {
			tenant = r.tenant
		}
		err := r.fx.Save(ctx, &domain.FxRate{
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
			r.log.Warn("fx feed save failed", zap.String("base", p.BaseAsset), zap.String("quote", p.QuoteAsset), zap.Error(err))
			continue
		}
		r.log.Info("fx feed saved", zap.String("base", p.BaseAsset), zap.String("quote", p.QuoteAsset), zap.String("rate", p.Rate))
	}
}
