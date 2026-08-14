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
	cfg    config.WorkerConfig
	log    *zap.Logger
	books  *application.Bookkeeping
	tenant string
	freezes domain.FreezeRepository
}

func New(cfg config.WorkerConfig, log *zap.Logger, books *application.Bookkeeping, freezes domain.FreezeRepository, tenant string) *Runner {
	return &Runner{cfg: cfg, log: log, books: books, freezes: freezes, tenant: tenant}
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
	expireTick := time.NewTicker(expireEvery)
	reconTick := time.NewTicker(reconEvery)
	go func() {
		defer expireTick.Stop()
		defer reconTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-expireTick.C:
				r.releaseExpired(ctx)
			case <-reconTick.C:
				r.log.Info("reconcile tick (Phase 2 placeholder)")
			}
		}
	}()
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
		})
		if err != nil {
			r.log.Warn("auto release freeze failed", zap.String("freeze_id", fz.FreezeID), zap.Error(err))
			continue
		}
		r.log.Info("auto released expired freeze", zap.String("freeze_id", fz.FreezeID))
	}
}
