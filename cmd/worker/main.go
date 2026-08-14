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

	db, err := persistence.Open(cfg.MySQL)
	if err != nil {
		zapLog.Fatal("open mysql", zap.Error(err))
	}

	repos := persistence.NewRepos(db)
	tx := persistence.NewTxManager(db)
	books := application.NewBookkeeping(tx, repos.Asset, repos.Account, repos.Entry, repos.Freeze, repos.Idempotency)
	run := worker.New(cfg.Worker, zapLog, books, repos.Freeze, cfg.App.DefaultTenant)

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
