package config

import "testing"

func TestValidateForEnvRejectsProdDevSecrets(t *testing.T) {
	cfg := &Config{
		App: AppConfig{Env: "prod"},
		Gateway: GatewayConfig{
			ConsoleToken: "dev-console-token",
			Clients:      []ClientAuth{{ClientID: "order", Secret: "dev-secret-order"}},
		},
	}
	if err := cfg.ValidateForEnv(); err == nil {
		t.Fatal("want reject prod + dev token")
	}
	cfg.Gateway.ConsoleToken = "prod-token"
	if err := cfg.ValidateForEnv(); err == nil {
		t.Fatal("want reject prod + dev client secret")
	}
	cfg.Gateway.Clients[0].Secret = "prod-secret-order"
	if err := cfg.ValidateForEnv(); err != nil {
		t.Fatalf("prod with real secrets: %v", err)
	}
	cfg.Gateway.ConsoleTokens = []ConsoleToken{{Token: "dev-console-readonly", Role: "readonly"}}
	if err := cfg.ValidateForEnv(); err == nil {
		t.Fatal("want reject prod + dev console_tokens")
	}
	cfg.Gateway.ConsoleTokens = []ConsoleToken{{Token: "prod-console-readonly", Role: "readonly"}}
	if err := cfg.ValidateForEnv(); err != nil {
		t.Fatalf("prod named tokens: %v", err)
	}
	cfg.App.Env = "local"
	cfg.Gateway.ConsoleToken = "dev-console-token"
	cfg.Gateway.Clients[0].Secret = "dev-secret-order"
	if err := cfg.ValidateForEnv(); err != nil {
		t.Fatalf("local should allow dev secrets: %v", err)
	}
}

func TestIsProd(t *testing.T) {
	if !IsProd("production") || !IsProd("PROD") {
		t.Fatal("prod aliases")
	}
	if IsProd("local") || IsProd("docker") {
		t.Fatal("non-prod")
	}
}

func TestConsolePrincipals(t *testing.T) {
	g := GatewayConfig{
		ConsoleToken: "dev-console-token",
		ConsoleTokens: []ConsoleToken{
			{Token: "dev-console-readonly", Role: "readonly", Operator: "viewer"},
			{Token: "dev-console-token", Role: "admin"},
		},
	}
	list := g.ConsolePrincipals()
	if len(list) != 2 {
		t.Fatalf("want 2 principals got %d", len(list))
	}
	if list[0].Role != "readonly" || list[1].Role != "admin" {
		t.Fatalf("roles %+v", list)
	}
}
