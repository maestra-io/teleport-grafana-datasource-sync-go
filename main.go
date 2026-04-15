package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/config"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/detection"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/grafana"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/sync"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/teleport"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	appsFile := envOr("TELEPORT_APPS_FILE", "/shared/apps.json")
	kubeClustersFile := envOr("KUBE_CLUSTERS_FILE", "/shared/kube-clusters.json")

	slog.Info("starting teleport-grafana-datasource-sync",
		"grafana", cfg.GrafanaURL,
		"apps_file", appsFile,
		"kube_clusters_file", kubeClustersFile,
		"api_key_file", cfg.GrafanaAPIKeyFile,
		"interval_secs", cfg.SyncIntervalSecs,
		"dry_run", cfg.DryRun,
		"health_port", 8080,
	)

	// Spawn health endpoint early so K8s probes work during startup.
	go healthServer()

	grafanaClient := grafana.NewClient(cfg.GrafanaURL, cfg.GrafanaAPIKeyFile)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	ticker := time.NewTicker(time.Duration(cfg.SyncIntervalSecs) * time.Second)
	defer ticker.Stop()

	// Run first sync immediately at startup.
	runSyncCycle(ctx, appsFile, kubeClustersFile, grafanaClient, cfg.DryRun)

	for {
		select {
		case <-ctx.Done():
			slog.Info("received shutdown signal, shutting down")
			goto shutdown
		case <-ticker.C:
		}

		runSyncCycle(ctx, appsFile, kubeClustersFile, grafanaClient, cfg.DryRun)
	}

shutdown:
	slog.Info("shutdown complete")
}

func runSyncCycle(ctx context.Context, appsFile, kubeClustersFile string, grafanaClient *grafana.Client, dryRun bool) {
	slog.Info("starting sync cycle")

	stats, err := runSync(ctx, appsFile, kubeClustersFile, grafanaClient, dryRun)
	if err != nil {
		slog.Error("sync cycle failed", "error", err)
		return
	}

	slog.Info("sync cycle complete",
		"created", stats.Created,
		"updated", stats.Updated,
		"deleted", stats.Deleted,
		"unchanged", stats.Unchanged,
		"failed", stats.Failed,
		"dry_run", dryRun,
	)
}

func runSync(ctx context.Context, appsFile, kubeClustersFile string, grafanaClient *grafana.Client, dryRun bool) (*sync.Stats, error) {
	apps, err := teleport.ListApps(ctx, appsFile)
	if err != nil {
		return nil, fmt.Errorf("listing apps: %w", err)
	}

	kubeClusters := teleport.ListKubeClusters(kubeClustersFile)

	detected := make([]detection.DetectedDatasource, 0, len(apps))
	for _, app := range apps {
		if ds, ok := detection.Detect(app); ok {
			detected = append(detected, ds)
		}
	}

	var desired []detection.DetectedDatasource
	var lokiReady bool

	if kubeClusters != nil {
		desired = detection.ExpandLokiTenants(detected, kubeClusters)
		lokiReady = true
	} else {
		for _, d := range detected {
			if d.DSType != detection.Loki {
				desired = append(desired, d)
			}
		}
		lokiReady = false
	}

	kubeClustersCount := -1
	if kubeClusters != nil {
		kubeClustersCount = len(kubeClusters)
	}

	slog.Info("detected monitoring datasources",
		"total_apps", len(apps),
		"matched", len(desired),
		"kube_clusters", kubeClustersCount,
		"loki_ready", lokiReady,
	)

	return sync.Reconcile(ctx, grafanaClient, desired, dryRun, lokiReady)
}

func healthServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "2")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		slog.Error("failed to bind health endpoint on :8080, exiting", "error", err)
		os.Exit(1)
	}
	slog.Info("health endpoint listening on :8080")

	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		slog.Error("health server error", "error", err)
	}
}

func envOr(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}
