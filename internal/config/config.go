package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App       AppConfig       `mapstructure:"app"`
	HTTP      HTTPConfig      `mapstructure:"http"`
	MySQL     MySQLConfig     `mapstructure:"mysql"`
	Log       LogConfig       `mapstructure:"log"`
	Gateway   GatewayConfig   `mapstructure:"gateway"`
	ACL       ACLConfig       `mapstructure:"acl"`
	Connector ConnectorConfig `mapstructure:"connector"`
	Worker    WorkerConfig    `mapstructure:"worker"`
}

type AppConfig struct {
	Name          string `mapstructure:"name"`
	Env           string `mapstructure:"env"`
	DefaultTenant string `mapstructure:"default_tenant"`
}

type HTTPConfig struct {
	APIAddr          string        `mapstructure:"api_addr"`
	GatewayAddr      string        `mapstructure:"gateway_addr"`
	WorkerAddr       string        `mapstructure:"worker_addr"`
	ReadTimeout      time.Duration `mapstructure:"read_timeout"`
	WriteTimeout     time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout  time.Duration `mapstructure:"shutdown_timeout"`
}

type MySQLConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

type GatewayConfig struct {
	Upstream     string       `mapstructure:"upstream"`
	RateLimitRPS int          `mapstructure:"rate_limit_rps"`
	Clients      []ClientAuth `mapstructure:"clients"`
}

type ClientAuth struct {
	ClientID string `mapstructure:"client_id"`
	Secret   string `mapstructure:"secret"`
}

type ACLConfig struct {
	Rules []ACLRuleConfig `mapstructure:"rules"`
}

type ACLRuleConfig struct {
	SourceSystem string   `mapstructure:"source_system"`
	Commands     []string `mapstructure:"commands"`
	Assets       []string `mapstructure:"assets"`
}

type ConnectorConfig struct {
	Addr          string `mapstructure:"addr"`
	LedgerBaseURL string `mapstructure:"ledger_base_url"`
}

type WorkerConfig struct {
	FreezeExpireInterval time.Duration `mapstructure:"freeze_expire_interval"`
	ReconcileInterval    time.Duration `mapstructure:"reconcile_interval"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}
	v.SetEnvPrefix("LEDGER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if cfg.App.DefaultTenant == "" {
		cfg.App.DefaultTenant = "t_default"
	}
	if cfg.Connector.Addr == "" {
		cfg.Connector.Addr = ":8090"
	}
	if cfg.Connector.LedgerBaseURL == "" {
		cfg.Connector.LedgerBaseURL = "http://127.0.0.1:8088"
	}
	return cfg, nil
}

func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}
	return cfg
}
