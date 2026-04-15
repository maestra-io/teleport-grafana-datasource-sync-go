package teleport

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// ListApps
// ---------------------------------------------------------------------------

func TestListApps_ValidMultipleApps(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `[
		{"metadata":{"name":"app-one"}},
		{"metadata":{"name":"app-two"}},
		{"metadata":{"name":"app-three"}}
	]`)

	apps, err := ListApps(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apps) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(apps))
	}
	want := []string{"app-one", "app-two", "app-three"}
	for i, w := range want {
		if apps[i].Name != w {
			t.Errorf("apps[%d].Name = %q, want %q", i, apps[i].Name, w)
		}
	}
}

func TestListApps_EmptyNameFiltered(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `[
		{"metadata":{"name":"keep-me"}},
		{"metadata":{"name":""}},
		{"metadata":{}}
	]`)

	apps, err := ListApps(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	if apps[0].Name != "keep-me" {
		t.Errorf("apps[0].Name = %q, want %q", apps[0].Name, "keep-me")
	}
}

func TestListApps_InvalidJSON(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `NOT VALID JSON`)

	_, err := ListApps(context.Background(), f)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestListApps_EmptyArray(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `[]`)

	apps, err := ListApps(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected 0 apps, got %d", len(apps))
	}
}

func TestListApps_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	nonexistent := filepath.Join(t.TempDir(), "does-not-exist.json")

	_, err := ListApps(ctx, nonexistent)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListKubeClusters
// ---------------------------------------------------------------------------

func TestListKubeClusters_ValidMultiple(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `[
		{"spec":{"cluster":{"metadata":{"name":"cluster-b"}}}},
		{"spec":{"cluster":{"metadata":{"name":"cluster-a"}}}}
	]`)

	names := ListKubeClusters(f)
	if len(names) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(names))
	}
	// Must be sorted.
	if names[0] != "cluster-a" || names[1] != "cluster-b" {
		t.Errorf("expected [cluster-a cluster-b], got %v", names)
	}
}

func TestListKubeClusters_Dedup(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `[
		{"spec":{"cluster":{"metadata":{"name":"dup"}}}},
		{"spec":{"cluster":{"metadata":{"name":"dup"}}}},
		{"spec":{"cluster":{"metadata":{"name":"unique"}}}}
	]`)

	names := ListKubeClusters(f)
	if len(names) != 2 {
		t.Fatalf("expected 2 unique clusters, got %d: %v", len(names), names)
	}
	if names[0] != "dup" || names[1] != "unique" {
		t.Errorf("expected [dup unique], got %v", names)
	}
}

func TestListKubeClusters_Sorted(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `[
		{"spec":{"cluster":{"metadata":{"name":"zebra"}}}},
		{"spec":{"cluster":{"metadata":{"name":"alpha"}}}},
		{"spec":{"cluster":{"metadata":{"name":"middle"}}}}
	]`)

	names := ListKubeClusters(f)
	want := []string{"alpha", "middle", "zebra"}
	if len(names) != len(want) {
		t.Fatalf("expected %d clusters, got %d", len(want), len(names))
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("names[%d] = %q, want %q", i, names[i], w)
		}
	}
}

func TestListKubeClusters_EmptyNameFiltered(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `[
		{"spec":{"cluster":{"metadata":{"name":"keep"}}}},
		{"spec":{"cluster":{"metadata":{"name":""}}}},
		{"spec":{"cluster":{"metadata":{}}}}
	]`)

	names := ListKubeClusters(f)
	if len(names) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %v", len(names), names)
	}
	if names[0] != "keep" {
		t.Errorf("expected 'keep', got %q", names[0])
	}
}

func TestListKubeClusters_FileMissing(t *testing.T) {
	t.Parallel()
	names := ListKubeClusters(filepath.Join(t.TempDir(), "no-such-file.json"))
	if names != nil {
		t.Fatalf("expected nil, got %v", names)
	}
}

func TestListKubeClusters_FileEmpty(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, "")

	names := ListKubeClusters(f)
	if names != nil {
		t.Fatalf("expected nil for empty file, got %v", names)
	}
}

func TestListKubeClusters_WhitespaceOnly(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, "   \n\t  \n  ")

	names := ListKubeClusters(f)
	if names != nil {
		t.Fatalf("expected nil for whitespace-only file, got %v", names)
	}
}

func TestListKubeClusters_InvalidJSON(t *testing.T) {
	t.Parallel()
	f := writeTempFile(t, `{not json}`)

	names := ListKubeClusters(f)
	if names != nil {
		t.Fatalf("expected nil for invalid JSON, got %v", names)
	}
}

func TestListKubeClusters_PermissionError(t *testing.T) {
	t.Parallel()
	// Using a directory path as file path triggers a read error that is not ErrNotExist.
	dir := t.TempDir()

	names := ListKubeClusters(dir)
	if names != nil {
		t.Fatalf("expected nil for permission/read error, got %v", names)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "teleport-test-*.json")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if err := os.WriteFile(f.Name(), []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing temp file: %v", err)
	}
	return f.Name()
}
