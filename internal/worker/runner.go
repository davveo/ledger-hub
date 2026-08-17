package worker

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/config"
)

type Runner struct {
	cfg  config.WorkerConfig
	log  *zap.Logger
	jobs *application.Jobs
}

func New(cfg config.WorkerConfig, log *zap.Logger, jobs *application.Jobs) *Runner {
	return &Runner{cfg: cfg, log: log, jobs: jobs}
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
	idemEvery := r.cfg.IdempotencyInterval
	if idemEvery <= 0 {
		idemEvery = time.Hour
	}
	expireTick := time.NewTicker(expireEvery)
	reconTick := time.NewTicker(reconEvery)
	assetTick := time.NewTicker(assetEvery)
	feedTick := time.NewTicker(feedEvery)
	idemTick := time.NewTicker(idemEvery)
	go func() {
		defer expireTick.Stop()
		defer reconTick.Stop()
		defer assetTick.Stop()
		defer feedTick.Stop()
		defer idemTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-expireTick.C:
				run := r.jobs.ReleaseExpired(ctx)
				r.log.Info("release expired", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
			case <-reconTick.C:
				run := r.jobs.Reconcile(ctx, "")
				r.log.Info("daily reconcile", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
			case <-assetTick.C:
				run := r.jobs.Expire(ctx)
				if run.Count > 0 || run.Status == "failed" {
					r.log.Info("asset expire", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
				}
			case <-feedTick.C:
				run := r.jobs.FxFeed(ctx)
				if run.Count > 0 || run.Status == "failed" {
					r.log.Info("fx feed", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
				}
			case <-idemTick.C:
				run := r.jobs.PurgeIdempotency(ctx)
				if run.Count > 0 || run.Status == "failed" {
					r.log.Info("purge idempotency", zap.String("status", run.Status), zap.Int("count", run.Count))
				}
			}
		}
	}()
}
