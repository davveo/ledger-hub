package connector

import (
	"context"
	"io"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/config"
)

type kafkaReader struct {
	cfg  config.KafkaConfig
	proc *Processor
	log  *zap.Logger
}

func newKafkaReader(cfg config.KafkaConfig, proc *Processor, log *zap.Logger) *kafkaReader {
	if cfg.Topic == "" {
		cfg.Topic = "ledger-events"
	}
	if cfg.GroupID == "" {
		cfg.GroupID = "ledger-connector"
	}
	return &kafkaReader{cfg: cfg, proc: proc, log: log}
}

func (k *kafkaReader) Run(ctx context.Context) error {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  k.cfg.Brokers,
		Topic:    k.cfg.Topic,
		GroupID:  k.cfg.GroupID,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	defer r.Close()
	k.log.Info("kafka consumer started", zap.Strings("brokers", k.cfg.Brokers), zap.String("topic", k.cfg.Topic))
	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return ctx.Err()
			}
			k.log.Warn("kafka fetch", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}
		id := string(m.Key)
		if id == "" {
			id = k.cfg.Topic + ":" + string(m.Time.UTC().Format("20060102150405.000000000"))
		}
		if _, err := k.proc.Ingest(ctx, id, k.cfg.Topic, 1, m.Value); err != nil {
			k.log.Warn("kafka ingest", zap.Error(err), zap.String("message_id", id))
			continue
		}
		if err := r.CommitMessages(ctx, m); err != nil {
			k.log.Warn("kafka commit", zap.Error(err))
		}
	}
}
