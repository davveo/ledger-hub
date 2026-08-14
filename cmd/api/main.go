package main

import (
	"flag"
	"log"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/bootstrap"
	"github.com/davveo/ledger-hub/internal/config"
	httpserver "github.com/davveo/ledger-hub/internal/iface/http"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/internal/infrastructure/persistence"
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

	db, err := persistence.Open(cfg.MySQL)
	if err != nil {
		zapLog.Fatal("open mysql", zap.Error(err))
	}
	if err := persistence.AutoMigrate(db); err != nil {
		zapLog.Fatal("auto migrate", zap.Error(err))
	}

	repos := persistence.NewRepos(db)
	tx := persistence.NewTxManager(db)
	acl := bootstrap.ACL(cfg.ACL)
	assetSvc := application.NewAssetService(repos.Asset)
	accountSvc := application.NewAccountService(repos.Asset, repos.Account)
	books := application.NewBookkeeping(tx, repos.Asset, repos.Account, repos.Entry, repos.Freeze, repos.Idempotency, acl)
	query := application.NewQueryService(repos.Entry, repos.Freeze)
	recon := application.NewReconcileService(repos.Entry, repos.Account, repos.Freeze, repos.Reconcile)

	srv := httpserver.New(assetSvc, accountSvc, books, query, recon, cfg.App.DefaultTenant)
	if err := httpserver.Serve(cfg.HTTP.APIAddr, srv.Engine(), cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.ShutdownTimeout, zapLog); err != nil {
		zapLog.Fatal("api exit", zap.Error(err))
	}
}
