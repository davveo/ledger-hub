package main

import (
	"context"
	"flag"
	"log"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/gateway"
	httpserver "github.com/davveo/ledger-hub/internal/iface/http"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/internal/infrastructure/persistence"
	"github.com/davveo/ledger-hub/internal/observability"
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
	stop := observability.InitTrace(context.Background(), "ledger-gateway")
	defer stop()

	gw, err := gateway.New(cfg.Gateway)
	if err != nil {
		zapLog.Fatal("init gateway", zap.Error(err))
	}
	if cluster, err := persistence.OpenCluster(cfg.MySQL); err == nil {
		if cfg.MySQL.AutoMigrate {
			_ = cluster.AutoMigrate()
		}
		gw.WithAudit(persistence.NewAuditRepo(cluster.Primary())).
			WithNonce(persistence.NewNonceRepo(cluster.Primary()))
	} else {
		zapLog.Warn("gateway audit db skipped", zap.Error(err))
	}
	if err := httpserver.Serve(cfg.HTTP.GatewayAddr, gw.Engine(), cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.ShutdownTimeout, zapLog); err != nil {
		zapLog.Fatal("gateway exit", zap.Error(err))
	}
}
