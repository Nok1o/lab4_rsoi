package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultPort = "8060"

type Config struct {
	HTTPAddress string
	DatabaseURL string
}

func Load() (Config, error) {
	port := envOrDefault("PORT", defaultPort)
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, fmt.Errorf("PORT must be an integer between 1 and 65535")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = buildDatabaseURL()
	}
	if err := validateDatabaseURL(databaseURL); err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddress: ":" + port,
		DatabaseURL: databaseURL,
	}, nil
}

func buildDatabaseURL() string {
	databaseURL := &url.URL{
		Scheme: "postgres",
		Host: envOrDefault("DB_HOST", "localhost") + ":" +
			envOrDefault("DB_PORT", "5432"),
		Path: envOrDefault("DB_NAME", "libraries"),
		User: url.UserPassword(
			envOrDefault("DB_USER", "program"),
			envOrDefault("DB_PASSWORD", "test"),
		),
	}
	query := databaseURL.Query()
	query.Set("sslmode", envOrDefault("DB_SSLMODE", "disable"))
	databaseURL.RawQuery = query.Encode()

	return databaseURL.String()
}

func validateDatabaseURL(databaseURL string) error {
	parsedURL, err := url.ParseRequestURI(databaseURL)
	if err != nil {
		return fmt.Errorf("invalid DATABASE_URL: %w", err)
	}
	if parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql" {
		return fmt.Errorf("DATABASE_URL must use postgres or postgresql scheme")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("DATABASE_URL must contain a host")
	}
	if strings.Trim(parsedURL.Path, "/") == "" {
		return fmt.Errorf("DATABASE_URL must contain a database name")
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	return fallback
}
