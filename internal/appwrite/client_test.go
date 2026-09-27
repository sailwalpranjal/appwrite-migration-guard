package appwrite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestHealth_RateLimit_RespectsServerRetryAfter is a regression test for
// a real bug: Retry-After was parsed into the error *message* but never
// actually used to time the next retry attempt, which always used amg's
// own exponential backoff regardless of what the server asked for. This
// proves the client now waits at least the server-specified delay before
// retrying a 429, not just amg's own (much shorter, in this test) guess.
func TestHealth_RateLimit_RespectsServerRetryAfter(t *testing.T) {
	calls := 0
	var firstCallAt, secondCallAt time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			firstCallAt = time.Now()
			w.Header().Set("Retry-After", "1") // 1 real second
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(apiError{Message: "rate limited", Code: 429})
			return
		}
		secondCallAt = time.Now()
		json.NewEncoder(w).Encode(HealthStatus{Name: "http", Status: "pass"})
	}))
	defer srv.Close()

	// A backoff far shorter than the server's Retry-After: if the client
	// used its own backoff instead of honoring the header, the second
	// call would arrive almost immediately.
	c := New(testEnv(srv.URL), WithMaxRetries(2), WithBackoff(time.Millisecond))
	_, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	gap := secondCallAt.Sub(firstCallAt)
	if gap < 900*time.Millisecond {
		t.Fatalf("expected the client to wait ~1s per Retry-After before retrying, only waited %v", gap)
	}
}

// TestHealth_RateLimit_RetryAfterZero_RetriesImmediately is a regression
// test for a bug caught in self-review: errs.RetryAfterOf used
// `e.RetryAfter > 0` to decide whether a server-suggested delay was
// present, which silently treated a genuine "Retry-After: 0" the same as
// "no header at all" and fell back to amg's own (here, much longer)
// backoff. A server sending Retry-After: 0 is asking for an immediate
// retry, so the client should not wait out its configured backoff first.
func TestHealth_RateLimit_RetryAfterZero_RetriesImmediately(t *testing.T) {
	calls := 0
	var firstCallAt, secondCallAt time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			firstCallAt = time.Now()
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(apiError{Message: "rate limited", Code: 429})
			return
		}
		secondCallAt = time.Now()
		json.NewEncoder(w).Encode(HealthStatus{Name: "http", Status: "pass"})
	}))
	defer srv.Close()

	// A backoff much longer than the (zero) Retry-After: if the client
	// ignored the header and fell back to its own backoff, the second
	// call would arrive after this delay, not immediately.
	c := New(testEnv(srv.URL), WithMaxRetries(2), WithBackoff(2*time.Second))
	_, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	gap := secondCallAt.Sub(firstCallAt)
	if gap > 500*time.Millisecond {
		t.Fatalf("expected an immediate retry per Retry-After: 0, waited %v (configured backoff was 2s)", gap)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		header string
		want   time.Duration
		wantOK bool
	}{
		{"5", 5 * time.Second, true},
		{"0", 0, true}, // a valid, if degenerate, delay-seconds value
		{"", 0, false},
		{"not-a-number", 0, false},
		{"-1", 0, false},
		{"999999", maxRetryAfter, true}, // capped
	}
	for _, tc := range cases {
		got, ok := parseRetryAfter(tc.header)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("parseRetryAfter(%q) = %v, %v; want %v, %v", tc.header, got, ok, tc.want, tc.wantOK)
		}
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

// TestHealth_ValidationError_SurfacesRealMessage is a regression guard
// grounded in a real, verified Appwrite bug: appwrite/appwrite#13477
// ("Self-hosted -> self-hosted migration fails at report stage —
// Missing required field 'policies' for Appwrite\Models\Database is
// masked as 'Unable to connect to the migration source'"). There, the
// masking happened server-side — Appwrite's own
// Migrations/Appwrite/Report/Get.php caught the real exception (a
// missing `policies` field) and rethrew a generic connectivity message,
// so the 400 response a client receives was already masked before it
// left Appwrite. amg has no way to recover a message Appwrite never
// sent, but it must not compound the problem: whatever specific message
// the server *did* send has to survive into amg's own returned error
// text, not be replaced by a generic "validation failed" or
// status-code-only string on amg's side.
func TestHealth_ValidationError_SurfacesRealMessage(t *testing.T) {
	const specificMessage = `Missing required field "policies" for Appwrite\Models\Database`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(apiError{Message: specificMessage, Code: 400})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	_, err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), specificMessage) {
		t.Fatalf("expected the server's specific validation message to survive in the error text, got: %v", err)
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
