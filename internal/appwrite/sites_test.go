package appwrite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListSites_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(siteListResponse{
			Total: 1,
			Sites: []Site{{ID: "site1", Name: "Marketing", Framework: "nextjs", Enabled: true}},
		})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	sites, err := c.ListSites(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sites) != 1 || sites[0].Framework != "nextjs" {
		t.Fatalf("unexpected sites: %+v", sites)
	}
}

// Regression guard: even if a real Appwrite server includes "vars" (env
// vars, which routinely hold build-time secrets) in the raw JSON, Site
// must never surface them.
func TestListSites_NeverDecodesVars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total":1,"sites":[{
			"$id":"site1","name":"Marketing","enabled":true,"framework":"nextjs",
			"vars":[{"key":"DEPLOY_TOKEN","value":"should_never_appear_ghp_xxx"}]
		}]}`))
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	sites, err := c.ListSites(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := json.Marshal(sites[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "should_never_appear_ghp_xxx") {
		t.Fatalf("secret leaked into decoded Site: %s", string(b))
	}
	if strings.Contains(string(b), "DEPLOY_TOKEN") {
		t.Fatalf("env var key leaked into decoded Site: %s", string(b))
	}
}
