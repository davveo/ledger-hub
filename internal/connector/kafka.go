package connector

import (
	"context"

	"go.uber.org/zap"

	"github.com/davveo/ledger-hub/internal/config"
)

// Consumer is a plugin-style MQ consumer. File and HTTP remain the default transports.
type Consumer interface {
	Run(ctx context.Context) error
}

// NewKafkaConsumer starts a Kafka reader when connector.kafka.brokers is non-empty.
// Implementation uses github.com/segmentio/kafka-go (Go 1.20 compatible).
func NewKafkaConsumer(cfg config.KafkaConfig, proc *Processor, log *zap.Logger) Consumer {
	if len(cfg.Brokers) == 0 {
		return nil
	}
	return newKafkaReader(cfg, proc, log)
}
