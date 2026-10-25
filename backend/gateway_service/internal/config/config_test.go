package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"PORT", "LIBRARY_SERVICE_URL", "RATING_SERVICE_URL", "RESERVATION_SERVICE_URL", "REDIS_URL"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddress != ":8080" || cfg.LibraryServiceURL != "http://localhost:8060" || cfg.RatingServiceURL != "http://localhost:8050" || cfg.ReservationServiceURL != "http://localhost:8070" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.RedisURL != "redis://localhost:6379/0" {
		t.Fatalf("RedisURL = %q", cfg.RedisURL)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("PORT", "70000")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid port error")
	}
}
