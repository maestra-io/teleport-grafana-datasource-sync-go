package main

import (
	"context"
	"errors"
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
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/reconcile"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/teleport"
)

const healthAddr = "0.0.0.0:8080"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting teleport-grafana-datasource-sync",
		"grafana", cfg.GrafanaURL,
		"apps_file", cfg.AppsFile,
		"kube_clusters_file", cfg.KubeClustersFile,
		"api_key_file", cfg.GrafanaAPIKeyFile,
		"interval", cfg.SyncInterval,
		"dry_run", cfg.DryRun,
		"health_addr", healthAddr,
	)

	// Start health endpoint early so K8s probes work during startup.
	healthServer, healthLn := newHealthServer()
	go func() {
		if err := healthServer.Serve(healthLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("health server error", "error", err)
		}
	}()

	grafanaClient := grafana.NewClient(cfg.GrafanaURL, cfg.GrafanaAPIKeyFile)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	// Run first sync immediately at startup.
	runSyncCycle(ctx, cfg.AppsFile, cfg.KubeClustersFile, grafanaClient, cfg.DryRun)

loop:
	for {
		select {
		case <-ctx.Done():
			slog.Info("received shutdown signal")
			break loop
		case <-ticker.C:
			runSyncCycle(ctx, cfg.AppsFile, cfg.KubeClustersFile, grafanaClient, cfg.DryRun)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = healthServer.Shutdown(shutdownCtx)

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
	)
}

func runSync(ctx context.Context, appsFile, kubeClustersFile string, grafanaClient *grafana.Client, dryRun bool) (*reconcile.Stats, error) {
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

	lokiReady := kubeClusters != nil
	var desired []detection.DetectedDatasource
	if lokiReady {
		desired = detection.ExpandLokiTenants(detected, kubeClusters)
	} else {
		for _, d := range detected {
			if d.DSType != detection.Loki {
				desired = append(desired, d)
			}
		}
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

	return reconcile.Reconcile(ctx, grafanaClient, desired, dryRun, lokiReady)
}

func newHealthServer() (*http.Server, net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "2")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	ln, err := net.Listen("tcp", healthAddr)
	if err != nil {
		slog.Error("failed to bind health endpoint", "addr", healthAddr, "error", err)
		os.Exit(1)
	}
	slog.Info("health endpoint listening", "addr", healthAddr)

	return &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}, ln
}
