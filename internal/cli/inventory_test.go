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

// Regression guard for the self-review finding: a row-sampling failure
// with a perfectly fine row count must never be reported as a row-count
// problem.
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
