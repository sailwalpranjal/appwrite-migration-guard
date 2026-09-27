package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func sitesOnlyServer(t *testing.T, sites []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		case "/functions":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "functions": []any{}})
		case "/sites":
			json.NewEncoder(w).Encode(map[string]any{"total": len(sites), "sites": sites})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollect_SitesPopulated(t *testing.T) {
	srv := sitesOnlyServer(t, []map[string]any{
		{"$id": "site1", "name": "Marketing", "enabled": true, "framework": "nextjs"},
	})
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var s *Resource
	for i := range inv.Resources {
		if inv.Resources[i].Type == ResourceSite {
			s = &inv.Resources[i]
		}
	}
	if s == nil {
		t.Fatal("expected a site resource")
	}
	if s.Name != "Marketing" {
		t.Fatalf("unexpected name: %q", s.Name)
	}
	if s.Metadata["framework"] != "nextjs" {
		t.Fatalf("unexpected framework: %v", s.Metadata["framework"])
	}
}

func TestCollect_ListSitesFailureAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		case "/functions":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "functions": []any{}})
		case "/sites":
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"message": "forbidden", "code": 403})
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err == nil {
		t.Fatal("expected an error when ListSites fails")
	}
}
