package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// FromEnv tests
// ---------------------------------------------------------------------------

func TestFromEnv_Defaults(t *testing.T) {
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GrafanaURL != "http://grafana.grafana.svc:80" {
		t.Errorf("GrafanaURL = %q, want %q", cfg.GrafanaURL, "http://grafana.grafana.svc:80")
	}
	if cfg.GrafanaAPIKeyFile != "/secrets/grafana-api-key" {
		t.Errorf("GrafanaAPIKeyFile = %q, want %q", cfg.GrafanaAPIKeyFile, "/secrets/grafana-api-key")
	}
	if cfg.SyncInterval != 30*time.Second {
		t.Errorf("SyncInterval = %v, want %v", cfg.SyncInterval, 30*time.Second)
	}
	if cfg.DryRun != false {
		t.Errorf("DryRun = %v, want false", cfg.DryRun)
	}
	if cfg.AppsFile != "/shared/apps.json" {
		t.Errorf("AppsFile = %q, want %q", cfg.AppsFile, "/shared/apps.json")
	}
	if cfg.KubeClustersFile != "/shared/kube-clusters.json" {
		t.Errorf("KubeClustersFile = %q, want %q", cfg.KubeClustersFile, "/shared/kube-clusters.json")
	}
}

func TestFromEnv_CustomValues(t *testing.T) {
	t.Setenv("GRAFANA_URL", "https://grafana.example.com")
	t.Setenv("GRAFANA_API_KEY_FILE", "/tmp/my-key")
	t.Setenv("SYNC_INTERVAL_SECS", "120")
	t.Setenv("DRY_RUN", "true")
	t.Setenv("TELEPORT_APPS_FILE", "/data/apps.json")
	t.Setenv("KUBE_CLUSTERS_FILE", "/data/clusters.json")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GrafanaURL != "https://grafana.example.com" {
		t.Errorf("GrafanaURL = %q, want %q", cfg.GrafanaURL, "https://grafana.example.com")
	}
	if cfg.GrafanaAPIKeyFile != "/tmp/my-key" {
		t.Errorf("GrafanaAPIKeyFile = %q, want %q", cfg.GrafanaAPIKeyFile, "/tmp/my-key")
	}
	if cfg.SyncInterval != 120*time.Second {
		t.Errorf("SyncInterval = %v, want %v", cfg.SyncInterval, 120*time.Second)
	}
	if cfg.DryRun != true {
		t.Errorf("DryRun = %v, want true", cfg.DryRun)
	}
	if cfg.AppsFile != "/data/apps.json" {
		t.Errorf("AppsFile = %q, want %q", cfg.AppsFile, "/data/apps.json")
	}
	if cfg.KubeClustersFile != "/data/clusters.json" {
		t.Errorf("KubeClustersFile = %q, want %q", cfg.KubeClustersFile, "/data/clusters.json")
	}
}

func TestFromEnv_InvalidGrafanaURL(t *testing.T) {
	t.Setenv("GRAFANA_URL", "ftp://grafana.example.com")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid GRAFANA_URL, got nil")
	}
	if !strings.Contains(err.Error(), "must start with http:// or https://") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFromEnv_EmptyGrafanaAPIKeyFile(t *testing.T) {
	t.Setenv("GRAFANA_API_KEY_FILE", "")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for empty GRAFANA_API_KEY_FILE, got nil")
	}
	if !strings.Contains(err.Error(), "GRAFANA_API_KEY_FILE is set but empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFromEnv_NonIntegerSyncInterval(t *testing.T) {
	t.Setenv("SYNC_INTERVAL_SECS", "abc")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for non-integer SYNC_INTERVAL_SECS, got nil")
	}
	if !strings.Contains(err.Error(), "invalid SYNC_INTERVAL_SECS") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFromEnv_ZeroSyncInterval(t *testing.T) {
	t.Setenv("SYNC_INTERVAL_SECS", "0")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for zero SYNC_INTERVAL_SECS, got nil")
	}
	if !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFromEnv_NegativeSyncInterval(t *testing.T) {
	t.Setenv("SYNC_INTERVAL_SECS", "-5")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for negative SYNC_INTERVAL_SECS, got nil")
	}
	if !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFromEnv_InvalidDryRun(t *testing.T) {
	t.Setenv("DRY_RUN", "maybe")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for invalid DRY_RUN, got nil")
	}
	if !strings.Contains(err.Error(), "invalid DRY_RUN") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFromEnv_DryRunTrue(t *testing.T) {
	t.Setenv("DRY_RUN", "true")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DryRun {
		t.Error("DryRun = false, want true")
	}
}

