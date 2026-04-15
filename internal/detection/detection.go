package detection

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/grafana"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/teleport"
)

// Maximum UID length in Grafana (hard limit enforced by API, returns 400 if exceeded).
const grafanaUIDMaxLen = 40

// DatasourceType represents a Grafana datasource type.
type DatasourceType int

const (
	Prometheus DatasourceType = iota
	VictoriaMetricsMetrics
	VictoriaMetricsLogs
	Loki
)

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

func (t DatasourceType) DefaultJSONData() map[string]any {
	switch t {
	case Prometheus:
		return map[string]any{
			"httpMethod":   "POST",
			"timeInterval": "15s",
		}
	case Loki:
		return map[string]any{
			"maxLines": json.Number("1000"),
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
// Returns the detected datasource and true, or zero value and false if the app doesn't match.
func Detect(app teleport.App) (DetectedDatasource, bool) {
	name := app.Name

	var dsType DatasourceType
	var dsName string

	switch {
	// 1. Prefix rules first (higher priority).
	case strings.HasPrefix(name, "victorialogs-"):
		dsType = VictoriaMetricsLogs
		dsName = name
	// 2. Loki: word-boundary match.
	case isLokiApp(name):
		dsType = Loki
		dsName = name
	// 3. Suffix rules.
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

	// Validate UID characters: Grafana v12+ allows only [a-zA-Z0-9-].
	if !isValidUID(uid) {
		slog.Warn("skipping app: UID contains invalid characters for Grafana",
			"name", name, "uid", uid)
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

// ExpandLokiTenants expands Loki datasources into per-tenant datasources.
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
				slog.Warn("skipping Loki tenant: UID contains invalid characters",
					"tenant", tenant, "uid", uid)
				continue
			}

			result = append(result, DetectedDatasource{
				Name:            name,
				UID:             uid,
				DSType:          Loki,
				URL:             ds.URL,
				TeleportAppName: ds.TeleportAppName,
				JSONData: map[string]any{
					"maxLines":        json.Number("1000"),
					"httpHeaderName1": "X-Scope-OrgID",
				},
				SecureJSONData: map[string]any{
					"httpHeaderValue1": tenant,
				},
			})
		}
	}
	return result
}

// makeUID generates a Grafana UID from a datasource name, respecting the 40-char limit.
func makeUID(name string) string {
	full := grafana.UIDPrefix + name
	if len(full) <= grafanaUIDMaxLen {
		return full
	}

	// FNV-1a 64-bit hash for deterministic short suffix.
	var hash uint64 = 0xcbf29ce484222325
	for i := 0; i < len(name); i++ {
		hash ^= uint64(name[i])
		hash *= 0x100000001b3
	}
	suffix := fmt.Sprintf("%016x", hash)

	// "tp-" (3) + truncated name + "-" + 8-char hash = 40.
	budget := grafanaUIDMaxLen - len(grafana.UIDPrefix) - 1 - 8
	end := budget
	if end > len(name) {
		end = len(name)
	}
	truncated := name[:end]
	return grafana.UIDPrefix + truncated + "-" + suffix[:8]
}

func isLokiApp(name string) bool {
	return name == "loki" ||
		strings.HasPrefix(name, "loki-") ||
		strings.HasSuffix(name, "-loki") ||
		strings.Contains(name, "-loki-")
}

func isValidUID(uid string) bool {
	for i := 0; i < len(uid); i++ {
		b := uid[i]
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-') {
			return false
		}
	}
	return true
}
