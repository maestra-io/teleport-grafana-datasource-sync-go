// Package detection implements rules for identifying monitoring apps
// among Teleport applications and mapping them to Grafana datasource types.
package detection

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/grafana"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/teleport"
)

// grafanaUIDMaxLen is the maximum UID length enforced by the Grafana API.
const grafanaUIDMaxLen = 40

// DatasourceType represents a Grafana datasource type.
type DatasourceType int

const (
	// Prometheus datasource backed by a Thanos Query endpoint.
	Prometheus DatasourceType = iota
	// VictoriaMetricsMetrics datasource backed by a VMAuth endpoint.
	VictoriaMetricsMetrics
	// VictoriaMetricsLogs datasource backed by a VictoriaLogs endpoint.
	VictoriaMetricsLogs
	// Loki datasource, potentially expanded into per-tenant datasources.
	Loki
)

// GrafanaType returns the Grafana plugin type identifier.
func (t DatasourceType) GrafanaType() string {
	switch t {
	case Prometheus:
		return "prometheus"
	case VictoriaMetricsMetrics:
		return "victoriametrics-metrics-datasource"
	case VictoriaMetricsLogs:
		return "victoriametrics-logs-datasource"
	case Loki:
		return "loki"
	default:
		return "unknown"
	}
}

// String implements fmt.Stringer.
func (t DatasourceType) String() string {
	return t.GrafanaType()
}

// DefaultJSONData returns the default jsonData for this datasource type.
func (t DatasourceType) DefaultJSONData() map[string]any {
	switch t {
	case Prometheus:
		return map[string]any{
			"httpMethod":   "POST",
			"timeInterval": "15s",
		}
	case Loki:
		return map[string]any{
			"maxLines": 1000,
		}
	default:
		return map[string]any{}
	}
}

// DetectedDatasource is the result of detecting a monitoring app.
type DetectedDatasource struct {
	Name            string
	UID             string
	DSType          DatasourceType
	URL             string
	TeleportAppName string
	JSONData        map[string]any
	SecureJSONData  map[string]any // nil means no secure data
}

// Detect applies detection rules to a Teleport app.
//
// Rule priority (highest first):
//  1. victorialogs- prefix → VictoriaMetricsLogs
//  2. loki word-boundary match → Loki
//  3. -thanos-query suffix → Prometheus (suffix stripped from name)
//  4. -vmauth suffix → VictoriaMetricsMetrics (suffix stripped from name)
//
// Returns the detected datasource and true, or zero value and false if no rule matches.
func Detect(app teleport.App) (DetectedDatasource, bool) {
	name := app.Name

	var dsType DatasourceType
	var dsName string

	switch {
	case strings.HasPrefix(name, "victorialogs-"):
		dsType = VictoriaMetricsLogs
		dsName = name
	case isLokiApp(name):
		dsType = Loki
		dsName = name
	case strings.HasSuffix(name, "-thanos-query"):
		stripped := strings.TrimSuffix(name, "-thanos-query")
		if stripped == "" {
			return DetectedDatasource{}, false
		}
		dsType = Prometheus
		dsName = stripped
	case strings.HasSuffix(name, "-vmauth"):
		stripped := strings.TrimSuffix(name, "-vmauth")
		if stripped == "" {
			return DetectedDatasource{}, false
		}
		dsType = VictoriaMetricsMetrics
		dsName = stripped
	default:
		return DetectedDatasource{}, false
	}

	url := "http://" + name
	uid := makeUID(dsName)

	if !isValidUID(uid) {
		slog.Warn("skipping app with invalid UID characters",
			"name", name, "uid", uid, "reason", "invalid characters for Grafana")
		return DetectedDatasource{}, false
	}

	return DetectedDatasource{
		Name:            dsName,
		UID:             uid,
		DSType:          dsType,
		URL:             url,
		TeleportAppName: name,
		JSONData:        dsType.DefaultJSONData(),
		SecureJSONData:  nil,
	}, true
}

// ExpandLokiTenants expands Loki datasources into per-tenant datasources
// using Teleport kube cluster names. The original Loki datasource is replaced
// by one datasource per tenant. Non-Loki datasources pass through unchanged.
func ExpandLokiTenants(datasources []DetectedDatasource, kubeClusters []string) []DetectedDatasource {
	if len(kubeClusters) == 0 {
		return datasources
	}

	result := make([]DetectedDatasource, 0, len(datasources))
	for _, ds := range datasources {
		if ds.DSType != Loki {
			result = append(result, ds)
			continue
		}

		for _, tenant := range kubeClusters {
			name := tenant + "-loki"
			uid := makeUID(name)

			if !isValidUID(uid) {
				slog.Warn("skipping Loki tenant with invalid UID characters",
					"tenant", tenant, "uid", uid, "reason", "invalid characters for Grafana")
				continue
			}

			jsonData := Loki.DefaultJSONData()
			jsonData["httpHeaderName1"] = "X-Scope-OrgID"

			result = append(result, DetectedDatasource{
				Name:            name,
				UID:             uid,
				DSType:          Loki,
				URL:             ds.URL,
				TeleportAppName: ds.TeleportAppName,
				JSONData:        jsonData,
				SecureJSONData: map[string]any{
					"httpHeaderValue1": tenant,
				},
			})
		}
	}
	return result
}

// makeUID generates a Grafana UID from a datasource name, respecting the 40-char limit.
// If the full UID fits, it is used directly. Otherwise the name is truncated and an
// 8-character FNV-1a hash suffix is appended for deterministic collision resistance.
func makeUID(name string) string {
	full := grafana.UIDPrefix + name
	if len(full) <= grafanaUIDMaxLen {
		return full
	}

	h := fnv.New64a()
	h.Write([]byte(name))
	suffix := fmt.Sprintf("%016x", h.Sum64())

	// UIDPrefix + truncated name + "-" + 8-char hash = grafanaUIDMaxLen
	budget := grafanaUIDMaxLen - len(grafana.UIDPrefix) - 1 - 8
	end := budget
	if end > len(name) {
		end = len(name)
	}
	return grafana.UIDPrefix + name[:end] + "-" + suffix[8:]
}

// isLokiApp returns true if the name matches "loki" at a word boundary:
// exact "loki", "loki-*" prefix, "*-loki" suffix, or "*-loki-*" infix.
func isLokiApp(name string) bool {
	return name == "loki" ||
		strings.HasPrefix(name, "loki-") ||
		strings.HasSuffix(name, "-loki") ||
		strings.Contains(name, "-loki-")
}

// isValidUID checks that the UID contains only Grafana-allowed characters [a-zA-Z0-9-].
func isValidUID(uid string) bool {
	if len(uid) == 0 {
		return false
	}
	for i := 0; i < len(uid); i++ {
		b := uid[i]
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-') {
			return false
		}
	}
	return true
}
