package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCollect_ResourcesFilter_NeverRequestsExcludedCategories is the
// core regression guard for Options.Resources: a category outside the
// filter must never be requested from Appwrite at all — not requested
// and its result discarded — because the whole point is letting a
// caller with a narrowly-scoped API key (e.g. only databases.read/
// tables.read/rows.read, exactly what the README recommends granting
// for a tables-only check) avoid a hard authorization failure on a
// category it was never granted access to. The server below fails the
// test outright if /users, /functions, or /sites is ever hit.
func TestCollect_ResourcesFilter_NeverRequestsExcludedCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			t.Fatalf("storage should not have been requested when Resources=[\"tables\"]: %s", r.URL.Path)
		case "/users":
			t.Fatalf("users should not have been requested when Resources=[\"tables\"]: %s", r.URL.Path)
		case "/functions":
			t.Fatalf("functions should not have been requested when Resources=[\"tables\"]: %s", r.URL.Path)
		case "/sites":
			t.Fatalf("sites should not have been requested when Resources=[\"tables\"]: %s", r.URL.Path)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{Resources: []string{"tables"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Collected) != 1 || inv.Collected[0] != "tables" {
		t.Fatalf("expected Collected=[tables], got %v", inv.Collected)
	}
}

// TestCollect_ResourcesFilter_EmptyMeansAll proves the default
// (unrestricted) behavior is unchanged: an empty/nil Resources still
// collects every category, and Inventory.Collected records the full
// ResourceCategories list rather than being left empty/ambiguous.
func TestCollect_ResourcesFilter_EmptyMeansAll(t *testing.T) {
	srv := fakeServer(t, func(string, string) (int, int) { return 5, 0 })
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Collected) != len(ResourceCategories) {
		t.Fatalf("expected Collected to list all %d categories, got %v", len(ResourceCategories), inv.Collected)
	}
}

// TestCollect_UnknownResourceCategory_Errors is a regression guard
// against a typo (e.g. "table" instead of "tables") silently meaning
// that category is never checked — it must be a loud configuration
// error instead.
func TestCollect_UnknownResourceCategory_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request should have been made for an invalid Resources filter: %s", r.URL.Path)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{Resources: []string{"table"}})
	if err == nil {
		t.Fatal("expected an error for an unknown resource category")
	}
}

// TestCollect_ResourcesFilter_DedupesDuplicateEntries is a regression
// guard caught in self-review: "tables,tables" must produce the same
// Inventory.Collected as "tables" (length-1, not length-2) — otherwise
// two runs that requested identical coverage but happened to phrase
// --resources differently would spuriously trip
// compare.Result.CoverageMismatch(), which compares Collected by value.
func TestCollect_ResourcesFilter_DedupesDuplicateEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tablesdb" {
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{Resources: []string{"tables", "tables"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Collected) != 1 {
		t.Fatalf("expected duplicate \"tables\" entries deduped to 1, got %v", inv.Collected)
	}
}

func TestValidateResources(t *testing.T) {
	if err := ValidateResources(nil); err != nil {
		t.Fatalf("nil should be valid (means all), got %v", err)
	}
	if err := ValidateResources([]string{"tables", "users"}); err != nil {
		t.Fatalf("known categories should be valid, got %v", err)
	}
	if err := ValidateResources([]string{"functions", "bogus"}); err == nil {
		t.Fatal("expected an error for an unknown category")
	}
}
