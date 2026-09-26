package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListBuckets_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/buckets" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(bucketListResponse{
			Total:   2,
			Buckets: []Bucket{{ID: "b1", Name: "avatars"}, {ID: "b2", Name: "uploads"}},
		})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	buckets, err := c.ListBuckets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}
}

func TestListFiles_ReturnsSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/buckets/b1/files" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(fileListResponse{
			Total: 1,
			Files: []File{{ID: "f1", BucketID: "b1", Name: "logo.png", Signature: "5d529fd02b544198ae075bd57c1762bb", MimeType: "image/png", SizeOriginal: 1024}},
		})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	files, err := c.ListFiles(context.Background(), "b1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0].Signature != "5d529fd02b544198ae075bd57c1762bb" {
		t.Fatalf("unexpected files: %+v", files)
	}
}

func TestListFiles_MultiplePages(t *testing.T) {
	total := pageSize + 5
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		queries := mustDecodeQueries(t, r)
		cursor := ""
		for _, q := range queries {
			if hasQueryMethod([]string{q}, "cursorAfter") {
				var parsed struct {
					Values []string `json:"values"`
				}
				json.Unmarshal([]byte(q), &parsed)
				cursor = parsed.Values[0]
			}
		}
		start := 0
		if cursor != "" {
			for i := 0; i < total; i++ {
				if idFor(i) == cursor {
					start = i + 1
					break
				}
			}
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		files := make([]File, 0, end-start)
		for i := start; i < end; i++ {
			files = append(files, File{ID: idFor(i), BucketID: "b1"})
		}
		json.NewEncoder(w).Encode(fileListResponse{Total: total, Files: files})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	files, err := c.ListFiles(context.Background(), "b1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != total {
		t.Fatalf("expected %d files, got %d", total, len(files))
	}
	if requests != 2 {
		t.Fatalf("expected 2 page requests, got %d", requests)
	}
}

func idFor(i int) string {
	return fmt.Sprintf("f%04d", i)
}
