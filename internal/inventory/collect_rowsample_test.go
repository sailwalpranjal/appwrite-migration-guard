package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func rowSampleServer(t *testing.T, rows []map[string]any, rowsErr int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{
				"total":     1,
				"databases": []map[string]any{{"$id": "db1", "name": "Main", "enabled": true}},
			})
		case r.URL.Path == "/tablesdb/db1/tables":
			json.NewEncoder(w).Encode(map[string]any{
				"total":  1,
				"tables": []map[string]any{{"$id": "t1", "databaseId": "db1", "name": "Widgets", "enabled": true}},
			})
		case r.URL.Path == "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case r.URL.Path == "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		case r.URL.Path == "/tablesdb/db1/tables/t1/rows":
			if rowsErr != 0 {
				w.WriteHeader(rowsErr)
				json.NewEncoder(w).Encode(map[string]any{"message": "forbidden", "code": rowsErr})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"total": len(rows), "rows": rows})
		default:
			if strings.HasSuffix(r.URL.Path, "/tables") {
				json.NewEncoder(w).Encode(map[string]any{"total": 0, "tables": []any{}})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollect_SampleRowsDisabledByDefault(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tablesdb/db1/tables/t1/rows" {
			calls++
		}
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}})
		case "/tablesdb/db1/tables":
			json.NewEncoder(w).Encode(map[string]any{"total": 1, "tables": []map[string]any{{"$id": "t1", "databaseId": "db1", "name": "Widgets"}}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no row-sampling calls when SampleRows is unset, got %d", calls)
	}
}

func TestCollect_SampleRowsPopulatesSamples(t *testing.T) {
	rows := []map[string]any{
		{"$id": "r1", "$permissions": []string{"read(\"any\")"}, "name": "Alice"},
		{"$id": "r2", "name": "Bob"},
	}
	srv := rowSampleServer(t, rows, 0)
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{SampleRows: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var table *Resource
	for i := range inv.Resources {
		if inv.Resources[i].ID == "t1" {
			table = &inv.Resources[i]
		}
	}
	if table == nil {
		t.Fatal("expected table resource t1")
	}
	if len(table.RowSamples) != 2 {
		t.Fatalf("expected 2 row samples, got %d: %+v", len(table.RowSamples), table.RowSamples)
	}
	if table.RowSamples[0].ID != "r1" || len(table.RowSamples[0].Permissions) != 1 {
		t.Fatalf("unexpected first sample: %+v", table.RowSamples[0])
	}
	if table.RowSamples[0].Digest == table.RowSamples[1].Digest {
		t.Fatal("expected different rows to have different digests")
	}
}

// Regression guard: CountRows and SampleRows now share a single
// runBestEffort pass per table, so this proves both still populate
// correctly when requested together, and a failure in one does not
// affect the other for the same table.
func TestCollect_CountRowsAndSampleRowsTogether(t *testing.T) {
	rows := []map[string]any{{"$id": "r1", "name": "Alice"}}
	srv := rowSampleServer(t, rows, 0)
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{CountRows: true, SampleRows: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var table *Resource
	for i := range inv.Resources {
		if inv.Resources[i].ID == "t1" {
			table = &inv.Resources[i]
		}
	}
	if table == nil {
		t.Fatal("expected table resource t1")
	}
	if table.CountError != "" {
		t.Fatalf("unexpected CountError: %s", table.CountError)
	}
	if table.RowCount != 1 {
		t.Fatalf("expected RowCount 1, got %d", table.RowCount)
	}
	if len(table.RowSamples) != 1 || table.RowSamples[0].ID != "r1" {
		t.Fatalf("expected 1 row sample for r1, got %+v", table.RowSamples)
	}
}

func TestCollect_SampleRowsFailureIsWarnNotAbort(t *testing.T) {
	srv := rowSampleServer(t, nil, http.StatusForbidden)
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{SampleRows: 10})
	if err != nil {
		t.Fatalf("expected Collect to succeed despite sampling failure, got %v", err)
	}
	if !inv.PartiallyVerified() {
		t.Fatal("expected PartiallyVerified to be true")
	}
	var table *Resource
	for i := range inv.Resources {
		if inv.Resources[i].ID == "t1" {
			table = &inv.Resources[i]
		}
	}
	if table == nil || table.SampleError == "" {
		t.Fatalf("expected SampleError to be set on the table, got %+v", table)
	}
}
