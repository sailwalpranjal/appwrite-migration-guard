package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

// Regression guard: after a manifest round-trips through JSON,
// column_count/index_count decode as float64, not int — metadataInt must
// handle both shapes, the same class of bug already fixed once for
// list-valued metadata (equalMetadataValue in internal/compare).
func TestMetadataInt_HandlesJSONRoundTrip(t *testing.T) {
	if got := metadataInt(3); got != 3 {
		t.Fatalf("native int: got %d, want 3", got)
	}
	if got := metadataInt(float64(3)); got != 3 {
		t.Fatalf("post-JSON float64: got %d, want 3", got)
	}
	if got := metadataInt(nil); got != 0 {
		t.Fatalf("absent value: got %d, want 0", got)
	}
}

func TestPartialVerificationReason(t *testing.T) {
	cases := []struct {
		name       string
		resources  []inventory.Resource
		wantReason string
	}{
		{"none", []inventory.Resource{{}}, ""},
		{"count only", []inventory.Resource{{CountError: "boom"}}, "some row counts were not verified"},
		{"sample only", []inventory.Resource{{SampleError: "boom"}}, "some row samples were not verified"},
		{"both on same table", []inventory.Resource{{CountError: "a", SampleError: "b"}}, "some row counts and row samples were not verified"},
		{"both across tables", []inventory.Resource{{CountError: "a"}, {SampleError: "b"}}, "some row counts and row samples were not verified"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv := &inventory.Inventory{Resources: tc.resources}
			if got := partialVerificationReason(inv); got != tc.wantReason {
				t.Fatalf("got %q, want %q", got, tc.wantReason)
			}
		})
	}
}

// Regression guard: a row-sampling failure with a perfectly fine row
// count must never be reported as a row-count problem.
func TestRunInventory_SampleFailureDoesNotClaimCountFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}})
		case "/tablesdb/db1/tables":
			json.NewEncoder(w).Encode(map[string]any{"total": 1, "tables": []map[string]any{{"$id": "t1", "databaseId": "db1", "name": "Widgets"}}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/tablesdb/db1/tables/t1/rows":
			q := r.URL.Query()["queries[]"]
			for _, v := range q {
				if v == `{"method":"limit","values":[1]}` {
					// CountRows path: succeed.
					json.NewEncoder(w).Encode(map[string]any{"total": 3, "rows": []any{}})
					return
				}
			}
			// SampleRows path (limit != 1): fail.
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"message": "forbidden", "code": 403})
		}
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunInventory(context.Background(), []string{"--sample-rows", "10"}, &stdout, &stderr)
	if code != ExitWarn {
		t.Fatalf("expected ExitWarn, got %d; stdout:\n%s", code, stdout.String())
	}
	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte("some row samples were not verified")) {
		t.Fatalf("expected row-sample-specific message, got:\n%s", out)
	}
	if bytes.Contains([]byte(out), []byte("row counts were not verified")) {
		t.Fatalf("must not claim a row-count failure when only sampling failed:\n%s", out)
	}
}

// TestRunInventory_ResourcesFlag_SkipsUnrequestedCategories is the
// CLI-level regression guard for --resources: a real user running
// `amg inventory --resources=tables` with an API key scoped only to
// databases.read/tables.read/rows.read (exactly what the README
// recommends granting for a tables-only check) must not hit a hard
// authorization failure on /users, /functions, or /sites — because
// those must never be requested at all when excluded.
func TestRunInventory_ResourcesFlag_SkipsUnrequestedCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/users", "/functions", "/sites", "/storage/buckets":
			t.Fatalf("category should have been excluded by --resources=tables: %s", r.URL.Path)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunInventory(context.Background(), []string{"--resources", "tables"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stdout:\n%s stderr:\n%s", code, stdout.String(), stderr.String())
	}
}

// TestRunInventory_ResourcesFlag_MarksSkippedCategories is a regression
// guard for a real ambiguity: without this, a category excluded via
// --resources prints "0 bucket(s)" — visually identical to a genuinely
// empty project — which misrepresents an intentionally narrowed run as
// having confirmed "nothing here." Skipped categories must say so.
func TestRunInventory_ResourcesFlag_MarksSkippedCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tablesdb" {
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunInventory(context.Background(), []string{"--resources", "tables"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK for an intentionally narrowed run, got %d; stdout:\n%s", code, stdout.String())
	}
	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte("bucket(s) (not requested")) {
		t.Fatalf("expected the bucket line to be marked as not requested, got:\n%s", out)
	}
	if bytes.Contains([]byte(out), []byte("database(s) (not requested")) {
		t.Fatalf("the requested 'tables' category must not be marked as skipped, got:\n%s", out)
	}
}

// TestRunInventory_ResourcesFlag_UnknownCategory_Blocks proves a typo
// in --resources is a loud configuration error, not a silently-ignored
// no-op that leaves a category unchecked without saying so.
func TestRunInventory_ResourcesFlag_UnknownCategory_Blocks(t *testing.T) {
	withEnv(t, "https://example.com/v1", "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunInventory(context.Background(), []string{"--resources", "table"}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock for an unknown resource category, got %d", code)
	}
}
