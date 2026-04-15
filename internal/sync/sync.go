package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/detection"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/grafana"
)

type Stats struct {
	Created   int
	Updated   int
	Deleted   int
	Unchanged int
	Failed    int
}

// Reconcile desired datasources (from Teleport) with actual (from Grafana).
func Reconcile(ctx context.Context, client *grafana.Client, desired []detection.DetectedDatasource, dryRun bool, lokiReady bool) (*Stats, error) {
	desired = dedupDesired(desired)

	actual, err := client.ListManagedDatasources(ctx)
	if err != nil {
		return nil, err
	}

	actualByUID := make(map[string]*grafana.Datasource, len(actual))
	for i := range actual {
		actualByUID[actual[i].UID] = &actual[i]
	}

	desiredUIDs := make(map[string]bool, len(desired))
	for _, d := range desired {
		desiredUIDs[d.UID] = true
	}

	stats := &Stats{}

	for _, ds := range desired {
		req := &grafana.DatasourceRequest{
			Name:           ds.Name,
			UID:            ds.UID,
			Type:           ds.DSType.GrafanaType(),
			URL:            ds.URL,
			Access:         "proxy",
			JSONData:       ds.JSONData,
			SecureJSONData: ds.SecureJSONData,
		}

		if existing, ok := actualByUID[ds.UID]; ok {
			changes := changedFields(existing, &ds)
			if len(changes) > 0 {
				slog.Info("updating datasource",
					"name", ds.Name,
					"uid", ds.UID,
					"teleport_app", ds.TeleportAppName,
					"changed", strings.Join(changes, ", "),
				)
				if !dryRun {
					if err := client.UpdateDatasource(ctx, req); err != nil {
						slog.Error("update failed", "name", ds.Name, "uid", ds.UID, "error", err)
						stats.Failed++
						continue
					}
				}
				stats.Updated++
			} else {
				stats.Unchanged++
			}
		} else {
			slog.Info("creating datasource",
				"name", ds.Name,
				"uid", ds.UID,
				"ds_type", ds.DSType.GrafanaType(),
				"url", ds.URL,
				"teleport_app", ds.TeleportAppName,
			)
			if !dryRun {
				if err := client.CreateDatasource(ctx, req); err != nil {
					slog.Error("create failed", "name", ds.Name, "uid", ds.UID, "error", err)
					stats.Failed++
					continue
				}
			}
			stats.Created++
		}
	}

	for i := range actual {
		existing := &actual[i]
		if desiredUIDs[existing.UID] {
			continue
		}

		// Don't delete Loki datasources when tenant discovery is unavailable.
		if !lokiReady && existing.Type == "loki" {
			slog.Info("preserving Loki datasource (tenant discovery unavailable)",
				"name", existing.Name,
				"uid", existing.UID,
			)
			stats.Unchanged++
			continue
		}

		slog.Info("deleting orphaned datasource",
			"name", existing.Name,
			"uid", existing.UID,
			"ds_type", existing.Type,
			"url", existing.URL,
		)
		if !dryRun {
			if err := client.DeleteDatasource(ctx, existing.UID); err != nil {
				slog.Error("delete failed", "uid", existing.UID, "error", err)
				stats.Failed++
				continue
			}
		}
		stats.Deleted++
	}

	return stats, nil
}

func changedFields(existing *grafana.Datasource, desired *detection.DetectedDatasource) []string {
	var changes []string
	if existing.Name != desired.Name {
		changes = append(changes, "name")
	}
	if existing.Type != desired.DSType.GrafanaType() {
		changes = append(changes, "type")
	}
	if normalizeURL(existing.URL) != normalizeURL(desired.URL) {
		changes = append(changes, "url")
	}
	if existing.Access != "proxy" {
		changes = append(changes, "access")
	}
	if !jsonDataMatches(existing.JSONData, desired.JSONData) {
		changes = append(changes, "jsonData")
	}
	if !secureJSONFieldsMatch(existing.SecureJSONFields, desired.SecureJSONData) {
		changes = append(changes, "secureJsonData")
	}
	return changes
}

func normalizeURL(url string) string {
	return strings.TrimRight(url, "/")
}

// secureJSONFieldsMatch checks that all keys in desired secureJSONData are present
// in Grafana's secureJsonFields. When secureJsonFields is empty (list API doesn't
// return them), the check is skipped to avoid spurious updates.
func secureJSONFieldsMatch(existingFields map[string]any, desiredData map[string]any) bool {
	if desiredData == nil {
		return true
	}
	if len(existingFields) == 0 {
		return true
	}
	for k := range desiredData {
		if existingFields[k] != true {
			return false
		}
	}
	return true
}

// jsonDataMatches checks that all keys in desired exist in existing with the same value.
// Extra keys in existing (added by Grafana) are ignored.
func jsonDataMatches(existing, desired map[string]any) bool {
	for k, dv := range desired {
		ev, ok := existing[k]
		if !ok {
			return false
		}
		if !valuesEqual(ev, dv) {
			return false
		}
	}
	return true
}

// valuesEqual compares two JSON-decoded values, handling json.Number vs float64.
func valuesEqual(a, b any) bool {
	// Normalize both to string for comparison when dealing with JSON number types.
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func dedupDesired(desired []detection.DetectedDatasource) []detection.DetectedDatasource {
	seen := make(map[string]int) // uid -> index in result
	result := make([]detection.DetectedDatasource, 0, len(desired))
	for _, ds := range desired {
		if idx, ok := seen[ds.UID]; ok {
			slog.Warn("duplicate datasource UID, keeping first occurrence",
				"uid", ds.UID,
				"kept_name", result[idx].Name,
				"dup_name", ds.Name,
				"kept_app", result[idx].TeleportAppName,
				"dup_app", ds.TeleportAppName,
			)
		} else {
			seen[ds.UID] = len(result)
			result = append(result, ds)
		}
	}
	return result
}
