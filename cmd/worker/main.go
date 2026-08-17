package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/bootstrap"
	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
	httpserver "github.com/davveo/ledger-hub/internal/iface/http"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/internal/infrastructure/persistence"
	"github.com/davveo/ledger-hub/internal/observability"
	"github.com/davveo/ledger-hub/internal/worker"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "config file")
	flag.Parse()

	cfg := config.MustLoad(*cfgPath)
	if err := cfg.ValidateForEnv(); err != nil {
		log.Fatal(err)
	}
	zapLog, err := logger.New(cfg.Log)
	if err != nil {
		log.Fatal(err)
	}
	defer zapLog.Sync()

	cluster, err := persistence.OpenCluster(cfg.MySQL)
	if err != nil {
		zapLog.Fatal("open mysql", zap.Error(err))
	}

	if cfg.MySQL.AutoMigrate {
		if err := cluster.AutoMigrate(); err != nil {
			zapLog.Fatal("auto migrate", zap.Error(err))
		}
	} else {
		zapLog.Info("mysql.auto_migrate=false, expect ledger-migrate to have run")
	}

	stop := observability.InitTrace(context.Background(), "ledger-worker")
	defer stop()

	repos := persistence.NewClusterRepos(cluster)
	tx := persistence.NewClusterTxManager(cluster)
	acl := bootstrap.ACL(cfg.ACL)
	limitRules := bootstrap.Limits(cfg.Limits)
	limiter := application.NewLimiter(limitRules, repos.Limit).WithAlerts(repos.Alert)
	books := application.NewBookkeeping(tx, repos.Asset, repos.Account, repos.Entry, repos.Freeze, repos.Idempotency, acl).
		UsePhase3(repos.Journal, repos.FxRate, repos.ExchangeLeg, limiter, cluster.SameShard).
		WithSaga(repos.Saga)
	recon := application.NewReconcileService(repos.Entry, repos.Account, repos.Freeze, repos.Reconcile).
		UsePhase3(repos.ExchangeLeg, repos.Journal).
		UseFx(repos.FxRate).
		WithOutput(cfg.App.ReconcileDir)
	expire := application.NewExpireEngine(repos.Asset, repos.Account, repos.Entry, books)
	fxSvc := application.NewFxService(repos.FxRate)
	feeds := make([]domain.FxFeedPair, 0, len(cfg.Worker.FxFeed))
	for _, p := range cfg.Worker.FxFeed {
		feeds = append(feeds, domain.FxFeedPair{TenantID: p.TenantID, BaseAsset: p.BaseAsset, QuoteAsset: p.QuoteAsset, Rate: p.Rate})
	}
	instanceID := application.NewInstanceID()
	jobs := application.NewJobs(books, recon, expire, repos.Freeze, cfg.App.DefaultTenant).
		WithFx(fxSvc, feeds).
		WithTenants(repos.Tenant).
		WithIdempotency(repos.Idempotency, cfg.Worker.IdempotencyRetain).
		WithRuns(repos.OpsRun).
		WithAudit(repos.OpsAudit).
		WithInstance(instanceID).
		WithRetry(3, time.Second)
	lease := application.NewLease(repos.Lease, instanceID, cfg.Worker.LeaseTTL)
	run := worker.New(cfg.Worker, zapLog, jobs).
		WithLease(lease).
		WithReady(observability.ClusterReady(cluster))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run.Start(ctx)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		cancel()
	}()

	if err := httpserver.Serve(cfg.HTTP.WorkerAddr, run.Engine(), cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.ShutdownTimeout, zapLog); err != nil {
		zapLog.Fatal("worker exit", zap.Error(err))
	}
}
