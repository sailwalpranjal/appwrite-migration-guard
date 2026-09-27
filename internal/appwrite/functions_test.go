package appwrite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListFunctions_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/functions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(functionListResponse{
			Total: 2,
			Functions: []Function{
				{ID: "fn1", Name: "SendEmail", Enabled: true, Runtime: "node-18.0"},
				{ID: "fn2", Name: "Cleanup", Enabled: false, Schedule: "0 0 * * *"},
			},
		})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	fns, err := c.ListFunctions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}
}

// Regression guard: even if a real Appwrite server includes "vars" (env
// vars, which routinely hold secrets) in the raw JSON, Function must
// never surface them.
func TestListFunctions_NeverDecodesVars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total":1,"functions":[{
			"$id":"fn1","name":"SendEmail","enabled":true,"runtime":"node-18.0",
			"vars":[{"key":"STRIPE_SECRET_KEY","value":"sk_live_should_never_appear"}]
		}]}`))
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	fns, err := c.ListFunctions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fns) != 1 {
		t.Fatalf("expected 1 function, got %d", len(fns))
	}
	b, err := json.Marshal(fns[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "sk_live_should_never_appear") {
		t.Fatalf("secret leaked into decoded Function: %s", string(b))
	}
	if strings.Contains(string(b), "STRIPE_SECRET_KEY") {
		t.Fatalf("env var key leaked into decoded Function: %s", string(b))
	}
}
