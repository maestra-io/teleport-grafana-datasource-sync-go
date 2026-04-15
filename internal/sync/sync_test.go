package sync

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/detection"
	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/grafana"
)

func ds(name, tpName string) detection.DetectedDatasource {
	return detection.MakeDetectedDS(name, tpName)
}

func makeExisting(name, tpName string) grafana.Datasource {
	return detection.MakeExistingDS(name, tpName)
}

// --- dedup ---

func TestDedupKeepsFirstByUID(t *testing.T) {
	desired := []detection.DetectedDatasource{
		ds("eu-aws-kube-infra-production-common", "app-a"),
		ds("eu-aws-kube-infra-production-common", "app-b"),
	}
	result := dedupDesired(desired)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	assertEqual(t, "teleport_app_name", result[0].TeleportAppName, "app-a")
}

func TestNoDupesAllKeptInOrder(t *testing.T) {
	desired := []detection.DetectedDatasource{
		ds("foo", "foo-thanos-query"),
		ds("bar", "bar-thanos-query"),
	}
	result := dedupDesired(desired)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	assertEqual(t, "name[0]", result[0].Name, "foo")
	assertEqual(t, "name[1]", result[1].Name, "bar")
}

func TestDedupEmptyInput(t *testing.T) {
	result := dedupDesired(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0, got %d", len(result))
	}
}

func TestDedupThreeWayCollision(t *testing.T) {
	desired := []detection.DetectedDatasource{
		ds("same", "app-a"),
		ds("same", "app-b"),
		ds("same", "app-c"),
	}
	result := dedupDesired(desired)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	assertEqual(t, "teleport_app_name", result[0].TeleportAppName, "app-a")
}

// --- changedFields ---

func TestChangedFieldsNoneWhenIdentical(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	desired := ds("foo", "foo-thanos-query")
	changes := changedFields(&existing, &desired)
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %v", changes)
	}
}

func TestChangedFieldsDetectsName(t *testing.T) {
	existing := makeExisting("old-name", "foo-thanos-query")
	desired := ds("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "name")
}

func TestChangedFieldsDetectsType(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.Type = "loki"
	desired := ds("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "type")
}

func TestChangedFieldsDetectsURL(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.URL = "https://old.teleport.maestra.io"
	desired := ds("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "url")
}

func TestChangedFieldsDetectsAccess(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.Access = "direct"
	desired := ds("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "access")
}

func TestChangedFieldsDetectsJSONData(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.JSONData = map[string]any{"httpMethod": "GET"}
	desired := ds("foo", "foo-thanos-query")
	assertContains(t, changedFields(&existing, &desired), "jsonData")
}

func TestChangedFieldsIgnoresExtraGrafanaJSONKeys(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	// Simulate Grafana's JSON round-trip with UseNumber.
	raw := `{"httpMethod":"POST","timeInterval":"15s","exemplarTraceIdDestinations":[]}`
	var decoded map[string]any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	_ = dec.Decode(&decoded)
	existing.JSONData = decoded
	desired := ds("foo", "foo-thanos-query")
	changes := changedFields(&existing, &desired)
	assertNotContains(t, changes, "jsonData")
}

func TestJSONDataSubsetMatchWorks(t *testing.T) {
	if !jsonDataMatches(
		map[string]any{"httpMethod": "POST", "timeInterval": "15s", "extra": true},
		map[string]any{"httpMethod": "POST", "timeInterval": "15s"},
	) {
		t.Fatal("expected match")
	}
	if jsonDataMatches(
		map[string]any{"httpMethod": "GET"},
		map[string]any{"httpMethod": "POST", "timeInterval": "15s"},
	) {
		t.Fatal("expected no match")
	}
	// Empty desired always matches.
	if !jsonDataMatches(
		map[string]any{"anything": true},
		map[string]any{},
	) {
		t.Fatal("expected match for empty desired")
	}
}

// --- secureJsonFields ---

func TestSecureJSONFieldsNoneDesiredAlwaysMatches(t *testing.T) {
	if !secureJSONFieldsMatch(map[string]any{}, nil) {
		t.Fatal("expected match")
	}
	if !secureJSONFieldsMatch(map[string]any{"httpHeaderValue1": true}, nil) {
		t.Fatal("expected match")
	}
}

func TestSecureJSONFieldsPresentMatches(t *testing.T) {
	if !secureJSONFieldsMatch(
		map[string]any{"httpHeaderValue1": true},
		map[string]any{"httpHeaderValue1": "some-tenant"},
	) {
		t.Fatal("expected match")
	}
}

func TestSecureJSONFieldsEmptySkipsCheck(t *testing.T) {
	if !secureJSONFieldsMatch(
		map[string]any{},
		map[string]any{"httpHeaderValue1": "some-tenant"},
	) {
		t.Fatal("expected match (list API returns empty)")
	}
}

func TestSecureJSONFieldsKeyMissingFromDetailTriggersChange(t *testing.T) {
	if secureJSONFieldsMatch(
		map[string]any{"otherKey": true},
		map[string]any{"httpHeaderValue1": "some-tenant"},
	) {
		t.Fatal("expected no match")
	}
}

func TestChangedFieldsNoSpuriousSecureJSONUpdateFromListAPI(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	desired := ds("foo", "foo-thanos-query")
	desired.SecureJSONData = map[string]any{"httpHeaderValue1": "tenant-a"}
	assertNotContains(t, changedFields(&existing, &desired), "secureJsonData")
}

func TestChangedFieldsNoChangeWhenSecureFieldsSet(t *testing.T) {
	existing := makeExisting("foo", "foo-thanos-query")
	existing.SecureJSONFields = map[string]any{"httpHeaderValue1": true}
	desired := ds("foo", "foo-thanos-query")
	desired.SecureJSONData = map[string]any{"httpHeaderValue1": "tenant-a"}
	assertNotContains(t, changedFields(&existing, &desired), "secureJsonData")
}

// helpers

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %q, want %q", field, got, want)
	}
}

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

// Unused but keeps the import valid.
var _ = fmt.Sprint
