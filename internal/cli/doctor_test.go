package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
)

func withEnv(t *testing.T, endpoint, project, key string) {
	t.Helper()
	for _, kv := range [][2]string{
		{"APPWRITE_ENDPOINT", endpoint},
		{"APPWRITE_PROJECT_ID", project},
		{"APPWRITE_API_KEY", key},
	} {
		old, had := os.LookupEnv(kv[0])
		os.Setenv(kv[0], kv[1])
		t.Cleanup(func() {
			if had {
				os.Setenv(kv[0], old)
			} else {
				os.Unsetenv(kv[0])
			}
		})
	}
}

func TestRunDoctor_MissingConfig(t *testing.T) {
	withEnv(t, "", "", "")
	var stdout, stderr bytes.Buffer
	code := RunDoctor(context.Background(), nil, &stdout, &stderr)
	if code != ExitWarn {
		t.Fatalf("expected ExitWarn, got %d; output:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("WARN")) {
		t.Fatalf("expected WARN in output:\n%s", stdout.String())
	}
}

func TestRunDoctor_HealthyEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/version":
			json.NewEncoder(w).Encode(appwrite.VersionInfo{Version: "2.3.0"})
		case "/health":
			json.NewEncoder(w).Encode(appwrite.HealthStatus{Name: "http", Status: "pass"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunDoctor(context.Background(), nil, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; output:\n%s", code, stdout.String())
	}
}

// TestRunDoctor_RegionMismatch_ExplainsCause is a regression guard for a
// real-world failure mode reproduced live against Appwrite Cloud 2.3.0: a
// valid project/key pointed at the wrong region's endpoint gets a 401 from
// the unauthenticated /health/version call, which looks like a network
// problem but is actually a routing mismatch. amg must surface Appwrite's
// own explanation plus a concrete fix, not a bare "401" string.
func TestRunDoctor_RegionMismatch_ExplainsCause(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"message": "Project is not accessible in this region. Please make sure you are using the correct endpoint",
			"code":    401,
			"type":    "general_access_forbidden",
			"version": "2.3.0",
		})
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunDoctor(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; output:\n%s", code, stdout.String())
	}
	out := stdout.String()
	for _, want := range []string{"not accessible in this region", "wrong Appwrite Cloud region"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestRunDoctor_JSONFlag_ProducesValidChecklistResult is a regression
// guard for a real documented limitation: `amg report`'s HTML output
// previously covered `compare`/`verify` results only, because `doctor`
// and `preflight` had no --json output at all to feed it in the first
// place. Proves --json produces a well-formed ChecklistResult with the
// schema version and command name `amg report` needs to render it.
func TestRunDoctor_JSONFlag_ProducesValidChecklistResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/version":
			json.NewEncoder(w).Encode(appwrite.VersionInfo{Version: "2.3.0"})
		case "/health":
			json.NewEncoder(w).Encode(appwrite.HealthStatus{Name: "http", Status: "pass"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunDoctor(context.Background(), []string{"--json"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stderr:\n%s", code, stderr.String())
	}

	var res ChecklistResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("expected valid JSON, got error %v; output:\n%s", err, stdout.String())
	}
	if res.SchemaVersion != ChecklistSchemaVersion {
		t.Fatalf("expected schema_version %d, got %d", ChecklistSchemaVersion, res.SchemaVersion)
	}
	if res.Command != "doctor" {
		t.Fatalf("expected command %q, got %q", "doctor", res.Command)
	}
	if res.Overall != StatusPass {
		t.Fatalf("expected overall PASS, got %s", res.Overall)
	}
	if len(res.Checks) == 0 {
		t.Fatal("expected at least one check")
	}
}

// TestRunDoctor_TimeoutFlag_IsConfigurable is a regression guard for a
// real launch-readiness gap: every live-network command's deadline
// (doctorTimeout, inventoryTimeout, etc.) was a hardcoded constant with
// no override, so a slow or high-latency Appwrite endpoint had no
// workaround short of rebuilding amg from source with a patched
// constant. Proves --timeout actually reaches the request deadline: a
// server that responds slower than the requested --timeout produces a
// timeout-shaped failure, not a hang or a silent success.
func TestRunDoctor_TimeoutFlag_IsConfigurable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		json.NewEncoder(w).Encode(appwrite.VersionInfo{Version: "2.3.0"})
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "key1")
	var stdout, stderr bytes.Buffer
	code := RunDoctor(context.Background(), []string{"--timeout", "10ms"}, &stdout, &stderr)
	if code == ExitOK {
		t.Fatalf("expected a short --timeout to cause a failure against a slow server, got ExitOK; output:\n%s", stdout.String())
	}
}

func TestRunDoctor_RejectedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/version":
			json.NewEncoder(w).Encode(appwrite.VersionInfo{Version: "2.3.0"})
		case "/health":
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"message": "invalid key", "code": 401})
		}
	}))
	defer srv.Close()

	withEnv(t, srv.URL, "proj1", "bad-key")
	var stdout, stderr bytes.Buffer
	code := RunDoctor(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; output:\n%s", code, stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("bad-key")) {
		t.Fatal("API key leaked into doctor output")
	}
}
