package appwrite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

func testEnv(url string) config.Environment {
	return config.Environment{Label: "target", Endpoint: url, ProjectID: "proj1", APIKey: "secret-key"}
}

func TestVersion_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/version" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(VersionInfo{Version: "2.3.0"})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Version != "2.3.0" {
		t.Fatalf("got version %q", v.Version)
	}
}

// TestVersion_NeverSendsAPIKey is a regression test for a real bug found
// while testing against a live Appwrite Cloud project: attaching
// X-Appwrite-Key to a "scope: public" request (like /health/version) makes
// Appwrite evaluate the call under the key's role and reject it for
// lacking a "public" scope that no key can ever hold. Version() must
// never send the key, regardless of whether one is configured.
func TestVersion_NeverSendsAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Appwrite-Key"); got != "" {
			t.Fatalf("Version must never send X-Appwrite-Key, got %q", got)
		}
		json.NewEncoder(w).Encode(VersionInfo{Version: "2.3.0"})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	if _, err := c.Version(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHealth_AuthHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Appwrite-Project"); got != "proj1" {
			t.Fatalf("X-Appwrite-Project = %q", got)
		}
		if got := r.Header.Get("X-Appwrite-Key"); got != "secret-key" {
			t.Fatalf("X-Appwrite-Key = %q", got)
		}
		json.NewEncoder(w).Encode(HealthStatus{Name: "http", Status: "pass"})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Status != "pass" {
		t.Fatalf("got status %q", h.Status)
	}
}

func TestHealth_Unauthorized_NotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apiError{Message: "invalid key", Code: 401, Type: "user_unauthorized"})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL), WithMaxRetries(3))
	_, err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindAuthentication) {
		t.Fatalf("expected KindAuthentication, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call (no retry), got %d", calls)
	}
}

func TestHealth_RateLimit_RetriesThenSucceeds(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(apiError{Message: "rate limited", Code: 429})
			return
		}
		json.NewEncoder(w).Encode(HealthStatus{Name: "http", Status: "pass"})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL), WithMaxRetries(5))
	c.backoff = time.Millisecond
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if h.Status != "pass" {
		t.Fatalf("got status %q", h.Status)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestHealth_ServerError_RetriesExhausted(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(apiError{Message: "boom", Code: 500})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL), WithMaxRetries(2))
	c.backoff = time.Millisecond
	_, err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindServer) {
		t.Fatalf("expected KindServer, got %v", err)
	}
	if calls != 3 { // initial + 2 retries
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestHealth_ValidationError_NotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(apiError{Message: "bad request", Code: 400})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL), WithMaxRetries(3))
	_, err := c.Health(context.Background())
	if !errs.IsKind(err, errs.KindValidation) {
		t.Fatalf("expected KindValidation, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRequest_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(HealthStatus{Status: "pass"})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	c := New(testEnv(srv.URL))
	_, err := c.Health(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindTimeout) {
		t.Fatalf("expected KindTimeout, got %v", err)
	}
}

func TestQueryParams_Encoding(t *testing.T) {
	q := QueryParams(Limit(25), CursorAfter("abc123"))
	got := q["queries[]"]
	if len(got) != 2 {
		t.Fatalf("expected 2 query entries, got %d: %v", len(got), got)
	}
	if got[0] != `{"method":"limit","values":[25]}` {
		t.Fatalf("unexpected limit encoding: %s", got[0])
	}
	if got[1] != `{"method":"cursorAfter","values":["abc123"]}` {
		t.Fatalf("unexpected cursorAfter encoding: %s", got[1])
	}
}
