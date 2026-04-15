package reconcile

import (
	"encoding/json"
	"testing"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/detection"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/grafana"
)

func makeDetected(name, tpName string) detection.DetectedDatasource {
	return detection.DetectedDatasource{
		Name:            name,
		UID:             grafana.UIDPrefix + name,
		DSType:          detection.Prometheus,
		URL:             "http://" + tpName,
		TeleportAppName: tpName,
		JSONData:        detection.Prometheus.DefaultJSONData(),
		SecureJSONData:  nil,
	}
}

func makeExisting(name, tpName string) grafana.Datasource {
	return grafana.Datasource{
		UID:              grafana.UIDPrefix + name,
		Name:             name,
		Type:             detection.Prometheus.GrafanaType(),
		URL:              "http://" + tpName,
		Access:           accessProxy,
		JSONData:         detection.Prometheus.DefaultJSONData(),
		SecureJSONFields: map[string]bool{},
	}
}

// --- dedup ---

func TestDedupKeepsFirstByUID(t *testing.T) {
	desired := []detection.DetectedDatasource{
		makeDetected("eu-aws-kube-infra-production-common", "app-a"),
		makeDetected("eu-aws-kube-infra-production-common", "app-b"),
	}
	result := deduplicateDesired(desired)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].TeleportAppName != "app-a" {
		t.Fatalf("expected app-a, got %s", result[0].TeleportAppName)
	}
}

func TestNoDupesAllKeptInOrder(t *testing.T) {
	desired := []detection.DetectedDatasource{
		makeDetected("foo", "foo-thanos-query"),
		makeDetected("bar", "bar-thanos-query"),
	}
	result := deduplicateDesired(desired)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].Name != "foo" {
		t.Fatalf("expected foo, got %s", result[0].Name)
	}
	if result[1].Name != "bar" {
		t.Fatalf("expected bar, got %s", result[1].Name)
	}
}

func TestDedupEmptyInput(t *testing.T) {
	result := deduplicateDesired(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0, got %d", len(result))
	}
}

func TestDedupThreeWayCollision(t *testing.T) {
	desired := []detection.DetectedDatasource{
		makeDetected("same", "app-a"),
		makeDetected("same", "app-b"),
		makeDetected("same", "app-c"),
	}
	result := deduplicateDesired(desired)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].TeleportAppName != "app-a" {
		t.Fatalf("expected app-a, got %s", result[0].TeleportAppName)
	}
}

// --- changedFields ---

func TestChangedFieldsNoneWhenIdentical(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	desired := makeDetected("foo", "foo-thanos-query")
	changes := changedFields(&existing, &desired)
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %v", changes)
	}
}

