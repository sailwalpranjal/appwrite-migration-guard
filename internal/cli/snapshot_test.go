package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/manifest"
)

func fakeInventoryServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{
				"total":     1,
				"databases": []map[string]any{{"$id": "db1", "name": "Main", "enabled": true}},
			})
		case "/tablesdb/db1/tables":
			json.NewEncoder(w).Encode(map[string]any{
				"total":  1,
				"tables": []map[string]any{{"$id": "t1", "databaseId": "db1", "name": "Widgets", "enabled": true}},
			})
		case "/tablesdb/db1/tables/t1/rows":
			json.NewEncoder(w).Encode(map[string]any{"total": 3, "rows": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRunSnapshot_WritesReadableManifest(t *testing.T) {
	srv := fakeInventoryServer(t)
	defer srv.Close()
	withEnv(t, srv.URL, "proj1", "key1")

	dir := t.TempDir()
	outPath := filepath.Join(dir, "manifest.json")

	var stdout, stderr bytes.Buffer
	code := RunSnapshot(context.Background(), []string{"--out", outPath, "--label", "source"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stderr:\n%s", code, stderr.String())
	}

	m, err := manifest.Read(outPath)
	if err != nil {
		t.Fatalf("manifest.Read: %v", err)
	}
	if m.Label != "source" {
		t.Fatalf("expected label %q, got %q", "source", m.Label)
	}
	if len(m.Inventory.Resources) != 2 {
		t.Fatalf("expected 2 resources (1 database + 1 table), got %d: %+v", len(m.Inventory.Resources), m.Inventory.Resources)
	}
}

func TestRunSnapshot_MissingConfig(t *testing.T) {
	withEnv(t, "", "", "")
	var stdout, stderr bytes.Buffer
	code := RunSnapshot(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
}