func TestFromEnv_DryRun1(t *testing.T) {
	t.Setenv("DRY_RUN", "1")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DryRun {
		t.Error("DryRun = false, want true (from '1')")
	}
}

func TestFromEnv_TrailingSlashTrimmed(t *testing.T) {
	t.Setenv("GRAFANA_URL", "https://grafana.example.com///")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GrafanaURL != "https://grafana.example.com" {
		t.Errorf("GrafanaURL = %q, want trailing slashes trimmed", cfg.GrafanaURL)
	}
}

func TestFromEnv_AppsFileDefault(t *testing.T) {
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppsFile != "/shared/apps.json" {
		t.Errorf("AppsFile = %q, want default %q", cfg.AppsFile, "/shared/apps.json")
	}
}

func TestFromEnv_AppsFileOverride(t *testing.T) {
	t.Setenv("TELEPORT_APPS_FILE", "/custom/apps.json")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppsFile != "/custom/apps.json" {
		t.Errorf("AppsFile = %q, want %q", cfg.AppsFile, "/custom/apps.json")
	}
}

func TestFromEnv_KubeClustersFileDefault(t *testing.T) {
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KubeClustersFile != "/shared/kube-clusters.json" {
		t.Errorf("KubeClustersFile = %q, want default %q", cfg.KubeClustersFile, "/shared/kube-clusters.json")
	}
}

func TestFromEnv_KubeClustersFileOverride(t *testing.T) {
	t.Setenv("KUBE_CLUSTERS_FILE", "/custom/clusters.json")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.KubeClustersFile != "/custom/clusters.json" {
		t.Errorf("KubeClustersFile = %q, want %q", cfg.KubeClustersFile, "/custom/clusters.json")
	}
}

// ---------------------------------------------------------------------------
// ReadGrafanaAPIKey tests
// ---------------------------------------------------------------------------

func TestReadGrafanaAPIKey_FileWithValidKey(t *testing.T) {
	f := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(f, []byte("my-secret-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	key, err := ReadGrafanaAPIKey(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "my-secret-key" {
		t.Errorf("key = %q, want %q", key, "my-secret-key")
	}
}

func TestReadGrafanaAPIKey_FileEmptyOrWhitespace(t *testing.T) {
	f := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(f, []byte("  \n\t "), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ReadGrafanaAPIKey(f)
	if err == nil {
		t.Fatal("expected error for empty/whitespace key file, got nil")
	}
	if !strings.Contains(err.Error(), "grafana API key is empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadGrafanaAPIKey_FileNotFoundEnvSet(t *testing.T) {
	t.Setenv("GRAFANA_API_KEY", "env-key-value")

	key, err := ReadGrafanaAPIKey("/nonexistent/path/to/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key-value" {
		t.Errorf("key = %q, want %q", key, "env-key-value")
	}
}

func TestReadGrafanaAPIKey_FileNotFoundEnvNotSet(t *testing.T) {
	_, err := ReadGrafanaAPIKey("/nonexistent/path/to/key")
	if err == nil {
		t.Fatal("expected error when file not found and env not set, got nil")
	}
	if !strings.Contains(err.Error(), "not found and GRAFANA_API_KEY env var not set") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadGrafanaAPIKey_FileNotFoundEnvEmpty(t *testing.T) {
	t.Setenv("GRAFANA_API_KEY", "   ")

	_, err := ReadGrafanaAPIKey("/nonexistent/path/to/key")
	if err == nil {
		t.Fatal("expected error for empty env var, got nil")
	}
	if !strings.Contains(err.Error(), "grafana API key is empty (from env var") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadGrafanaAPIKey_FileTrimWhitespace(t *testing.T) {
	f := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(f, []byte("  trimmed-key  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	key, err := ReadGrafanaAPIKey(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "trimmed-key" {
		t.Errorf("key = %q, want %q", key, "trimmed-key")
	}
}

func TestReadGrafanaAPIKey_EnvTrimWhitespace(t *testing.T) {
	t.Setenv("GRAFANA_API_KEY", "  env-trimmed  \n")

	key, err := ReadGrafanaAPIKey("/nonexistent/path/to/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-trimmed" {
		t.Errorf("key = %q, want %q", key, "env-trimmed")
	}
}

func TestReadGrafanaAPIKey_FileUnreadable(t *testing.T) {
	// Use a directory as the "file" path — os.ReadFile on a directory returns a
	// non-ErrNotExist error, exercising the permission/unreadable branch.
	dir := t.TempDir()

	_, err := ReadGrafanaAPIKey(dir)
	if err == nil {
		t.Fatal("expected error when reading a directory as file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read API key") {
		t.Errorf("unexpected error message: %v", err)
	}
}
