package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"PORT",
		"DATABASE_URL",
		"DB_HOST",
		"DB_PORT",
		"DB_NAME",
		"DB_USER",
		"DB_PASSWORD",
		"DB_SSLMODE",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddress != ":8060" {
		t.Fatalf("HTTPAddress = %q, want %q", cfg.HTTPAddress, ":8060")
	}
	if !strings.Contains(cfg.DatabaseURL, "program:test@localhost:5432/libraries") {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if !strings.Contains(cfg.DatabaseURL, "sslmode=disable") {
		t.Fatalf("DatabaseURL = %q, want sslmode=disable", cfg.DatabaseURL)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("PORT", "70000")
	t.Setenv("DATABASE_URL", "postgres://program:test@localhost:5432/libraries")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid port error")
	}
}
