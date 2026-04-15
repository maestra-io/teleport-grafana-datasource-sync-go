# teleport-grafana-datasource-sync

Automatically syncs Teleport-registered monitoring apps to Grafana datasources.

## How it works

Runs as a 3-container Kubernetes pod:

1. **tbot** — obtains a Teleport machine identity via in-cluster Kubernetes join.
2. **tctl sidecar** — uses the identity to periodically run `tctl get apps` and `tctl get kube_server`, writing JSON to a shared volume.
3. **sync** (this app) — reads the JSON files, detects monitoring apps, and reconciles Grafana datasources via the API.

```text
tbot (identity) --> tctl (JSON files) --> sync (Grafana API)
                    /shared/apps.json
                    /shared/kube-clusters.json
```

## Detection rules

Apps are matched by name pattern and mapped to Grafana datasource types. Rules are evaluated in priority order — the first match wins:

| Priority | Pattern | Datasource type | Name transform | jsonData |
|----------|---------|----------------|----------------|----------|
| 1 | `victorialogs-*` prefix | `victoriametrics-logs-datasource` | Keep as-is | `{}` |
| 2 | `loki` exact, `loki-*` prefix, `*-loki` suffix, `*-loki-*` infix | `loki` | Expand per tenant (see below) | `{"maxLines": 1000}` |
| 3 | `*-thanos-query` suffix | `prometheus` | Strip `-thanos-query` suffix | `{"httpMethod": "POST", "timeInterval": "15s"}` |
| 4 | `*-vmauth` suffix | `victoriametrics-metrics-datasource` | Strip `-vmauth` suffix | `{}` |

## Multi-tenant Loki

Loki runs in multi-tenant mode (`auth_enabled: true`). Tenants are identified by the `X-Scope-OrgID` HTTP header. Tenant names correspond to Teleport Kubernetes cluster names (discovered via `tctl get kube_server`).

Each Loki Teleport app is expanded into one Grafana datasource per tenant:

- **Name**: `{tenant}-loki` (e.g., `eu-aws-kube-infra-production-loki`)
- **UID**: `tp-{tenant}-loki` (truncated with FNV hash suffix if >40 chars)
- **URL**: Loki app's public address (routed through Grafana's tbot HTTP proxy)
- **jsonData**: `{ "maxLines": 1000, "httpHeaderName1": "X-Scope-OrgID" }`
- **secureJsonData**: `{ "httpHeaderValue1": "{tenant}" }`

If the kube clusters file is unavailable or invalid, Loki datasources are excluded from reconciliation entirely — existing Loki datasources are preserved unchanged until the next successful tenant discovery.

## Reconciliation

Every 30 seconds (configurable):

1. Read apps and kube clusters from sidecar JSON files
2. Apply detection rules to build desired datasource list
3. Fetch existing managed datasources from Grafana (filtered by `tp-` UID prefix)
4. Diff: create missing, update changed, delete orphaned
5. Log each action with structured JSON

Individual failures are counted (`failed` stat) but don't stop the cycle.

## Health endpoint

An HTTP health endpoint listens on `:8080` and responds with `200 ok` to all requests. Use it for Kubernetes liveness/readiness probes.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEPORT_APPS_FILE` | `/shared/apps.json` | Path to tctl apps JSON |
| `KUBE_CLUSTERS_FILE` | `/shared/kube-clusters.json` | Path to tctl kube clusters JSON |
| `GRAFANA_URL` | `http://grafana.grafana.svc:80` | Grafana API base URL |
| `GRAFANA_API_KEY_FILE` | `/secrets/grafana-api-key` | Path to Grafana API key file |
| `GRAFANA_API_KEY` | — | Fallback: API key via env var (used when file is not found) |
| `SYNC_INTERVAL_SECS` | `30` | Reconciliation interval |
| `DRY_RUN` | `false` | Log-only mode (no mutations) |

## Teleport role

The bot needs read access to `app_server` and `kube_cluster` resources:

```yaml
spec:
  allow:
    rules:
      - resources: [app_server]
        verbs: [list, read]
      - resources: [kube_cluster]
        verbs: [list, read]
```

## Building

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o teleport-grafana-datasource-sync .
docker build -t teleport-grafana-datasource-sync .
```

Produces a static binary in a distroless container.
