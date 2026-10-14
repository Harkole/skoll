package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	// Clear relevant env vars
	keys := []string{
		"SKOLL_PORT", "SKOLL_DB_PATH", "SKOLL_PUBLIC_URL", "SKOLL_ADMIN_KEY",
		"DISCORD_BOT_TOKEN", "DISCORD_CLIENT_ID", "DISCORD_CLIENT_SECRET",
		"DISCORD_REDIRECT_URI", "FENRIR_API_URL", "FENRIR_API_KEY",
	}
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	if cfg.Port != ":8081" {
		t.Errorf("expected port :8081, got %s", cfg.Port)
	}
	if cfg.DatabasePath != "skoll.db" {
		t.Errorf("expected db skoll.db, got %s", cfg.DatabasePath)
	}
	if cfg.FenrirAPIURL != "http://localhost:8080" {
		t.Errorf("expected Fenrir URL http://localhost:8080, got %s", cfg.FenrirAPIURL)
	}
	if cfg.HTTPTimeout != 15*time.Second {
		t.Errorf("expected 15s timeout, got %v", cfg.HTTPTimeout)
	}

	if err := cfg.Validate(false); err != nil {
		t.Errorf("non-strict validate should pass: %v", err)
	}

	if err := cfg.Validate(true); err == nil {
		t.Errorf("strict validate should fail when discord/fenrir tokens are missing")
	}
}

func TestConfigCustomEnv(t *testing.T) {
	_ = os.Setenv("SKOLL_PORT", "9090")
	_ = os.Setenv("SKOLL_DB_PATH", "custom.db")
	_ = os.Setenv("DISCORD_BOT_TOKEN", "bot_token_123")
	_ = os.Setenv("DISCORD_CLIENT_ID", "client_123")
	_ = os.Setenv("DISCORD_CLIENT_SECRET", "secret_123")
	_ = os.Setenv("FENRIR_API_URL", "http://fenrir.local:8080")
	_ = os.Setenv("FENRIR_API_KEY", "fenrir_key_xyz")
	defer func() {
		_ = os.Unsetenv("SKOLL_PORT")
		_ = os.Unsetenv("SKOLL_DB_PATH")
		_ = os.Unsetenv("DISCORD_BOT_TOKEN")
		_ = os.Unsetenv("DISCORD_CLIENT_ID")
		_ = os.Unsetenv("DISCORD_CLIENT_SECRET")
		_ = os.Unsetenv("FENRIR_API_URL")
		_ = os.Unsetenv("FENRIR_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	if cfg.Port != ":9090" {
		t.Errorf("expected :9090, got %s", cfg.Port)
	}
	if cfg.DatabasePath != "custom.db" {
		t.Errorf("expected custom.db, got %s", cfg.DatabasePath)
	}
	if err := cfg.Validate(true); err != nil {
		t.Errorf("strict validate should succeed with all keys: %v", err)
	}
}
