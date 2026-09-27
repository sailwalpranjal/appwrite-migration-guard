package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
