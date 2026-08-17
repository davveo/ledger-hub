package main

import (
	"context"
	"flag"
	"log"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/internal/infrastructure/persistence"
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
	if err := persistence.RunMigrations(context.Background(), cluster, zapLog); err != nil {
		zapLog.Fatal("migrate", zap.Error(err))
	}
	zapLog.Info("migrations applied")
}
