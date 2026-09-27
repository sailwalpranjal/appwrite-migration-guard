package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func functionsOnlyServer(t *testing.T, fns []map[string]any) *httptest.Server {
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
			json.NewEncoder(w).Encode(map[string]any{"total": len(fns), "functions": fns})
		case "/sites":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "sites": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollect_FunctionsPopulated(t *testing.T) {
	srv := functionsOnlyServer(t, []map[string]any{
		{"$id": "fn1", "name": "SendEmail", "enabled": true, "runtime": "node-18.0", "execute": []string{"users"}},
	})
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var fn *Resource
	for i := range inv.Resources {
		if inv.Resources[i].Type == ResourceFunction {
			fn = &inv.Resources[i]
		}
	}
	if fn == nil {
		t.Fatal("expected a function resource")
	}
	if fn.Name != "SendEmail" {
		t.Fatalf("unexpected name: %q", fn.Name)
	}
	if fn.Metadata["runtime"] != "node-18.0" {
		t.Fatalf("unexpected runtime: %v", fn.Metadata["runtime"])
	}
	if len(fn.Permissions) != 1 || fn.Permissions[0] != "users" {
		t.Fatalf("unexpected execute permissions: %v", fn.Permissions)
	}
}

func TestCollect_FunctionsIsNoLongerUnsupported(t *testing.T) {
	srv := functionsOnlyServer(t, nil)
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, u := range inv.Unsupported {
		if u == "functions" {
			t.Fatal("functions should no longer be listed as unsupported")
		}
	}
}

func TestCollect_ListFunctionsFailureAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "users": []any{}})
		case "/functions":
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"message": "forbidden", "code": 403})
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err == nil {
		t.Fatal("expected an error when ListFunctions fails")
	}
}
