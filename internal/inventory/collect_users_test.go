package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func usersOnlyServer(t *testing.T, users []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]any{"total": len(users), "users": users})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollect_UsersPopulated(t *testing.T) {
	srv := usersOnlyServer(t, []map[string]any{
		{"$id": "u1", "name": "Alice", "status": true, "emailVerification": true, "labels": []string{"vip"}},
	})
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var user *Resource
	for i := range inv.Resources {
		if inv.Resources[i].Type == ResourceUser {
			user = &inv.Resources[i]
		}
	}
	if user == nil {
		t.Fatal("expected a user resource")
	}
	if user.Name != "Alice" {
		t.Fatalf("unexpected name: %q", user.Name)
	}
	if user.Metadata["enabled"] != true {
		t.Fatalf("unexpected enabled: %v", user.Metadata["enabled"])
	}
}

func TestCollect_UsersIsNoLongerUnsupported(t *testing.T) {
	srv := usersOnlyServer(t, nil)
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, u := range inv.Unsupported {
		if u == "users" {
			t.Fatal("users should no longer be listed as unsupported")
		}
	}
}

func TestCollect_ListUsersFailureAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "buckets": []any{}})
		case "/users":
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"message": "forbidden", "code": 403})
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err == nil {
		t.Fatal("expected an error when ListUsers fails")
	}
}
