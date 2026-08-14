package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/bootstrap"
	"github.com/davveo/ledger-hub/internal/config"
	httpserver "github.com/davveo/ledger-hub/internal/iface/http"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/internal/infrastructure/persistence"
	"github.com/davveo/ledger-hub/internal/worker"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "config file")
	flag.Parse()

	cfg := config.MustLoad(*cfgPath)
	zapLog, err := logger.New(cfg.Log)
	if err != nil {
		log.Fatal(err)
	}
	defer zapLog.Sync()

	cluster, err := persistence.OpenCluster(cfg.MySQL)
	if err != nil {
		zapLog.Fatal("open mysql", zap.Error(err))
	}

	if err := cluster.AutoMigrate(); err != nil {
		zapLog.Fatal("auto migrate", zap.Error(err))
	}

	repos := persistence.NewClusterRepos(cluster)
	tx := persistence.NewClusterTxManager(cluster)
	acl := bootstrap.ACL(cfg.ACL)
	limitRules := bootstrap.Limits(cfg.Limits)
	limiter := application.NewLimiter(limitRules, repos.Limit)
	books := application.NewBookkeeping(tx, repos.Asset, repos.Account, repos.Entry, repos.Freeze, repos.Idempotency, acl).
		UsePhase3(repos.Journal, repos.FxRate, repos.ExchangeLeg, limiter, cluster.SameShard)
	recon := application.NewReconcileService(repos.Entry, repos.Account, repos.Freeze, repos.Reconcile).
		UsePhase3(repos.ExchangeLeg, repos.Journal).
		UseFx(repos.FxRate).
		WithOutput(cfg.App.ReconcileDir)
	expire := application.NewExpireEngine(repos.Asset, repos.Account, repos.Entry, books)
	fxSvc := application.NewFxService(repos.FxRate)
	run := worker.New(cfg.Worker, zapLog, books, recon, repos.Freeze, cfg.App.DefaultTenant).
		WithExpire(expire).
		WithTenants(repos.Tenant).
		WithFxFeed(fxSvc)

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