func TestChangedFieldsDetectsName(t *testing.T) {
	existing := makeExisting("old-name", "foo-thanos-query")
	desired := makeDetected("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "name")
}

func TestChangedFieldsDetectsType(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.Type = "loki"
	desired := makeDetected("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "type")
}

func TestChangedFieldsDetectsURL(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.URL = "https://old.teleport.maestra.io"
	desired := makeDetected("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "url")
}

func TestChangedFieldsURLTrailingSlashIgnored(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.URL = "http://foo-thanos-query/"
	desired := makeDetected("foo", "foo-thanos-query")
	changes := changedFields(&existing, &desired)
	assertNotContains(t, changes, "url")
}

func TestChangedFieldsDetectsAccess(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.Access = "direct"
	desired := makeDetected("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "access")
}

func TestChangedFieldsDetectsJSONData(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.JSONData = map[string]any{"httpMethod": "GET"}
	desired := makeDetected("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "jsonData")
}

func TestChangedFieldsIgnoresExtraGrafanaJSONKeys(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.JSONData = map[string]any{
		"httpMethod":                  "POST",
		"timeInterval":                "15s",
		"exemplarTraceIdDestinations": []any{},
	}
	desired := makeDetected("foo", "foo-thanos-query")
	changes := changedFields(&existing, &desired)
	assertNotContains(t, changes, "jsonData")
}

func TestJSONDataSubsetMatch(t *testing.T) {
	tests := []struct {
		name     string
		existing map[string]any
		desired  map[string]any
		want     bool
	}{
		{
			name:     "superset matches",
			existing: map[string]any{"httpMethod": "POST", "timeInterval": "15s", "extra": true},
			desired:  map[string]any{"httpMethod": "POST", "timeInterval": "15s"},
			want:     true,
		},
		{
			name:     "value mismatch",
			existing: map[string]any{"httpMethod": "GET"},
			desired:  map[string]any{"httpMethod": "POST", "timeInterval": "15s"},
			want:     false,
		},
		{
			name:     "missing key",
			existing: map[string]any{"httpMethod": "POST"},
			desired:  map[string]any{"httpMethod": "POST", "timeInterval": "15s"},
			want:     false,
		},
		{
			name:     "empty desired always matches",
			existing: map[string]any{"anything": true},
			desired:  map[string]any{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonDataMatches(tt.existing, tt.desired)
			if got != tt.want {
				t.Fatalf("jsonDataMatches: got %v, want %v", got, tt.want)
			}
		})
	}
}

// --- valuesEqual ---

func TestValuesEqualIntVsFloat64(t *testing.T) {
	if !valuesEqual(float64(1000), 1000) {
		t.Fatal("expected float64(1000) == int(1000)")
	}
	if !valuesEqual(1000, float64(1000)) {
		t.Fatal("expected int(1000) == float64(1000)")
	}
}

func TestValuesEqualStrings(t *testing.T) {
	if !valuesEqual("POST", "POST") {
		t.Fatal("expected equal strings")
	}
	if valuesEqual("POST", "GET") {
		t.Fatal("expected unequal strings")
	}
}

func TestValuesEqualDifferentTypes(t *testing.T) {
	if valuesEqual("1000", 1000) {
		t.Fatal("expected string != int")
	}
}

func TestValuesEqualJSONNumber(t *testing.T) {
	if !valuesEqual(json.Number("1000"), 1000) {
		t.Fatal("expected json.Number(1000) == int(1000)")
	}
	if !valuesEqual(json.Number("1000"), float64(1000)) {
		t.Fatal("expected json.Number(1000) == float64(1000)")
	}
	if valuesEqual(json.Number("999"), 1000) {
		t.Fatal("expected json.Number(999) != int(1000)")
	}
}

// --- normalizeURL ---

func TestNormalizeURLStripsTrailingSlash(t *testing.T) {
	if normalizeURL("http://example.com/") != "http://example.com" {
		t.Fatal("expected trailing slash stripped")
	}
}

func TestNormalizeURLNoChange(t *testing.T) {
	if normalizeURL("http://example.com") != "http://example.com" {
		t.Fatal("expected no change")
	}
}

// --- secureJsonFields ---

func TestSecureJSONFieldsNoneDesiredAlwaysMatches(t *testing.T) {
	if !secureJSONFieldsMatch(map[string]bool{}, nil) {
		t.Fatal("expected match")
	}
	if !secureJSONFieldsMatch(map[string]bool{"httpHeaderValue1": true}, nil) {
		t.Fatal("expected match")
	}
}

func TestSecureJSONFieldsPresentMatches(t *testing.T) {
	if !secureJSONFieldsMatch(
		map[string]bool{"httpHeaderValue1": true},
		map[string]any{"httpHeaderValue1": "some-tenant"},
	) {
		t.Fatal("expected match")
	}
}

func TestSecureJSONFieldsEmptySkipsCheck(t *testing.T) {
	if !secureJSONFieldsMatch(
		map[string]bool{},
		map[string]any{"httpHeaderValue1": "some-tenant"},
	) {
		t.Fatal("expected match (list API returns empty)")
	}
}

func TestSecureJSONFieldsKeyMissingFromDetailTriggersChange(t *testing.T) {
	if secureJSONFieldsMatch(
		map[string]bool{"otherKey": true},
		map[string]any{"httpHeaderValue1": "some-tenant"},
	) {
		t.Fatal("expected no match")
	}
}

func TestChangedFieldsSecureFieldMissingTriggersUpdate(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.SecureJSONFields = map[string]bool{"otherKey": true}
	desired := makeDetected("foo", "foo-thanos-query")
	desired.SecureJSONData = map[string]any{"httpHeaderValue1": "tenant-a"}
	assertContains(t, changedFields(&existing, &desired), "secureJsonData")
}

func TestChangedFieldsNoSpuriousSecureJSONUpdateFromListAPI(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	desired := makeDetected("foo", "foo-thanos-query")
	desired.SecureJSONData = map[string]any{"httpHeaderValue1": "tenant-a"}
	assertNotContains(t, changedFields(&existing, &desired), "secureJsonData")
}

func TestChangedFieldsNoChangeWhenSecureFieldsSet(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.SecureJSONFields = map[string]bool{"httpHeaderValue1": true}
	desired := makeDetected("foo", "foo-thanos-query")
	desired.SecureJSONData = map[string]any{"httpHeaderValue1": "tenant-a"}
	assertNotContains(t, changedFields(&existing, &desired), "secureJsonData")
}

// helpers

func assertContains(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Fatalf("expected %v to contain %q", slice, want)
}

func assertNotContains(t *testing.T, slice []string, notWant string) {
	t.Helper()
	for _, s := range slice {
		if s == notWant {
			t.Fatalf("expected %v to NOT contain %q", slice, notWant)
		}
	}
}
