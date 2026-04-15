package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestClient creates a Client pointing at the given httptest.Server with a
// temp file containing a known API key.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "api-key")
	if err := os.WriteFile(keyFile, []byte("test-api-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return NewClient(serverURL, keyFile)
}

// --- NewClient ---

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	c := NewClient("http://grafana.example.com/", "unused")
	if c.baseURL != "http://grafana.example.com" {
		t.Fatalf("expected trailing slash trimmed, got %q", c.baseURL)
	}
}

// --- ListManagedDatasources ---

func TestListManagedDatasources_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/datasources" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Datasource{
			{UID: "tp-pg", Name: "Postgres"},
			{UID: "regular-ds", Name: "Regular"},
			{UID: "tp-mysql", Name: "MySQL"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ds, err := c.ListManagedDatasources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds) != 2 {
		t.Fatalf("expected 2 managed datasources, got %d", len(ds))
	}
	if ds[0].UID != "tp-pg" || ds[1].UID != "tp-mysql" {
		t.Errorf("unexpected UIDs: %v, %v", ds[0].UID, ds[1].UID)
	}
}

func TestListManagedDatasources_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ds, err := c.ListManagedDatasources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds) != 0 {
		t.Fatalf("expected 0 datasources, got %d", len(ds))
	}
}

func TestListManagedDatasources_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid API key"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.ListManagedDatasources(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to contain status code, got: %v", err)
	}
}

func TestListManagedDatasources_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.ListManagedDatasources(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "deserializing") {
		t.Errorf("expected deserialization error, got: %v", err)
	}
}

func TestListManagedDatasources_AuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, _ = c.ListManagedDatasources(context.Background())
	if gotAuth != "Bearer test-api-key" {
		t.Errorf("expected 'Bearer test-api-key', got %q", gotAuth)
	}
}

// --- CreateDatasource ---

func TestCreateDatasource_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/datasources" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.CreateDatasource(context.Background(), &DatasourceRequest{
		Name: "test-ds", UID: "tp-test", Type: "postgres",
		URL: "localhost:5432", Access: "proxy",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDatasource_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"data source with the same name already exists"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.CreateDatasource(context.Background(), &DatasourceRequest{
		Name: "test-ds", UID: "tp-test",
	})
	if err == nil {
		t.Fatal("expected error for 409")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

func TestCreateDatasource_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.CreateDatasource(context.Background(), &DatasourceRequest{
		Name: "test-ds", UID: "tp-test",
	})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error, got: %v", err)
	}
}

func TestCreateDatasource_RequestBody(t *testing.T) {
	var gotBody DatasourceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("request body is not valid JSON: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &DatasourceRequest{
		Name:   "my-ds",
		UID:    "tp-my-ds",
		Type:   "postgres",
		URL:    "pg:5432",
		Access: "proxy",
		JSONData: map[string]any{
			"sslmode": "require",
		},
	}
	if err := c.CreateDatasource(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody.Name != "my-ds" {
		t.Errorf("expected name 'my-ds', got %q", gotBody.Name)
	}
	if gotBody.UID != "tp-my-ds" {
		t.Errorf("expected uid 'tp-my-ds', got %q", gotBody.UID)
	}
	if gotBody.Type != "postgres" {
		t.Errorf("expected type 'postgres', got %q", gotBody.Type)
	}
}

// --- UpdateDatasource ---

func TestUpdateDatasource_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.UpdateDatasource(context.Background(), &DatasourceRequest{
		Name: "test-ds", UID: "tp-test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateDatasource_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"datasource not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.UpdateDatasource(context.Background(), &DatasourceRequest{
		Name: "test-ds", UID: "tp-test",
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestUpdateDatasource_URLContainsUID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_ = c.UpdateDatasource(context.Background(), &DatasourceRequest{
		Name: "test-ds", UID: "tp-my-uid",
	})
	expected := "/api/datasources/uid/tp-my-uid"
	if gotPath != expected {
		t.Errorf("expected path %q, got %q", expected, gotPath)
	}
}

// --- DeleteDatasource ---

func TestDeleteDatasource_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.DeleteDatasource(context.Background(), "tp-del")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteDatasource_NotFoundTreatedAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.DeleteDatasource(context.Background(), "tp-gone")
	if err != nil {
		t.Fatalf("expected no error for 404, got: %v", err)
	}
}

func TestDeleteDatasource_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server on fire`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	err := c.DeleteDatasource(context.Background(), "tp-fail")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "server on fire") {
		t.Errorf("expected body in error message, got: %v", err)
	}
}

func TestDeleteDatasource_AuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_ = c.DeleteDatasource(context.Background(), "tp-auth")
	if gotAuth != "Bearer test-api-key" {
		t.Errorf("expected 'Bearer test-api-key', got %q", gotAuth)
	}
}
