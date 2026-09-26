package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func withSourceDestEnv(t *testing.T, srcURL, dstURL string) {
	t.Helper()
	vars := map[string]string{
		"AMG_SOURCE_ENDPOINT": srcURL, "AMG_SOURCE_PROJECT_ID": "src-proj", "AMG_SOURCE_API_KEY": "src-key",
		"AMG_DEST_ENDPOINT": dstURL, "AMG_DEST_PROJECT_ID": "dst-proj", "AMG_DEST_API_KEY": "dst-key",
	}
	for k, v := range vars {
		old, had := os.LookupEnv(k)
		os.Setenv(k, v)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func tablesDBServer(t *testing.T, databases, tables, rowTotal int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tablesdb":
			dbs := make([]map[string]any, databases)
			for i := range dbs {
				dbs[i] = map[string]any{"$id": "db1", "name": "Main", "enabled": true}
			}
			if databases == 0 {
				dbs = []map[string]any{}
			}
			json.NewEncoder(w).Encode(map[string]any{"total": databases, "databases": dbs})
		case r.URL.Path == "/tablesdb/db1/tables":
			ts := []map[string]any{}
			if tables > 0 {
				ts = append(ts, map[string]any{"$id": "t1", "databaseId": "db1", "name": "Widgets", "enabled": true})
			}
			json.NewEncoder(w).Encode(map[string]any{"total": len(ts), "tables": ts})
		case r.URL.Path == "/tablesdb/db1/tables/t1/rows":
			json.NewEncoder(w).Encode(map[string]any{"total": rowTotal, "rows": []any{}})
		case r.URL.Path == "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRunVerify_IdenticalEnvironments_Pass(t *testing.T) {
	src := tablesDBServer(t, 1, 1, 5)
	defer src.Close()
	dst := tablesDBServer(t, 1, 1, 5)
	defer dst.Close()
	withSourceDestEnv(t, src.URL, dst.URL)

	var stdout, stderr bytes.Buffer
	code := RunVerify(context.Background(), nil, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stdout:\n%s stderr:\n%s", code, stdout.String(), stderr.String())
	}
}

func TestRunVerify_MissingDestinationTable_Blocks(t *testing.T) {
	src := tablesDBServer(t, 1, 1, 5)
	defer src.Close()
	dst := tablesDBServer(t, 1, 0, 0) // destination database exists but the table is missing
	defer dst.Close()
	withSourceDestEnv(t, src.URL, dst.URL)

	var stdout, stderr bytes.Buffer
	code := RunVerify(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stdout:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("missing_resource")) {
		t.Fatalf("expected missing_resource in output:\n%s", stdout.String())
	}
}

func TestRunVerify_RowCountDrift_Blocks(t *testing.T) {
	src := tablesDBServer(t, 1, 1, 100)
	defer src.Close()
	dst := tablesDBServer(t, 1, 1, 42)
	defer dst.Close()
	withSourceDestEnv(t, src.URL, dst.URL)

	var stdout, stderr bytes.Buffer
	code := RunVerify(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stdout:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("row_count_mismatch")) {
		t.Fatalf("expected row_count_mismatch in output:\n%s", stdout.String())
	}
}

func TestRunVerify_MissingConfig(t *testing.T) {
	withSourceDestEnv(t, "", "")
	var stdout, stderr bytes.Buffer
	code := RunVerify(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
}
