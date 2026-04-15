// Package config handles application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the application configuration.
type Config struct {
	GrafanaURL        string
	GrafanaAPIKeyFile string
	SyncInterval      time.Duration
	DryRun            bool
}

// FromEnv loads configuration from environment variables with sensible defaults.
func FromEnv() (*Config, error) {
	grafanaURL := envOr("GRAFANA_URL", "http://grafana.grafana.svc:80")
	if !strings.HasPrefix(grafanaURL, "http://") && !strings.HasPrefix(grafanaURL, "https://") {
		return nil, fmt.Errorf("GRAFANA_URL must start with http:// or https://, got: %s", grafanaURL)
	}

	apiKeyFile := envOr("GRAFANA_API_KEY_FILE", "/secrets/grafana-api-key")
	if apiKeyFile == "" {
		return nil, fmt.Errorf("GRAFANA_API_KEY_FILE must not be empty")
	}

	intervalStr := envOr("SYNC_INTERVAL_SECS", "30")
	intervalSecs, err := strconv.Atoi(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("SYNC_INTERVAL_SECS must be a positive integer: %w", err)
	}
	if intervalSecs <= 0 {
		return nil, fmt.Errorf("SYNC_INTERVAL_SECS must be > 0")
	}

	dryRunStr := envOr("DRY_RUN", "false")
	dryRun, err := strconv.ParseBool(dryRunStr)
	if err != nil {
		return nil, fmt.Errorf("DRY_RUN must be a boolean value: %w", err)
	}

	return &Config{
		GrafanaURL:        strings.TrimRight(grafanaURL, "/"),
		GrafanaAPIKeyFile: apiKeyFile,
		SyncInterval:      time.Duration(intervalSecs) * time.Second,
		DryRun:            dryRun,
	}, nil
}

// ReadGrafanaAPIKey reads the API key from file, falling back to the GRAFANA_API_KEY env var.
func ReadGrafanaAPIKey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		key := strings.TrimSpace(string(data))
		if key == "" {
			return "", fmt.Errorf("grafana API key is empty (from file %s)", path)
		}
		return key, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("failed to read API key from %s: %w", path, err)
	}

	// File not found — fall back to env var.
	val, ok := os.LookupEnv("GRAFANA_API_KEY")
	if !ok {
		return "", fmt.Errorf("API key file %s not found and GRAFANA_API_KEY env var not set", path)
	}

	key := strings.TrimSpace(val)
	if key == "" {
		return "", fmt.Errorf("grafana API key is empty (from env var GRAFANA_API_KEY)")
	}
	return key, nil
}

func envOr(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}
