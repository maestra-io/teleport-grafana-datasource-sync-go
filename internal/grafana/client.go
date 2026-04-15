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

const UIDPrefix = "tp-"

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKeyFile string
}

type Datasource struct {
	UID              string         `json:"uid"`
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	URL              string         `json:"url"`
	Access           string         `json:"access"`
	JSONData         map[string]any `json:"jsonData"`
	SecureJSONFields map[string]any `json:"secureJsonFields"`
}

type DatasourceRequest struct {
	Name           string         `json:"name"`
	UID            string         `json:"uid"`
	Type           string         `json:"type"`
	URL            string         `json:"url"`
	Access         string         `json:"access"`
	JSONData       map[string]any `json:"jsonData"`
	SecureJSONData map[string]any `json:"secureJsonData,omitempty"`
}

func NewClient(baseURL, apiKeyFile string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				ResponseHeaderTimeout: 10 * time.Second,
			},
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
		return nil, err
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list datasources failed: %d %s", resp.StatusCode, string(body))
	}

	var all []Datasource
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, fmt.Errorf("deserializing Grafana datasource list: %w", err)
	}

	managed := make([]Datasource, 0)
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

func (c *Client) CreateDatasource(ctx context.Context, req *DatasourceRequest) error {
	key, err := c.apiKey()
	if err != nil {
		return err
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
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("create datasource '%s' (uid=%s) conflict — already exists: %s", req.Name, req.UID, string(respBody))
	}
	return fmt.Errorf("create datasource '%s' failed: %d %s", req.Name, resp.StatusCode, string(respBody))
}

func (c *Client) UpdateDatasource(ctx context.Context, req *DatasourceRequest) error {
	key, err := c.apiKey()
	if err != nil {
		return err
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
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("update datasource '%s' failed: %d %s", req.Name, resp.StatusCode, string(respBody))
}

func (c *Client) DeleteDatasource(ctx context.Context, uid string) error {
	key, err := c.apiKey()
	if err != nil {
		return err
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

	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusNotFound {
		if resp.StatusCode == http.StatusNotFound {
			slog.Debug("datasource already absent, skipping delete", "uid", uid)
		}
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("delete datasource uid=%s failed: %d %s", uid, resp.StatusCode, string(respBody))
}
