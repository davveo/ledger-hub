package main

import (
	"context"
	"flag"
	"log"
	"time"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/config"
	"github.com/davveo/ledger-hub/internal/connector"
	"github.com/davveo/ledger-hub/internal/domain"
	httpserver "github.com/davveo/ledger-hub/internal/iface/http"
	"github.com/davveo/ledger-hub/internal/infrastructure/logger"
	"github.com/davveo/ledger-hub/internal/infrastructure/persistence"
	"github.com/davveo/ledger-hub/internal/observability"
	"github.com/davveo/ledger-hub/pkg/client"
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
	stop := observability.InitTrace(context.Background(), "ledger-connector")
	defer stop()

	secrets := map[string]string{}
	for _, cl := range cfg.Gateway.Clients {
		secrets[cl.ClientID] = cl.Secret
	}
	orderCli := client.New(cfg.Connector.LedgerBaseURL, "order", secrets["order"]).WithTimeout(10 * time.Second)
	payCli := client.New(cfg.Connector.LedgerBaseURL, "pay", secrets["pay"]).WithTimeout(10 * time.Second)

	var inboxStore domain.InboxRepository = connector.NewMemoryInbox()
	if cluster, err := persistence.OpenCluster(cfg.MySQL); err == nil {
		if cfg.MySQL.AutoMigrate {
			_ = cluster.AutoMigrate()
		}
		inboxStore = persistence.NewInboxRepo(cluster.Primary())
	} else {
		zapLog.Warn("connector inbox db skipped, using memory", zap.Error(err))
	}
	proc := connector.NewProcessor(inboxStore, orderCli, payCli)

	ready := func(ctx context.Context) error {
		if cfg.Connector.LedgerBaseURL == "" {
			return nil
		}
		return observability.HTTPReady(cfg.Connector.LedgerBaseURL+"/livez", time.Second)(ctx)
	}
	h := connector.NewHTTP(proc, orderCli, payCli, ready)

	if cfg.Connector.MQDir != "" {
		go connector.PollFiles(cfg.Connector.MQDir, cfg.Connector.MQInterval, proc, zapLog)
	}
	if kc := connector.NewKafkaConsumer(cfg.Connector.Kafka, proc, zapLog); kc != nil {
		go func() {
			if err := kc.Run(context.Background()); err != nil {
				zapLog.Warn("kafka consumer exit", zap.Error(err))
			}
		}()
	}

	if err := httpserver.Serve(cfg.Connector.Addr, h.Engine(), cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.ShutdownTimeout, zapLog); err != nil {
		zapLog.Fatal("connector exit", zap.Error(err))
	}
}
