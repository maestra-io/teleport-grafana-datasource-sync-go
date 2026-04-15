package teleport

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"
)

// App is a simplified Teleport app.
type App struct {
	Name string
}

// tctlAppResource matches `tctl get apps --format=json` output.
type tctlAppResource struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
}

// tctlKubeServerResource matches `tctl get kube_server --format=json` output.
type tctlKubeServerResource struct {
	Spec struct {
		Cluster struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"cluster"`
	} `json:"spec"`
}

// ListApps reads apps from the JSON file written by the tctl sidecar.
// Waits up to 120s for the file to appear on first run.
func ListApps(ctx context.Context, appsFile string) ([]App, error) {
	if err := waitForFile(ctx, appsFile); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(appsFile)
	if err != nil {
		return nil, fmt.Errorf("reading apps from %s: %w", appsFile, err)
	}

	var resources []tctlAppResource
	if err := json.Unmarshal(data, &resources); err != nil {
		return nil, fmt.Errorf("parsing tctl apps JSON: %w", err)
	}

	apps := make([]App, len(resources))
	for i, r := range resources {
		apps[i] = App{Name: r.Metadata.Name}
	}

	slog.Info("loaded apps from tctl sidecar JSON", "count", len(apps))
	return apps, nil
}

// ListKubeClusters reads kube cluster names from the JSON file.
// Returns nil if the file is missing, empty, or unparseable.
func ListKubeClusters(kubeClustersFile string) []string {
	data, err := os.ReadFile(kubeClustersFile)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		slog.Info("kube clusters file not ready, Loki tenant discovery skipped",
			"path", kubeClustersFile)
		return nil
	}

	var resources []tctlKubeServerResource
	if err := json.Unmarshal(data, &resources); err != nil {
		slog.Warn("failed to parse kube clusters JSON, Loki tenant discovery skipped",
			"error", err)
		return nil
	}

	seen := make(map[string]bool)
	var names []string
	for _, r := range resources {
		name := r.Spec.Cluster.Metadata.Name
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)

	slog.Info("loaded kube clusters from tctl sidecar JSON", "count", len(names))
	return names
}

// waitForFile waits for the file to exist with non-zero size.
func waitForFile(ctx context.Context, path string) error {
	const timeout = 120 * time.Second
	const poll = 2 * time.Second

	deadline := time.Now().Add(timeout)

	for {
		info, err := os.Stat(path)
		if err == nil && info.Size() > 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %ds waiting for tctl sidecar to write %s",
				int(timeout.Seconds()), path)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}

		elapsed := time.Since(deadline.Add(-timeout))
		if int(elapsed.Seconds())%10 == 0 {
			slog.Info("waiting for tctl sidecar...",
				"elapsed_secs", int(elapsed.Seconds()),
				"path", path,
			)
		}
	}
}
