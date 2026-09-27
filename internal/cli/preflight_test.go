package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// preflightServer builds a minimal TablesDB+Storage API reporting the
// given Appwrite version and a fixed set of database IDs (no tables, to
// keep these tests focused on preflight's own logic).
func preflightServer(t *testing.T, version string, databaseIDs []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/version":
			json.NewEncoder(w).Encode(map[string]any{"version": version})
		case "/health":
			json.NewEncoder(w).Encode(map[string]any{"name": "http", "status": "pass"})
		case "/tablesdb":
			dbs := make([]map[string]any, len(databaseIDs))
			for i, id := range databaseIDs {
				dbs[i] = map[string]any{"$id": id, "name": id}
			}
			json.NewEncoder(w).Encode(map[string]any{"total": len(dbs), "databases": dbs})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		case "/functions":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "functions": []any{}})
		case "/sites":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "sites": []any{}})
		default:
			if strings.HasPrefix(r.URL.Path, "/tablesdb/") && strings.HasSuffix(r.URL.Path, "/tables") {
				json.NewEncoder(w).Encode(map[string]any{"total": 0, "tables": []any{}})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRunPreflight_MissingConfig(t *testing.T) {
	withSourceDestEnv(t, "", "")
	var stdout, stderr bytes.Buffer
	code := RunPreflight(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stdout:\n%s", code, stdout.String())
	}
}

// No conflicts still ends as WARN, not PASS: amg always discloses that it
// cannot inspect legacy Databases/Users/Functions/Sites for conflicts
// either, rather than staying silent about that blind spot (spec
// section 19: "resource types that may require manual handling").
// Now that every original-spec resource type is inventoried,
// Inventory.Unsupported is empty and preflight no longer has anything to
// warn about when there are no ID conflicts — a clean PASS, not noise.
func TestRunPreflight_NoConflicts_Pass(t *testing.T) {
	src := preflightServer(t, "2.3.0", []string{"db-a"})
	defer src.Close()
	dst := preflightServer(t, "2.3.0", []string{"db-b"})
	defer dst.Close()
	withSourceDestEnv(t, src.URL, dst.URL)

	var stdout, stderr bytes.Buffer
	code := RunPreflight(context.Background(), nil, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stdout:\n%s stderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("No destination resource ID conflicts")) {
		t.Fatalf("expected a no-conflicts line in output:\n%s", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("Resource types requiring manual verification")) {
		t.Fatalf("did not expect a manual-verification warning now that Unsupported is empty:\n%s", stdout.String())
	}
}

func TestRunPreflight_DestinationConflict_Blocks(t *testing.T) {
	src := preflightServer(t, "2.3.0", []string{"db-shared"})
	defer src.Close()
	dst := preflightServer(t, "2.3.0", []string{"db-shared"})
	defer dst.Close()
	withSourceDestEnv(t, src.URL, dst.URL)

	var stdout, stderr bytes.Buffer
	code := RunPreflight(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stdout:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Destination conflict")) {
		t.Fatalf("expected a Destination conflict finding in output:\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"db-shared"`)) {
		t.Fatalf("expected the conflicting resource ID named in output:\n%s", stdout.String())
	}
}

func TestRunPreflight_VersionMismatch_Warns(t *testing.T) {
	src := preflightServer(t, "2.3.0", nil)
	defer src.Close()
	dst := preflightServer(t, "2.2.0", nil)
	defer dst.Close()
	withSourceDestEnv(t, src.URL, dst.URL)

	var stdout, stderr bytes.Buffer
	code := RunPreflight(context.Background(), nil, &stdout, &stderr)
	if code != ExitWarn {
		t.Fatalf("expected ExitWarn, got %d; stdout:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Appwrite version match")) {
		t.Fatalf("expected a version mismatch warning in output:\n%s", stdout.String())
	}
}

func TestRunPreflight_UnreachableSource_Blocks(t *testing.T) {
	dst := preflightServer(t, "2.3.0", nil)
	defer dst.Close()
	withSourceDestEnv(t, "http://127.0.0.1:1", dst.URL) // nothing listens here

	var stdout, stderr bytes.Buffer
	code := RunPreflight(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stdout:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Source endpoint reachable")) {
		t.Fatalf("expected a source-unreachable finding in output:\n%s", stdout.String())
	}
	// Regression guard: an unreachable source must not also be re-dialed
	// for an authentication check — that produces a confusing second
	// finding for the same root cause and wastes a retry cycle.
	if bytes.Contains(stdout.Bytes(), []byte("Source authentication")) {
		t.Fatalf("did not expect a separate Source authentication finding when the endpoint was already unreachable:\n%s", stdout.String())
	}
}

func TestRunPreflight_UnauthenticatedDestination_SkipsInventory(t *testing.T) {
	src := preflightServer(t, "2.3.0", nil)
	defer src.Close()
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/version":
			json.NewEncoder(w).Encode(map[string]any{"version": "2.3.0"})
		case "/health":
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"message": "invalid key", "code": 401})
		}
	}))
	defer dst.Close()
	withSourceDestEnv(t, src.URL, dst.URL)

	var stdout, stderr bytes.Buffer
	code := RunPreflight(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stdout:\n%s", code, stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("resource inventory")) {
		t.Fatalf("did not expect inventory to run after failed authentication:\n%s", stdout.String())
	}
}
