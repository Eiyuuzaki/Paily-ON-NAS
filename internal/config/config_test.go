package config

import (
	"strings"
	"testing"
)

// Load must surface PAILY_AUTH_SUBSCRIBE_TOKEN. Viper's AutomaticEnv only
// answers for keys it already knows, and the shipped examples never mention
// auth.subscribe_token, so without an explicit default the variable was
// silently ignored: the service then ran with an empty token and the
// subscription endpoint fell open. Guard the key's presence, not just the
// parse.
func TestSubscribeTokenReadsFromEnv(t *testing.T) {
	t.Chdir(t.TempDir()) // never pick up a developer's real config.yaml
	t.Setenv("PAILY_AUTH_JWT_SECRET", strings.Repeat("k", 32))
	t.Setenv("PAILY_AUTH_SERVICE_SECRET", "svc-token-value")
	t.Setenv("PAILY_AUTH_ADMIN_PASSWORD", "admin-pass-value")
	t.Setenv("PAILY_AUTH_SUBSCRIBE_TOKEN", "sub-token-from-env")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.SubscribeToken != "sub-token-from-env" {
		t.Fatalf("auth.subscribe_token not read from env, got %q", cfg.Auth.SubscribeToken)
	}
}
