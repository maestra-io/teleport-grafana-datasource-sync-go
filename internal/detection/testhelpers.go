package detection

import (
	"encoding/json"
	"strings"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/grafana"
)

// MakeDetectedDS creates a DetectedDatasource for testing.
func MakeDetectedDS(name, tpName string) DetectedDatasource {
	return DetectedDatasource{
		Name:            name,
		UID:             grafana.UIDPrefix + name,
		DSType:          Prometheus,
		URL:             "http://" + tpName,
		TeleportAppName: tpName,
		JSONData:        Prometheus.DefaultJSONData(),
		SecureJSONData:  nil,
	}
}

// MakeExistingDS creates a grafana.Datasource for testing with JSON round-trip.
func MakeExistingDS(name, tpName string) grafana.Datasource {
	jsonData := Prometheus.DefaultJSONData()
	raw, _ := json.Marshal(jsonData)
	var decoded map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	_ = dec.Decode(&decoded)

	return grafana.Datasource{
		UID:              grafana.UIDPrefix + name,
		Name:             name,
		Type:             "prometheus",
		URL:              "http://" + tpName,
		Access:           "proxy",
		JSONData:         decoded,
		SecureJSONFields: map[string]any{},
	}
}
