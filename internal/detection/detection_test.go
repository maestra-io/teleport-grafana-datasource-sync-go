package detection

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/teleport"
)

func app(name string) teleport.App {
	return teleport.App{Name: name}
}

// --- Thanos Query ---

func TestThanosQueryDetectedStrippedAndLegacyPrefixed(t *testing.T) {
	ds, ok := Detect(app("eu-aws-kube-infra-production-common-thanos-query"))
	if !ok {
		t.Fatal("expected detection")
	}
	// Display name carries the legacy_ prefix so dashboards can filter Thanos
	// datasources out of variable dropdowns during the VM migration.
	assertEqual(t, "name", ds.Name, "legacy_eu-aws-kube-infra-production-common")
	// UID is derived from the un-prefixed name so existing panel references stay valid.
	assertEqual(t, "uid", ds.UID, "tp-eu-aws-kube-infra-production-common")
	assertDSType(t, ds.DSType, Prometheus)
	assertEqual(t, "url", ds.URL, "http://eu-aws-kube-infra-production-common-thanos-query")
	assertEqual(t, "teleport_app_name", ds.TeleportAppName, "eu-aws-kube-infra-production-common-thanos-query")
}

// VMAuth-backed prometheus datasources are part of the VM stack we migrate *to*
// and must NOT receive the legacy_ prefix.
func TestVmauthNotLegacyPrefixed(t *testing.T) {
	ds, ok := Detect(app("vmcluster-foo-vmauth"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertEqual(t, "name", ds.Name, "vmcluster-foo")
	assertEqual(t, "uid", ds.UID, "tp-vmcluster-foo")
	assertDSType(t, ds.DSType, Prometheus)
}

// --- VMAuth ---

func TestVmauthDetectedAsPrometheusAndStripped(t *testing.T) {
	ds, ok := Detect(app("us-omicron-lw-kube-common-vmauth"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertEqual(t, "name", ds.Name, "us-omicron-lw-kube-common")
	assertEqual(t, "uid", ds.UID, "tp-us-omicron-lw-kube-common")
	assertDSType(t, ds.DSType, Prometheus)
	assertEqual(t, "teleport_app_name", ds.TeleportAppName, "us-omicron-lw-kube-common-vmauth")
	assertEqual(t, "url", ds.URL, "http://us-omicron-lw-kube-common-vmauth")
}

// --- VictoriaLogs ---

func TestVictorialogsDetectedAsIs(t *testing.T) {
	ds, ok := Detect(app("victorialogs-siem"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertEqual(t, "name", ds.Name, "victorialogs-siem")
	assertEqual(t, "uid", ds.UID, "tp-victorialogs-siem")
	assertDSType(t, ds.DSType, VictoriaMetricsLogs)
	assertEqual(t, "teleport_app_name", ds.TeleportAppName, "victorialogs-siem")
	assertEqual(t, "url", ds.URL, "http://victorialogs-siem")
}

func TestVictorialogsNotMistakenForThanos(t *testing.T) {
	ds, ok := Detect(app("victorialogs-eu-thanos-query"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertDSType(t, ds.DSType, VictoriaMetricsLogs)
	assertEqual(t, "name", ds.Name, "victorialogs-eu-thanos-query")
}

func TestVictorialogsNotMistakenForVmauth(t *testing.T) {
	ds, ok := Detect(app("victorialogs-foo-vmauth"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertDSType(t, ds.DSType, VictoriaMetricsLogs)
	assertEqual(t, "name", ds.Name, "victorialogs-foo-vmauth")
}

// --- Loki ---

func TestLokiDetectedAsIs(t *testing.T) {
	ds, ok := Detect(app("eu-omega-loki-distributed"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertEqual(t, "name", ds.Name, "eu-omega-loki-distributed")
	assertEqual(t, "uid", ds.UID, "tp-eu-omega-loki-distributed")
	assertDSType(t, ds.DSType, Loki)
	assertEqual(t, "teleport_app_name", ds.TeleportAppName, "eu-omega-loki-distributed")
	assertEqual(t, "url", ds.URL, "http://eu-omega-loki-distributed")
}

func TestLokiExactName(t *testing.T) {
	ds, ok := Detect(app("loki"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertDSType(t, ds.DSType, Loki)
}

func TestLokiPrefix(t *testing.T) {
	ds, ok := Detect(app("loki-gateway"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertDSType(t, ds.DSType, Loki)
}

func TestLokiSuffix(t *testing.T) {
	ds, ok := Detect(app("eu-omega-loki"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertDSType(t, ds.DSType, Loki)
}

func TestLokiSubstringNoBoundaryRejected(t *testing.T) {
	_, ok := Detect(app("lokibridge"))
	if ok {
		t.Fatal("expected rejection")
	}
}

func TestLokiTakesPriorityOverThanosSuffix(t *testing.T) {
	ds, ok := Detect(app("loki-region-thanos-query"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertDSType(t, ds.DSType, Loki)
	assertEqual(t, "name", ds.Name, "loki-region-thanos-query")
}

func TestLokiTakesPriorityOverVmauthSuffix(t *testing.T) {
	ds, ok := Detect(app("loki-foo-vmauth"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertDSType(t, ds.DSType, Loki)
	assertEqual(t, "name", ds.Name, "loki-foo-vmauth")
}

func TestLokiSuffixSubstringNoBoundaryRejected(t *testing.T) {
	_, ok := Detect(app("havaloki"))
	if ok {
		t.Fatal("expected rejection")
	}
}

// --- Ignored apps ---

func TestIgnoredApps(t *testing.T) {
	ignored := []string{
		"eu-aws-kube-infra-production-common-prometheus",
		"eu-aws-kube-infra-production-common-thanos-compactor",
		"-thanos-query",
		"-vmauth",
		"vmagent-vmks-victoria-metrics-us-omicron-lw-kube-common",
		"vmalert-vmks-victoria-metrics-us-omicron-lw-kube-common",
		"auth-jwt-omega",
		"",
	}
	for _, name := range ignored {
		testName := name
		if testName == "" {
			testName = "empty_string"
		}
		t.Run(testName, func(t *testing.T) {
			_, ok := Detect(app(name))
			if ok {
				t.Fatalf("expected app %q to be ignored", name)
			}
		})
	}
}

// --- URL handling ---

func TestURLIsHTTPAppName(t *testing.T) {
	ds, ok := Detect(app("victorialogs-test"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertEqual(t, "url", ds.URL, "http://victorialogs-test")
}

// --- UID validation ---

func TestUIDShortNameExact(t *testing.T) {
	longName := "victorialogs-" + strings.Repeat("a", 24) // 37 chars
	ds, ok := Detect(app(longName))
	if !ok {
		t.Fatal("expected detection")
	}
	assertEqual(t, "uid", ds.UID, "tp-"+longName)
	if len(ds.UID) != 40 {
		t.Fatalf("expected UID len 40, got %d", len(ds.UID))
	}
}

func TestUIDLongNameGetsTruncatedWithHash(t *testing.T) {
	longName := "victorialogs-" + strings.Repeat("a", 25) // 38 chars
	ds, ok := Detect(app(longName))
	if !ok {
		t.Fatal("expected detection")
	}
	if len(ds.UID) != 40 {
		t.Fatalf("expected UID len 40, got %d", len(ds.UID))
	}
	if !strings.HasPrefix(ds.UID, "tp-") {
		t.Fatalf("expected UID to start with tp-, got %s", ds.UID)
	}
	// UID is deterministic
	ds2, _ := Detect(app(longName))
	if ds.UID != ds2.UID {
		t.Fatalf("UID not deterministic: %s vs %s", ds.UID, ds2.UID)
	}
	// Verify separator and hex suffix
	if ds.UID[len(ds.UID)-9] != '-' {
		t.Fatalf("expected '-' separator at position %d, got %q", len(ds.UID)-9, ds.UID[len(ds.UID)-9])
	}
	hexSuffix := ds.UID[len(ds.UID)-8:]
	for _, c := range hexSuffix {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("expected hex char in suffix, got %q in UID %q", c, ds.UID)
		}
	}
}

func TestUIDLongNamesDiffer(t *testing.T) {
	ds1, _ := Detect(app("victorialogs-" + strings.Repeat("a", 25)))
	ds2, _ := Detect(app("victorialogs-" + strings.Repeat("b", 25)))
	if ds1.UID == ds2.UID {
		t.Fatalf("expected different UIDs for different names, both got %s", ds1.UID)
	}
}

func TestUIDLongLeasewebNameAccepted(t *testing.T) {
	ds, ok := Detect(app("eu-leaseweb-kube-cdp-common-production-01-cdp-thanos-query"))
	if !ok {
		t.Fatal("expected detection")
	}
	assertEqual(t, "name", ds.Name, "legacy_eu-leaseweb-kube-cdp-common-production-01-cdp")
	if len(ds.UID) > grafanaUIDMaxLen {
		t.Fatalf("UID too long: %d > %d", len(ds.UID), grafanaUIDMaxLen)
	}
}

func TestUIDInvalidCharsRejected(t *testing.T) {
	invalid := []string{
		"victorialogs-foo@bar",
		"victorialogs-foo_bar",
		"victorialogs-foo.bar",
		"victorialogs-foo bar",
	}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			_, ok := Detect(app(name))
			if ok {
				t.Fatalf("expected app %q to be rejected (invalid UID chars)", name)
			}
		})
	}
}

func TestIsValidUIDRejectsEmpty(t *testing.T) {
	if isValidUID("") {
		t.Fatal("expected empty UID to be invalid")
	}
}

// --- json_data per type ---

func TestDefaultJSONDataPrometheus(t *testing.T) {
	data := Prometheus.DefaultJSONData()
	if data["httpMethod"] != "POST" {
		t.Fatalf("expected POST, got %v", data["httpMethod"])
	}
	if data["timeInterval"] != "15s" {
		t.Fatalf("expected 15s, got %v", data["timeInterval"])
	}
}

func TestDefaultJSONDataLoki(t *testing.T) {
	data := Loki.DefaultJSONData()
	if data["maxLines"] != 1000 {
		t.Fatalf("expected 1000, got %v", data["maxLines"])
	}
}

func TestDefaultJSONDataVMMetricsEmpty(t *testing.T) {
	data := VictoriaMetricsMetrics.DefaultJSONData()
	if len(data) != 0 {
		t.Fatalf("expected empty map, got %v", data)
	}
}

func TestDefaultJSONDataVMLogsEmpty(t *testing.T) {
	data := VictoriaMetricsLogs.DefaultJSONData()
	if len(data) != 0 {
		t.Fatalf("expected empty map, got %v", data)
	}
}

// --- as_grafana_type ---

func TestGrafanaTypeStrings(t *testing.T) {
	tests := []struct {
		dsType DatasourceType
		want   string
	}{
		{Prometheus, "prometheus"},
		{VictoriaMetricsMetrics, "victoriametrics-metrics-datasource"},
		{VictoriaMetricsLogs, "victoriametrics-logs-datasource"},
		{Loki, "loki"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.dsType.GrafanaType(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- expand_loki_tenants ---

func TestExpandLokiNoClustersPassthrough(t *testing.T) {
	ds, ok := Detect(app("eu-omega-loki-distributed"))
	if !ok {
		t.Fatal("expected detection")
	}
	result := ExpandLokiTenants([]DetectedDatasource{ds}, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	assertEqual(t, "name", result[0].Name, "eu-omega-loki-distributed")
}

func TestExpandLokiCreatesPerTenantDatasources(t *testing.T) {
	ds, ok := Detect(app("eu-omega-loki-distributed"))
	if !ok {
		t.Fatal("expected detection")
	}
	clusters := []string{"eu-aws-kube-infra-production", "eu-aws-kube-common-production"}
	result := ExpandLokiTenants([]DetectedDatasource{ds}, clusters)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	assertEqual(t, "name[0]", result[0].Name, "eu-aws-kube-infra-production-loki")
	assertEqual(t, "uid[0]", result[0].UID, "tp-eu-aws-kube-infra-production-loki")
	assertDSType(t, result[0].DSType, Loki)
	assertEqual(t, "url[0]", result[0].URL, "http://eu-omega-loki-distributed")
	assertEqual(t, "httpHeaderName1", fmt.Sprint(result[0].JSONData["httpHeaderName1"]), "X-Scope-OrgID")
	assertEqual(t, "httpHeaderValue1", fmt.Sprint(result[0].SecureJSONData["httpHeaderValue1"]), "eu-aws-kube-infra-production")
	assertEqual(t, "name[1]", result[1].Name, "eu-aws-kube-common-production-loki")
	assertEqual(t, "teleportAppName[0]", result[0].TeleportAppName, "eu-omega-loki-distributed")
}

func TestExpandLokiPreservesNonLoki(t *testing.T) {
	prom, ok := Detect(app("eu-aws-kube-infra-production-common-thanos-query"))
	if !ok {
		t.Fatal("expected detection")
	}
	loki, ok := Detect(app("eu-omega-loki-distributed"))
	if !ok {
		t.Fatal("expected detection")
	}
	clusters := []string{"tenant-a"}
	result := ExpandLokiTenants([]DetectedDatasource{prom, loki}, clusters)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	assertDSType(t, result[0].DSType, Prometheus)
	assertDSType(t, result[1].DSType, Loki)
	assertEqual(t, "name[1]", result[1].Name, "tenant-a-loki")
}

func TestExpandLokiTenantDSHasCorrectJSONData(t *testing.T) {
	ds, ok := Detect(app("eu-omega-loki-distributed"))
	if !ok {
		t.Fatal("expected detection")
	}
	clusters := []string{"my-cluster"}
	result := ExpandLokiTenants([]DetectedDatasource{ds}, clusters)
	assertEqual(t, "name", result[0].Name, "my-cluster-loki")
	assertEqual(t, "uid", result[0].UID, "tp-my-cluster-loki")
	if result[0].JSONData["maxLines"] != 1000 {
		t.Fatalf("expected maxLines=1000, got %v", result[0].JSONData["maxLines"])
	}
	assertEqual(t, "httpHeaderName1", fmt.Sprint(result[0].JSONData["httpHeaderName1"]), "X-Scope-OrgID")
	if result[0].SecureJSONData == nil {
		t.Fatal("expected non-nil SecureJSONData")
	}
	assertEqual(t, "httpHeaderValue1", fmt.Sprint(result[0].SecureJSONData["httpHeaderValue1"]), "my-cluster")
}

func TestExpandLokiEmptyClusterSlicePassthrough(t *testing.T) {
	ds, ok := Detect(app("eu-omega-loki-distributed"))
	if !ok {
		t.Fatal("expected detection")
	}
	result := ExpandLokiTenants([]DetectedDatasource{ds}, []string{})
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	assertEqual(t, "name", result[0].Name, "eu-omega-loki-distributed")
}

func TestExpandLokiSkipsInvalidTenantUID(t *testing.T) {
	ds, ok := Detect(app("eu-omega-loki-distributed"))
	if !ok {
		t.Fatal("expected detection")
	}
	// tenant with underscore produces invalid UID
	clusters := []string{"valid-tenant", "bad_tenant"}
	result := ExpandLokiTenants([]DetectedDatasource{ds}, clusters)
	if len(result) != 1 {
		t.Fatalf("expected 1 (invalid tenant skipped), got %d", len(result))
	}
	assertEqual(t, "name", result[0].Name, "valid-tenant-loki")
}

// helpers

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %q, want %q", field, got, want)
	}
}

func assertDSType(t *testing.T, got, want DatasourceType) {
	t.Helper()
	if got != want {
		t.Fatalf("ds_type: got %s, want %s", got.GrafanaType(), want.GrafanaType())
	}
}

func TestGrafanaTypeUnknown(t *testing.T) {
	if DatasourceType(99).GrafanaType() != "unknown" {
		t.Fatal("expected unknown for out-of-range DatasourceType")
	}
}
