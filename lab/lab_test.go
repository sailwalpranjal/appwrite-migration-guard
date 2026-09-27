// Package lab is amg's fault-injection migration lab: a set of
// deterministic scenarios (spec section 33) that prove the comparison
// engine actually detects real integrity problems, correctly recognizes
// documented expected transformations, and never reports success on an
// incomplete run — run through the real client/pagination/retry/
// collection/comparison stack against httptest servers, not against
// mocked-out internals.
//
// This is deliberately synthetic, fixture-driven data — unlike the rest
// of amg's development, which was proven against a live Appwrite Cloud
// project at every stage (see README.md's Testing section and the
// CHANGELOG). The lab's whole purpose is the opposite: repeatable,
// CI-runnable, credential-free proof of specific edge cases, not a
// demonstration against real infrastructure. Both kinds of evidence are
// real; they answer different questions.
//
// Run just this package with: go test ./lab/...
package lab

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/cli"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func newClient(t *testing.T, url string, opts ...appwrite.Option) *appwrite.Client {
	t.Helper()
	env := config.Environment{Label: "target", Endpoint: url, ProjectID: "lab-project", APIKey: "lab-key"}
	return appwrite.New(env, opts...)
}

func collect(t *testing.T, url string, opts ...appwrite.Option) *inventory.Inventory {
	t.Helper()
	c := newClient(t, url, opts...)
	inv, err := inventory.Collect(context.Background(), c, url, "lab-project", inventory.Options{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	return inv
}

// emptyProjectHandler serves a project with no resources at all, for
// tests that only care about one specific resource type and want every
// other collection phase to succeed trivially.
func emptyProjectHandler(overrides map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h, ok := overrides[r.URL.Path]; ok {
			h(w, r)
			return
		}
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		case "/functions":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "functions": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func jsonHandler(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(v)
	}
}

// --- Scenario A: destination missing a resource -> BLOCK -----------------

func TestScenarioA_DestinationMissingTable_Blocks(t *testing.T) {
	src := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/tablesdb": jsonHandler(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}}),
		"/tablesdb/db1/tables": jsonHandler(map[string]any{"total": 1, "tables": []map[string]any{
			{"$id": "orders", "databaseId": "db1", "name": "Orders"},
		}}),
	}))
	defer src.Close()

	dst := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/tablesdb":            jsonHandler(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}}),
		"/tablesdb/db1/tables": jsonHandler(map[string]any{"total": 0, "tables": []any{}}),
	}))
	defer dst.Close()

	srcInv := collect(t, src.URL)
	dstInv := collect(t, dst.URL)

	res := compare.Compare("source", srcInv, "destination", dstInv)
	if res.Overall() != compare.SeverityBlock {
		t.Fatalf("scenario A: expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	found := false
	for _, f := range res.Findings {
		if f.Rule == compare.RuleMissingResource && f.ResourceID == "orders" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scenario A: expected a missing_resource finding for 'orders', got %+v", res.Findings)
	}
}

// --- Scenario B: destination missing a file -> BLOCK ----------------------

func TestScenarioB_DestinationMissingFile_Blocks(t *testing.T) {
	src := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/storage/buckets": jsonHandler(map[string]any{"total": 1, "buckets": []map[string]any{{"$id": "avatars", "name": "Avatars"}}}),
		"/storage/buckets/avatars/files": jsonHandler(map[string]any{"total": 1, "files": []map[string]any{
			{"$id": "logo.png", "bucketId": "avatars", "name": "logo.png", "signature": "abc123"},
		}}),
	}))
	defer src.Close()

	dst := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/storage/buckets":               jsonHandler(map[string]any{"total": 1, "buckets": []map[string]any{{"$id": "avatars", "name": "Avatars"}}}),
		"/storage/buckets/avatars/files": jsonHandler(map[string]any{"total": 0, "files": []any{}}),
	}))
	defer dst.Close()

	res := compare.Compare("source", collect(t, src.URL), "destination", collect(t, dst.URL))
	if res.Overall() != compare.SeverityBlock {
		t.Fatalf("scenario B: expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	found := false
	for _, f := range res.Findings {
		if f.Rule == compare.RuleMissingResource && f.ResourceID == "logo.png" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scenario B: expected a missing_resource finding for 'logo.png', got %+v", res.Findings)
	}
}

// --- Scenario C: permission changed -> BLOCK -------------------------------

func TestScenarioC_PermissionChanged_Blocks(t *testing.T) {
	table := func(perms []string) http.HandlerFunc {
		return jsonHandler(map[string]any{"total": 1, "tables": []map[string]any{
			{"$id": "orders", "databaseId": "db1", "name": "Orders", "$permissions": perms},
		}})
	}
	src := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/tablesdb":            jsonHandler(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}}),
		"/tablesdb/db1/tables": table([]string{`read("any")`}),
	}))
	defer src.Close()

	dst := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/tablesdb":            jsonHandler(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}}),
		"/tablesdb/db1/tables": table([]string{`read("users")`}),
	}))
	defer dst.Close()

	res := compare.Compare("source", collect(t, src.URL), "destination", collect(t, dst.URL))
	if res.Overall() != compare.SeverityBlock {
		t.Fatalf("scenario C: expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	found := false
	for _, f := range res.Findings {
		if f.Rule == compare.RulePermissionChanged {
			found = true
		}
	}
	if !found {
		t.Fatalf("scenario C: expected a permission_changed finding, got %+v", res.Findings)
	}
}

// --- Scenario D: expected timestamp transformation -> PASS ----------------

func TestScenarioD_ExpectedTimestampTransformation_Pass(t *testing.T) {
	table := func(createdAt string) http.HandlerFunc {
		return jsonHandler(map[string]any{"total": 1, "tables": []map[string]any{
			{"$id": "orders", "databaseId": "db1", "name": "Orders", "$createdAt": createdAt, "$updatedAt": createdAt},
		}})
	}
	src := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/tablesdb":            jsonHandler(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}}),
		"/tablesdb/db1/tables": table("2020-01-01T00:00:00.000Z"), // original migration source
	}))
	defer src.Close()

	dst := httptest.NewServer(emptyProjectHandler(map[string]http.HandlerFunc{
		"/tablesdb":            jsonHandler(map[string]any{"total": 1, "databases": []map[string]any{{"$id": "db1", "name": "Main"}}}),
		"/tablesdb/db1/tables": table(time.Now().UTC().Format(time.RFC3339)), // recreated "just now" by the migration
	}))
	defer dst.Close()

	res := compare.Compare("source", collect(t, src.URL), "destination", collect(t, dst.URL))
	if res.Overall() != compare.SeverityPass {
		t.Fatalf("scenario D: expected PASS (timestamps are an expected transformation, never compared), got %s (%+v)", res.Overall(), res.Findings)
	}
}

