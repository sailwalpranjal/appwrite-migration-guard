package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

func mustDecodeQueries(t *testing.T, r *http.Request) []string {
	t.Helper()
	return r.URL.Query()["queries[]"]
}

func hasQueryMethod(queries []string, method string) bool {
	for _, q := range queries {
		if strings.Contains(q, `"method":"`+method+`"`) {
			return true
		}
	}
	return false
}

// TestListDatabases_SinglePage covers the "one page" pagination scenario.
func TestListDatabases_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(databaseListResponse{
			Total:     2,
			Databases: []Database{{ID: "db1", Name: "one"}, {ID: "db2", Name: "two"}},
		})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	dbs, err := c.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dbs) != 2 {
		t.Fatalf("expected 2 databases, got %d", len(dbs))
	}
}

// TestListDatabases_MultiplePages covers pagination across several full
// pages followed by a short final page.
func TestListDatabases_MultiplePages(t *testing.T) {
	total := pageSize*2 + 3 // two full pages + one short page
	all := make([]Database, total)
	for i := range all {
		all[i] = Database{ID: fmt.Sprintf("db%04d", i), Name: fmt.Sprintf("db-%d", i)}
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		queries := mustDecodeQueries(t, r)
		if !hasQueryMethod(queries, "orderAsc") {
			t.Fatalf("expected orderAsc query for deterministic pagination, got %v", queries)
		}
		cursor := ""
		for _, q := range queries {
			if strings.Contains(q, `"method":"cursorAfter"`) {
				var parsed struct {
					Values []string `json:"values"`
				}
				json.Unmarshal([]byte(q), &parsed)
				if len(parsed.Values) > 0 {
					cursor = parsed.Values[0]
				}
			}
		}
		start := 0
		if cursor != "" {
			for i, d := range all {
				if d.ID == cursor {
					start = i + 1
					break
				}
			}
		}
		end := start + pageSize
		if end > len(all) {
			end = len(all)
		}
		json.NewEncoder(w).Encode(databaseListResponse{Total: len(all), Databases: all[start:end]})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	got, err := c.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != total {
		t.Fatalf("expected %d databases, got %d", total, len(got))
	}
	if requests != 3 {
		t.Fatalf("expected 3 page requests, got %d", requests)
	}
	for i, d := range got {
		if d.ID != all[i].ID {
			t.Fatalf("result out of order at index %d: got %s want %s", i, d.ID, all[i].ID)
		}
	}
}

// TestListDatabases_EmptyPage covers a project with zero databases.
func TestListDatabases_EmptyPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(databaseListResponse{Total: 0, Databases: []Database{}})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	got, err := c.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 databases, got %d", len(got))
	}
}

// TestListDatabases_MalformedCursor covers a server that returns a page
// whose last item's ID never changes (or is empty), which would spin
// forever without the guard in paginate().
func TestListDatabases_MalformedCursor(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Always return a full page ending in the same ID, regardless of cursor.
		page := make([]Database, pageSize)
		for i := range page {
			page[i] = Database{ID: fmt.Sprintf("stuck%03d", i)}
		}
		json.NewEncoder(w).Encode(databaseListResponse{Total: 999999, Databases: page})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	_, err := c.ListDatabases(context.Background())
	if err == nil {
		t.Fatal("expected pagination error")
	}
	if !errs.IsKind(err, errs.KindPagination) {
		t.Fatalf("expected KindPagination, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 requests before detecting the stuck cursor, got %d", calls)
	}
}

// TestListDatabases_TransientFailureMidPagination covers a transient
// server failure on the third page that succeeds on retry, per spec
// section 16/24: transient failures during pagination must not silently
// truncate results.
func TestListDatabases_TransientFailureMidPagination(t *testing.T) {
	all := make([]Database, pageSize*2+1)
	for i := range all {
		all[i] = Database{ID: fmt.Sprintf("db%04d", i)}
	}

	pageRequests := map[int]int{}
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries := mustDecodeQueries(t, r)
		cursor := ""
		for _, q := range queries {
			if strings.Contains(q, `"method":"cursorAfter"`) {
				var parsed struct {
					Values []string `json:"values"`
				}
				json.Unmarshal([]byte(q), &parsed)
				cursor = parsed.Values[0]
			}
		}
		start := 0
		if cursor != "" {
			for i, d := range all {
				if d.ID == cursor {
					start = i + 1
					break
				}
			}
		}
		thisPage := start / pageSize
		pageRequests[thisPage]++
		if thisPage == 2 && pageRequests[thisPage] == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(apiError{Message: "transient", Code: 500})
			return
		}
		end := start + pageSize
		if end > len(all) {
			end = len(all)
		}
		json.NewEncoder(w).Encode(databaseListResponse{Total: len(all), Databases: all[start:end]})
		page++
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL), WithMaxRetries(2))
	c.backoff = time.Millisecond
	got, err := c.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(all) {
		t.Fatalf("expected %d databases despite transient failure, got %d", len(all), len(got))
	}
}

// cancelAfterFirstRoundTrip wraps a Transport and cancels a context once
// the first HTTP round trip completes, so the *next* paginate() loop
// iteration deterministically observes ctx.Done() before issuing another
// request — avoiding a race against the server closing its response.
type cancelAfterFirstRoundTrip struct {
	next   http.RoundTripper
	cancel context.CancelFunc
	calls  int
}

func (rt *cancelAfterFirstRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := rt.next.RoundTrip(r)
	rt.calls++
	if rt.calls == 1 {
		rt.cancel()
	}
	return resp, err
}

// TestListDatabases_CancellationDuringPagination covers a caller-cancelled
// context mid-pagination: amg must stop and report a timeout error rather
// than returning a silently truncated result as success.
func TestListDatabases_CancellationDuringPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := make([]Database, pageSize)
		for i := range page {
			page[i] = Database{ID: fmt.Sprintf("db%03d", i)}
		}
		json.NewEncoder(w).Encode(databaseListResponse{Total: 999, Databases: page})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := &cancelAfterFirstRoundTrip{next: http.DefaultTransport, cancel: cancel}
	c := New(testEnv(srv.URL), WithHTTPClient(&http.Client{Transport: rt}))
	_, err := c.ListDatabases(ctx)
	if err == nil {
		t.Fatal("expected an error after cancellation")
	}
	if !errs.IsKind(err, errs.KindTimeout) {
		t.Fatalf("expected KindTimeout, got %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("expected pagination to stop after 1 request once cancelled, got %d", rt.calls)
	}
}

func TestListTables_UsesDatabasePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tablesdb/db1/tables" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(tableListResponse{Total: 1, Tables: []Table{{ID: "t1", DatabaseID: "db1", Name: "widgets"}}})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	tables, err := c.ListTables(context.Background(), "db1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tables) != 1 || tables[0].Name != "widgets" {
		t.Fatalf("unexpected tables: %+v", tables)
	}
}

func TestCountRows_ReportsCapAtThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tablesdb/db1/tables/t1/rows" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(rowListResponse{Total: rowCountCap, Rows: []map[string]any{{"$id": "r1"}}})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	count, capped, err := c.CountRows(context.Background(), "db1", "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != rowCountCap || !capped {
		t.Fatalf("expected capped count of %d, got count=%d capped=%v", rowCountCap, count, capped)
	}
}

func TestCountRows_BelowCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(rowListResponse{Total: 42})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	count, capped, err := c.CountRows(context.Background(), "db1", "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 42 || capped {
		t.Fatalf("expected count=42 capped=false, got count=%d capped=%v", count, capped)
	}
}
