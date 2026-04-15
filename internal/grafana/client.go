// Package grafana provides a client for the Grafana datasource API.
package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/maestra-io/teleport-grafana-datasource-sync-go/internal/config"
)

// UIDPrefix is prepended to all managed datasource UIDs.
const UIDPrefix = "tp-"

// Client communicates with the Grafana datasource API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKeyFile string
}

// Datasource represents a Grafana datasource returned by the list API.
type Datasource struct {
	UID              string          `json:"uid"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	URL              string          `json:"url"`
	Access           string          `json:"access"`
	JSONData         map[string]any  `json:"jsonData"`
	SecureJSONFields map[string]bool `json:"secureJsonFields"`
}

// DatasourceRequest is the payload for creating or updating a datasource.
type DatasourceRequest struct {
	Name           string         `json:"name"`
	UID            string         `json:"uid"`
	Type           string         `json:"type"`
	URL            string         `json:"url"`
	Access         string         `json:"access"`
	JSONData       map[string]any `json:"jsonData"`
	SecureJSONData map[string]any `json:"secureJsonData,omitempty"`
}

// NewClient creates a Grafana API client. The API key is read fresh from
// apiKeyFile on every request to support Vault/VSO rotation.
func NewClient(baseURL, apiKeyFile string) *Client {
	var transport *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = dt.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.ResponseHeaderTimeout = 10 * time.Second

	return &Client{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKeyFile: apiKeyFile,
	}
}

func (c *Client) apiKey() (string, error) {
	return config.ReadGrafanaAPIKey(c.apiKeyFile)
}

// ListManagedDatasources returns all datasources whose UID starts with the managed prefix.
func (c *Client) ListManagedDatasources(ctx context.Context) ([]Datasource, error) {
	key, err := c.apiKey()
	if err != nil {
		return nil, fmt.Errorf("reading API key: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/datasources", nil)
	if err != nil {
		return nil, fmt.Errorf("building list request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending list datasources request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("list datasources failed: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var all []Datasource
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, fmt.Errorf("deserializing datasource list: %w", err)
	}

	var managed []Datasource
	for _, ds := range all {
		if strings.HasPrefix(ds.UID, UIDPrefix) {
			managed = append(managed, ds)
		}
	}

	slog.Info("fetched datasources from Grafana",
		"total", len(all),
		"managed", len(managed),
	)
	return managed, nil
}

// CreateDatasource creates a new datasource in Grafana.
func (c *Client) CreateDatasource(ctx context.Context, req *DatasourceRequest) error {
	key, err := c.apiKey()
	if err != nil {
		return fmt.Errorf("reading API key: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling create request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/datasources", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending create datasource request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("create datasource %q (uid=%q): already exists: %s", req.Name, req.UID, strings.TrimSpace(string(respBody)))
	}
	return fmt.Errorf("create datasource %q failed: %d %s", req.Name, resp.StatusCode, strings.TrimSpace(string(respBody)))
}

// UpdateDatasource updates an existing datasource by UID.
func (c *Client) UpdateDatasource(ctx context.Context, req *DatasourceRequest) error {
	key, err := c.apiKey()
	if err != nil {
		return fmt.Errorf("reading API key: %w", err)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling update request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/datasources/uid/"+req.UID, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building update request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending update datasource request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("update datasource %q failed: %d %s", req.Name, resp.StatusCode, strings.TrimSpace(string(respBody)))
}

// DeleteDatasource deletes a datasource by UID. A 404 is treated as success.
func (c *Client) DeleteDatasource(ctx context.Context, uid string) error {
	key, err := c.apiKey()
	if err != nil {
		return fmt.Errorf("reading API key: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/datasources/uid/"+uid, nil)
	if err != nil {
		return fmt.Errorf("building delete request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending delete datasource request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		slog.Debug("datasource already absent, skipping delete", "uid", uid)
		return nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("delete datasource uid=%q failed: %d %s", uid, resp.StatusCode, strings.TrimSpace(string(respBody)))
}
