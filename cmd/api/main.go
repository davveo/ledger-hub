package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/bootstrap"
	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/domain"
	httpserver "github.com/davveo/ledger-hub/internal/iface/http"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/internal/infrastructure/persistence"
)

func watchConfig(path string, reload func() error, log *zap.Logger) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	var last time.Time
	if fi, err := os.Stat(path); err == nil {
		last = fi.ModTime()
	}
	for range tick.C {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !fi.ModTime().After(last) {
			continue
		}
		last = fi.ModTime()
		if err := reload(); err != nil {
			log.Warn("reload config failed", zap.Error(err))
			continue
		}
		log.Info("acl/limits reloaded from config")
	}
}

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
	assetSvc := application.NewAssetService(repos.Asset)
	accountSvc := application.NewAccountService(repos.Asset, repos.Account)
	books := application.NewBookkeeping(tx, repos.Asset, repos.Account, repos.Entry, repos.Freeze, repos.Idempotency, acl).
		UsePhase3(repos.Journal, repos.FxRate, repos.ExchangeLeg, limiter, cluster.SameShard)
	query := application.NewQueryService(repos.Entry, repos.Freeze).WithJournal(repos.Journal)
	recon := application.NewReconcileService(repos.Entry, repos.Account, repos.Freeze, repos.Reconcile).
		UsePhase3(repos.ExchangeLeg, repos.Journal).
		UseFx(repos.FxRate).
		WithOutput(cfg.App.ReconcileDir)
	fxSvc := application.NewFxService(repos.FxRate)
	tenantSvc := application.NewTenantService(repos.Tenant)
	_ = tenantSvc.Save(context.Background(), &domain.Tenant{
		TenantID: cfg.App.DefaultTenant,
		Name:     "default",
		Status:   "active",
	})
	_ = application.SeedAssets(context.Background(), assetSvc, cfg.App.DefaultTenant)

	reload := func() error {
		fresh, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		acl.Replace(bootstrap.ACL(fresh.ACL).Rules())
		limiter.Replace(bootstrap.Limits(fresh.Limits))
		return nil
	}
	go watchConfig(*cfgPath, reload, zapLog)

	srv := httpserver.New(assetSvc, accountSvc, books, query, recon, cfg.App.DefaultTenant).
		WithPhase3(fxSvc, tenantSvc, limitRules).
		WithOps(acl, limiter, reload)
	if err := httpserver.Serve(cfg.HTTP.APIAddr, srv.Engine(), cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.ShutdownTimeout, zapLog); err != nil {
		zapLog.Fatal("api exit", zap.Error(err))
	}
}
