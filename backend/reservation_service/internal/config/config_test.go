package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"PORT", "DATABASE_URL", "DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSLMODE"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddress != ":8070" || !strings.Contains(cfg.DatabaseURL, "/reservations?") {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Setenv("PORT", "70000")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid port error")
	}
	t.Setenv("PORT", "8070")
	t.Setenv("DATABASE_URL", "postgres://localhost")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid database URL error")
	}
}
