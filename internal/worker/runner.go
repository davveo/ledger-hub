package worker

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/observability"
)

type Runner struct {
	cfg     config.WorkerConfig
	log     *zap.Logger
	jobs    *application.Jobs
	lease   *application.Lease
	ready   func(context.Context) error
	service string
}

func New(cfg config.WorkerConfig, log *zap.Logger, jobs *application.Jobs) *Runner {
	return &Runner{cfg: cfg, log: log, jobs: jobs, service: "ledger-worker"}
}

func (r *Runner) WithLease(l *application.Lease) *Runner {
	r.lease = l
	return r
}

func (r *Runner) WithReady(fn func(context.Context) error) *Runner {
	r.ready = fn
	return r
}

func (r *Runner) Engine() *gin.Engine {
	e := gin.New()
	e.Use(gin.Recovery(), observability.HTTPMetrics())
	observability.RegisterProbes(e, r.service, r.ready)
	e.GET("/metrics", observability.MetricsHandler())
	return e
}

func (r *Runner) runJob(ctx context.Context, name string, fn func()) {
	if r.lease != nil && !r.lease.Acquire(ctx, name) {
		r.log.Debug("skip job, not lease holder", zap.String("job", name))
		return
	}
	if r.lease != nil {
		r.lease.Hold(ctx, name, fn)
		return
	}
	fn()
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
	sagaEvery := r.cfg.SagaInterval
	if sagaEvery <= 0 {
		sagaEvery = 15 * time.Second
	}
	drainEvery := 15 * time.Second
	expireTick := time.NewTicker(expireEvery)
	reconTick := time.NewTicker(reconEvery)
	assetTick := time.NewTicker(assetEvery)
	feedTick := time.NewTicker(feedEvery)
	idemTick := time.NewTicker(idemEvery)
	sagaTick := time.NewTicker(sagaEvery)
	drainTick := time.NewTicker(drainEvery)
	go func() {
		defer expireTick.Stop()
		defer reconTick.Stop()
		defer assetTick.Stop()
		defer feedTick.Stop()
		defer idemTick.Stop()
		defer sagaTick.Stop()
		defer drainTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-expireTick.C:
				r.runJob(ctx, "release_expired", func() {
					run := r.jobs.ReleaseExpired(ctx)
					r.log.Info("release expired", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail), zap.String("instance_id", run.InstanceID), zap.Int64("duration_ms", run.DurationMs))
				})
			case <-reconTick.C:
				r.runJob(ctx, "reconcile", func() {
					run := r.jobs.Reconcile(ctx, "")
					r.log.Info("daily reconcile", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
				})
			case <-drainTick.C:
				r.runJob(ctx, "queued-reconcile", func() {
					run := r.jobs.DrainReconcile(ctx)
					if run.Count > 0 || run.Status == "failed" || run.Status == "dead" {
						r.log.Info("drain reconcile", zap.String("status", run.Status), zap.Int("count", run.Count))
					}
				})
			case <-assetTick.C:
				r.runJob(ctx, "expire", func() {
					run := r.jobs.Expire(ctx)
					if run.Count > 0 || run.Status == "failed" {
						r.log.Info("asset expire", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
					}
				})
			case <-feedTick.C:
				r.runJob(ctx, "fx_feed", func() {
					run := r.jobs.FxFeed(ctx)
					if run.Count > 0 || run.Status == "failed" {
						r.log.Info("fx feed", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
					}
				})
			case <-idemTick.C:
				r.runJob(ctx, "idempotency", func() {
					run := r.jobs.PurgeIdempotency(ctx)
					if run.Count > 0 || run.Status == "failed" {
						r.log.Info("purge idempotency", zap.String("status", run.Status), zap.Int("count", run.Count))
					}
				})
			case <-sagaTick.C:
				r.runJob(ctx, "saga", func() {
					run := r.jobs.ResumeSagas(ctx)
					if run.Count > 0 || run.Status == "failed" {
						r.log.Info("resume saga", zap.String("status", run.Status), zap.Int("count", run.Count), zap.String("detail", run.Detail))
					}
				})
			}
		}
	}()
}
