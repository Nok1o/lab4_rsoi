package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddress string
	DatabaseURL string
}

func Load() (Config, error) {
	port := envOrDefault("PORT", "8070")
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, fmt.Errorf("PORT must be an integer between 1 and 65535")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		parsedURL := &url.URL{
			Scheme: "postgres",
			Host:   envOrDefault("DB_HOST", "localhost") + ":" + envOrDefault("DB_PORT", "5432"),
			Path:   envOrDefault("DB_NAME", "reservations"),
			User: url.UserPassword(
				envOrDefault("DB_USER", "program"),
				envOrDefault("DB_PASSWORD", "test"),
			),
		}
		query := parsedURL.Query()
		query.Set("sslmode", envOrDefault("DB_SSLMODE", "disable"))
		parsedURL.RawQuery = query.Encode()
		databaseURL = parsedURL.String()
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil || (parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql") || parsedURL.Host == "" || strings.Trim(parsedURL.Path, "/") == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must be a valid postgres URL with host and database name")
	}
	return Config{HTTPAddress: ":" + port, DatabaseURL: databaseURL}, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
