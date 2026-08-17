package config

import (
	"fmt"
	"os"
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
	Limits    LimitConfig     `mapstructure:"limits"`
}

type AppConfig struct {
	Name          string `mapstructure:"name"`
	Env           string `mapstructure:"env"`
	DefaultTenant string `mapstructure:"default_tenant"`
	ReconcileDir  string `mapstructure:"reconcile_dir"`
}

type HTTPConfig struct {
	APIAddr         string        `mapstructure:"api_addr"`
	GatewayAddr     string        `mapstructure:"gateway_addr"`
	WorkerAddr      string        `mapstructure:"worker_addr"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type MySQLConfig struct {
	DSN             string        `mapstructure:"dsn"`
	Shards          []string      `mapstructure:"shards"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	AutoMigrate     bool          `mapstructure:"auto_migrate"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

type GatewayConfig struct {
	Upstream           string         `mapstructure:"upstream"`
	RateLimitRPS       int            `mapstructure:"rate_limit_rps"`
	MaxSkewSeconds     int            `mapstructure:"max_skew_seconds"`
	ConsoleToken       string         `mapstructure:"console_token"`
	ConsoleTokens      []ConsoleToken `mapstructure:"console_tokens"`
	DefaultTenant      string         `mapstructure:"default_tenant"`
	AcceptSignVersions []string       `mapstructure:"accept_sign_versions"`
	Clients            []ClientAuth   `mapstructure:"clients"`
}

type ConsoleToken struct {
	Token    string `mapstructure:"token"`
	Role     string `mapstructure:"role"`
	Operator string `mapstructure:"operator"`
}

type ClientAuth struct {
	ClientID     string      `mapstructure:"client_id"`
	Secret       string      `mapstructure:"secret"`
	KeyVersion   string      `mapstructure:"key_version"`
	Keys         []ClientKey `mapstructure:"keys"`
	RateLimitRPS int         `mapstructure:"rate_limit_rps"`
	Tenants      []string    `mapstructure:"tenants"`
}

type ClientKey struct {
	Version string `mapstructure:"version"`
	Secret  string `mapstructure:"secret"`
}

type ACLConfig struct {
	Rules []ACLRuleConfig `mapstructure:"rules"`
}

type ACLRuleConfig struct {
	TenantID     string   `mapstructure:"tenant_id"`
	SourceSystem string   `mapstructure:"source_system"`
	Commands     []string `mapstructure:"commands"`
	Assets       []string `mapstructure:"assets"`
}

type ConnectorConfig struct {
	Addr          string        `mapstructure:"addr"`
	LedgerBaseURL string        `mapstructure:"ledger_base_url"`
	MQDir         string        `mapstructure:"mq_dir"`
	MQInterval    time.Duration `mapstructure:"mq_interval"`
	Kafka         KafkaConfig   `mapstructure:"kafka"`
}

type KafkaConfig struct {
	Brokers []string `mapstructure:"brokers"`
	Topic   string   `mapstructure:"topic"`
	GroupID string   `mapstructure:"group_id"`
}

type WorkerConfig struct {
	FreezeExpireInterval time.Duration `mapstructure:"freeze_expire_interval"`
	ReconcileInterval    time.Duration `mapstructure:"reconcile_interval"`
	AssetExpireInterval  time.Duration `mapstructure:"asset_expire_interval"`
	FxFeedInterval       time.Duration `mapstructure:"fx_feed_interval"`
	IdempotencyInterval  time.Duration `mapstructure:"idempotency_interval"`
	IdempotencyRetain    time.Duration `mapstructure:"idempotency_retain"`
	SagaInterval         time.Duration `mapstructure:"saga_interval"`
	LeaseTTL             time.Duration `mapstructure:"lease_ttl"`
	FxFeed               []FxFeedPair  `mapstructure:"fx_feed"`
}

type FxFeedPair struct {
	TenantID   string `mapstructure:"tenant_id"`
	BaseAsset  string `mapstructure:"base_asset"`
	QuoteAsset string `mapstructure:"quote_asset"`
	Rate       string `mapstructure:"rate"`
}

type LimitConfig struct {
	Rules []LimitRuleConfig `mapstructure:"rules"`
}

type LimitRuleConfig struct {
	TenantID     string `mapstructure:"tenant_id"`
	SourceSystem string `mapstructure:"source_system"`
	AssetCode    string `mapstructure:"asset_code"`
	Command      string `mapstructure:"command"`
	MaxAmount    int64  `mapstructure:"max_amount"`
	DailyAmount  int64  `mapstructure:"daily_amount"`
	DailyCount   int    `mapstructure:"daily_count"`
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
	if !v.IsSet("mysql.auto_migrate") {
		cfg.MySQL.AutoMigrate = !IsProd(cfg.App.Env)
	}
	if cfg.App.DefaultTenant == "" {
		cfg.App.DefaultTenant = "t_default"
	}
	if cfg.App.ReconcileDir == "" {
		cfg.App.ReconcileDir = "data/reconcile"
	}
	if cfg.Gateway.DefaultTenant == "" {
		cfg.Gateway.DefaultTenant = cfg.App.DefaultTenant
	}
	if len(cfg.Gateway.AcceptSignVersions) == 0 {
		cfg.Gateway.AcceptSignVersions = []string{"v1", "v2"}
	}
	if cfg.Worker.SagaInterval <= 0 {
		cfg.Worker.SagaInterval = 15 * time.Second
	}
	if cfg.Worker.LeaseTTL <= 0 {
		cfg.Worker.LeaseTTL = 30 * time.Second
	}
	if cfg.Connector.Addr == "" {
		cfg.Connector.Addr = ":8090"
	}
	if cfg.Connector.LedgerBaseURL == "" {
		cfg.Connector.LedgerBaseURL = "http://127.0.0.1:8088"
	}
	overlaySecrets(cfg)
	return cfg, nil
}

func overlaySecrets(cfg *Config) {
	if cfg == nil {
		return
	}
	if v := os.Getenv("LEDGER_GATEWAY_CONSOLE_TOKEN"); v != "" {
		cfg.Gateway.ConsoleToken = v
	}
	for i, cl := range cfg.Gateway.Clients {
		key := clientSecretEnv(cl.ClientID)
		if v := os.Getenv(key); v != "" {
			cfg.Gateway.Clients[i].Secret = v
		}
	}
}

func clientSecretEnv(clientID string) string {
	id := strings.ToUpper(strings.ReplaceAll(clientID, "-", "_"))
	return "LEDGER_GATEWAY_CLIENT_" + id + "_SECRET"
}

func (g GatewayConfig) ConsolePrincipals() []ConsoleToken {
	out := make([]ConsoleToken, 0, len(g.ConsoleTokens)+1)
	seen := map[string]bool{}
	for _, t := range g.ConsoleTokens {
		tok := strings.TrimSpace(t.Token)
		if tok == "" {
			continue
		}
		t.Token = tok
		if strings.TrimSpace(t.Role) == "" {
			t.Role = "admin"
		}
		if strings.TrimSpace(t.Operator) == "" {
			t.Operator = "console"
		}
		out = append(out, t)
		seen[tok] = true
	}
	if tok := strings.TrimSpace(g.ConsoleToken); tok != "" && !seen[tok] {
		out = append(out, ConsoleToken{Token: tok, Role: "admin", Operator: "console"})
	}
	return out
}

func IsProd(env string) bool {
	e := strings.ToLower(strings.TrimSpace(env))
	return e == "prod" || e == "production"
}

func (c *Config) ValidateForEnv() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if !IsProd(c.App.Env) {
		return nil
	}
	token := strings.TrimSpace(c.Gateway.ConsoleToken)
	if token == "" || token == "dev-console-token" {
		return fmt.Errorf("production 拒绝默认或空的 console_token，请设置 LEDGER_GATEWAY_CONSOLE_TOKEN")
	}
	for _, t := range c.Gateway.ConsoleTokens {
		sec := strings.TrimSpace(t.Token)
		if sec == "" || strings.HasPrefix(sec, "dev-") {
			return fmt.Errorf("production 拒绝空或 dev- 前缀的 console_tokens.token，请替换运营 Token")
		}
	}
	for _, cl := range c.Gateway.Clients {
		sec := strings.TrimSpace(cl.Secret)
		if sec == "" || strings.HasPrefix(sec, "dev-") {
			return fmt.Errorf("production 拒绝空或 dev- 前缀的 client secret（client_id=%s），请设置 %s", cl.ClientID, clientSecretEnv(cl.ClientID))
		}
		for _, k := range cl.Keys {
			ks := strings.TrimSpace(k.Secret)
			if ks == "" || strings.HasPrefix(ks, "dev-") {
				return fmt.Errorf("production 拒绝空或 dev- 前缀的 client key secret（client_id=%s）", cl.ClientID)
			}
		}
	}
	return nil
}

func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}
	return cfg
}
