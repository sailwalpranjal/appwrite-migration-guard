package appwrite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSampleRows_Disabled(t *testing.T) {
	c := New(testEnv("http://unused.invalid"))
	samples, err := c.SampleRows(context.Background(), "db1", "t1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if samples != nil {
		t.Fatalf("expected nil samples for limit<=0, got %v", samples)
	}
}

func TestSampleRows_DeterministicDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tablesdb/db1/tables/t1/rows" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"total": 1,
			"rows": []map[string]any{
				{
					"$id": "r1", "$sequence": 1, "$tableId": "t1", "$databaseId": "db1",
					"$createdAt": "2026-01-01T00:00:00Z", "$updatedAt": "2026-01-01T00:00:00Z",
					"$permissions": []string{"read(\"any\")"},
					"name":         "Alice", "age": 30,
				},
			},
		})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	samples, err := c.SampleRows(context.Background(), "db1", "t1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	s := samples[0]
	if s.ID != "r1" {
		t.Fatalf("expected ID r1, got %q", s.ID)
	}
	if len(s.Permissions) != 1 || s.Permissions[0] != `read("any")` {
		t.Fatalf("unexpected permissions: %v", s.Permissions)
	}
	if s.Digest == "" {
		t.Fatal("expected a non-empty digest")
	}
}

func TestSampleRows_DigestIgnoresManagedFieldsAndKeyOrder(t *testing.T) {
	makeServer := func(rows []map[string]any) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"total": len(rows), "rows": rows})
		}))
	}

	// Same logical row, but different $createdAt/$updatedAt (as would
	// happen after any migration that recreates the row) and different
	// key order in the raw JSON.
	srv1 := makeServer([]map[string]any{
		{"$id": "r1", "$createdAt": "2020-01-01T00:00:00Z", "$updatedAt": "2020-01-01T00:00:00Z", "name": "Alice", "age": 30},
	})
	defer srv1.Close()
	srv2 := makeServer([]map[string]any{
		{"age": 30, "name": "Alice", "$id": "r1", "$createdAt": "2026-09-27T00:00:00Z", "$updatedAt": "2026-09-27T00:00:00Z"},
	})
	defer srv2.Close()

	c1 := New(testEnv(srv1.URL))
	c2 := New(testEnv(srv2.URL))

	s1, err := c1.SampleRows(context.Background(), "db1", "t1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s2, err := c2.SampleRows(context.Background(), "db1", "t1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s1[0].Digest != s2[0].Digest {
		t.Fatalf("expected identical digests for logically-identical rows, got %s vs %s", s1[0].Digest, s2[0].Digest)
	}
}

func TestSampleRows_DigestChangesWithContent(t *testing.T) {
	makeServer := func(value string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"total": 1,
				"rows":  []map[string]any{{"$id": "r1", "name": value}},
			})
		}))
	}
	srv1 := makeServer("Alice")
	defer srv1.Close()
	srv2 := makeServer("Bob")
	defer srv2.Close()

	c1 := New(testEnv(srv1.URL))
	c2 := New(testEnv(srv2.URL))

	s1, _ := c1.SampleRows(context.Background(), "db1", "t1", 10)
	s2, _ := c2.SampleRows(context.Background(), "db1", "t1", 10)
	if s1[0].Digest == s2[0].Digest {
		t.Fatal("expected different digests for different row content")
	}
}

func TestSampleRows_LimitCappedAtMax(t *testing.T) {
	var gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, q := range r.URL.Query()["queries[]"] {
			if hasQueryMethod([]string{q}, "limit") {
				gotLimit = q
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"total": 0, "rows": []any{}})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	if _, err := c.SampleRows(context.Background(), "db1", "t1", 999999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"method":"limit","values":[500]}`
	if gotLimit != want {
		t.Fatalf("expected limit capped at maxSampleRows, got %s", gotLimit)
	}
}

func TestSampleRows_NeverIncludesRawContentInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// $id is not valid JSON for a string, forcing rowSampleFrom to error.
		w.Write([]byte(`{"total":1,"rows":[{"$id":12345,"secret_column":"super-secret-value"}]}`))
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	_, err := c.SampleRows(context.Background(), "db1", "t1", 10)
	if err == nil {
		t.Fatal("expected an error decoding a malformed $id")
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("row content leaked into error text: %v", err)
	}
}