// --- Scenario E: transient API failure -> retries, then succeeds ----------

func TestScenarioE_TransientFailure_RetriesThenSucceeds(t *testing.T) {
	attempts := 0
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tablesdb" {
			attempts++
			if attempts < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]any{"message": "temporarily unavailable", "code": 503})
				return
			}
		}
		emptyProjectHandler(nil)(w, r)
	}))
	defer src.Close()

	client := newClient(t, src.URL, appwrite.WithMaxRetries(5), appwrite.WithBackoff(5*time.Millisecond))
	_, err := inventory.Collect(context.Background(), client, src.URL, "lab-project", inventory.Options{})
	if err != nil {
		t.Fatalf("scenario E: expected the transient failure to be retried away, got error: %v", err)
	}
	if attempts < 3 {
		t.Fatalf("scenario E: expected at least 3 attempts (2 failures + 1 success), got %d", attempts)
	}
}

// --- Scenario F: persistent API failure -> BLOCK, not silently ignored ----

func TestScenarioF_PersistentFailure_Blocks(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"message": "database unavailable", "code": 500})
	}))
	defer src.Close()

	client := newClient(t, src.URL, appwrite.WithMaxRetries(2), appwrite.WithBackoff(5*time.Millisecond))
	_, err := inventory.Collect(context.Background(), client, src.URL, "lab-project", inventory.Options{})
	if err == nil {
		t.Fatal("scenario F: expected Collect to return an error for a persistently failing endpoint, not silently succeed")
	}
}

// --- Scenario G: interrupted verification -> never reported as success ----

func TestScenarioG_InterruptedRun_NeverReportsSuccess(t *testing.T) {
	// A server that responds well after the caller's deadline, simulating
	// a verification run interrupted partway through. Sleeping (rather
	// than blocking on a channel released by a later defer) avoids a
	// deadlock against httptest.Server.Close(), which itself waits for
	// in-flight handlers to return.
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	client := newClient(t, src.URL)
	_, err := inventory.Collect(ctx, client, src.URL, "lab-project", inventory.Options{})
	if err == nil {
		t.Fatal("scenario G: expected an interrupted Collect to return an error, never a silently-partial success")
	}
}

// TestScenarioG_InterruptedDoctorRun_NeverExitsOK exercises the same
// principle at the full `amg doctor` CLI command layer: a run interrupted
// before it can complete any check must exit non-zero, never ExitOK.
func TestScenarioG_InterruptedDoctorRun_NeverExitsOK(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer src.Close()

	t.Setenv("APPWRITE_ENDPOINT", src.URL)
	t.Setenv("APPWRITE_PROJECT_ID", "lab-project")
	t.Setenv("APPWRITE_API_KEY", "lab-key")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var stdout, stderr bytes.Buffer
	code := cli.RunDoctor(ctx, nil, &stdout, &stderr)
	if code == cli.ExitOK {
		t.Fatalf("scenario G: an interrupted doctor run must never exit OK, got exit %d; stdout:\n%s", code, stdout.String())
	}
}
