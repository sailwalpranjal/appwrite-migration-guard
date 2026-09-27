package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
)

func newTestClient(url string, opts ...appwrite.Option) *appwrite.Client {
	env := config.Environment{Label: "target", Endpoint: url, ProjectID: "proj1", APIKey: "key1"}
	return appwrite.New(env, opts...)
}

// fakeServer builds a minimal in-memory Appwrite TablesDB API for tests:
// 2 databases, each with 2 tables, each table reporting a fixed row count.
func fakeServer(t *testing.T, onRowCount func(databaseID, tableID string) (int, int)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{
				"total": 2,
				"databases": []map[string]any{
					{"$id": "db-b", "name": "B", "enabled": true, "type": "regular", "status": "ok"},
					{"$id": "db-a", "name": "A", "enabled": true, "type": "regular", "status": "ok"},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/tables") && strings.HasPrefix(r.URL.Path, "/tablesdb/"):
			dbID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/tablesdb/"), "/tables")
			json.NewEncoder(w).Encode(map[string]any{
				"total": 2,
				"tables": []map[string]any{
					{"$id": dbID + "-t2", "databaseId": dbID, "name": "T2", "enabled": true},
					{"$id": dbID + "-t1", "databaseId": dbID, "name": "T1", "enabled": true},
				},
			})
		case r.URL.Path == "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case r.URL.Path == "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		case strings.HasSuffix(r.URL.Path, "/rows"):
			parts := strings.Split(r.URL.Path, "/")
			// /tablesdb/{db}/tables/{table}/rows
			dbID, tableID := parts[2], parts[4]
			total, code := onRowCount(dbID, tableID)
			if code != 0 {
				w.WriteHeader(code)
			}
			json.NewEncoder(w).Encode(map[string]any{"total": total, "rows": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollect_DeterministicOrdering(t *testing.T) {
	srv := fakeServer(t, func(dbID, tableID string) (int, int) { return 5, 0 })
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{CountRows: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var order []string
	for _, r := range inv.Resources {
		order = append(order, string(r.Type)+":"+r.ParentID+":"+r.ID)
	}
	want := []string{
		"database::db-a",
		"database::db-b",
		"table:db-a:db-a-t1",
		"table:db-a:db-a-t2",
		"table:db-b:db-b-t1",
		"table:db-b:db-b-t2",
	}
	if len(order) != len(want) {
		t.Fatalf("expected %d resources, got %d: %v", len(want), len(order), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("resource %d: got %q want %q (full: %v)", i, order[i], want[i], order)
		}
	}
}

func TestCollect_RowCountsAttached(t *testing.T) {
	srv := fakeServer(t, func(dbID, tableID string) (int, int) { return 42, 0 })
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{CountRows: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range inv.Resources {
		if r.Type != ResourceTable {
			continue
		}
		if r.RowCount != 42 || r.RowCountCapped {
			t.Fatalf("table %s: expected RowCount=42 capped=false, got %d/%v", r.ID, r.RowCount, r.RowCountCapped)
		}
	}
}

func TestCollect_CountRowsDisabled(t *testing.T) {
	called := int32(0)
	srv := fakeServer(t, func(dbID, tableID string) (int, int) {
		atomic.AddInt32(&called, 1)
		return 0, 0
	})
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{CountRows: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 0 {
		t.Fatalf("expected CountRows to never be called, got %d calls", called)
	}
	if inv.PartiallyVerified() {
		t.Fatal("did not expect PartiallyVerified when row counting is disabled")
	}
}

func TestCollect_PartialRowCountFailureIsWarnNotAbort(t *testing.T) {
	srv := fakeServer(t, func(dbID, tableID string) (int, int) {
		if tableID == "db-a-t1" {
			return 0, http.StatusForbidden
		}
		return 7, 0
	})
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{CountRows: true})
	if err != nil {
		t.Fatalf("expected Collect to succeed despite one table's count failing, got %v", err)
	}
	if !inv.PartiallyVerified() {
		t.Fatal("expected PartiallyVerified to be true")
	}

	var found bool
	for _, r := range inv.Resources {
		if r.ID == "db-a-t1" {
			found = true
			if r.CountError == "" {
				t.Fatal("expected CountError to be set on db-a-t1")
			}
		} else if r.Type == ResourceTable && r.CountError != "" {
			t.Fatalf("unexpected CountError on unrelated table %s: %s", r.ID, r.CountError)
		}
	}
	if !found {
		t.Fatal("expected db-a-t1 to still be present in the inventory")
	}
}

func TestCollect_ListDatabasesFailureAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"message": "boom", "code": 500})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, appwrite.WithMaxRetries(0))
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err == nil {
		t.Fatal("expected an error when ListDatabases fails")
	}
}

func TestCollect_UnsupportedResourcesAreExplicit(t *testing.T) {
	srv := fakeServer(t, func(string, string) (int, int) { return 0, 0 })
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Unsupported) == 0 {
		t.Fatal("expected Unsupported to list resource types amg does not collect yet")
	}
}

func TestCollect_ConcurrencyIsBounded(t *testing.T) {
	var inFlight, maxInFlight int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/tablesdb/") && strings.HasSuffix(r.URL.Path, "/tables") {
			cur := atomic.AddInt32(&inFlight, 1)
			defer atomic.AddInt32(&inFlight, -1)
			time.Sleep(15 * time.Millisecond) // hold the slot so concurrent requests overlap
			for {
				m := atomic.LoadInt32(&maxInFlight)
				if cur <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, cur) {
					break
				}
			}
			dbID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/tablesdb/"), "/tables")
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "tables": []map[string]any{{"$id": dbID + "-t1", "databaseId": dbID, "name": "T"}}})
			return
		}
		// 20 databases so there's real fan-out to bound.
		dbs := make([]map[string]any, 20)
		for i := range dbs {
			dbs[i] = map[string]any{"$id": fmt.Sprintf("db%02d", i), "name": fmt.Sprintf("db%02d", i)}
		}
		json.NewEncoder(w).Encode(map[string]any{"total": len(dbs), "databases": dbs})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{Concurrency: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxInFlight > 3 {
		t.Fatalf("expected at most 3 concurrent ListTables requests, observed %d", maxInFlight)
	}
	if maxInFlight < 2 {
		t.Fatalf("expected genuine concurrency (>=2 in flight at once), observed %d", maxInFlight)
	}
}
