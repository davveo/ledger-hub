package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/davveo/ledger-hub/internal/config"
)

func New(cfg config.LogConfig) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(strings.ToLower(cfg.Level))); err != nil {
		level = zapcore.InfoLevel
	}
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	var encoder zapcore.Encoder
	if cfg.Encoding == "console" {
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encCfg)
	}
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
	return zap.New(core, zap.AddCaller()), nil
}
