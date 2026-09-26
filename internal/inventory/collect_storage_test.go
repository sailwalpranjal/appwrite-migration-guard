package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// storageOnlyServer serves an empty TablesDB (no databases) and a
// controllable Storage API, so storage collection can be tested in
// isolation from TablesDB collection.
func storageOnlyServer(t *testing.T, buckets []map[string]any, filesByBucket map[string][]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case r.URL.Path == "/storage/buckets":
			json.NewEncoder(w).Encode(map[string]any{"total": len(buckets), "buckets": buckets})
		case strings.HasPrefix(r.URL.Path, "/storage/buckets/") && strings.HasSuffix(r.URL.Path, "/files"):
			bucketID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/storage/buckets/"), "/files")
			files := filesByBucket[bucketID]
			json.NewEncoder(w).Encode(map[string]any{"total": len(files), "files": files})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollect_StorageBucketsAndFiles(t *testing.T) {
	buckets := []map[string]any{
		{"$id": "avatars", "name": "Avatars", "enabled": true, "fileSecurity": false},
	}
	files := map[string][]map[string]any{
		"avatars": {
			{"$id": "f1", "bucketId": "avatars", "name": "logo.png", "signature": "5d529fd02b544198ae075bd57c1762bb", "mimeType": "image/png", "sizeOriginal": 1024},
		},
	}
	srv := storageOnlyServer(t, buckets, files)
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var bucket, file *Resource
	for i := range inv.Resources {
		switch inv.Resources[i].ID {
		case "avatars":
			bucket = &inv.Resources[i]
		case "f1":
			file = &inv.Resources[i]
		}
	}
	if bucket == nil || bucket.Type != ResourceBucket {
		t.Fatalf("expected a bucket resource, got %+v", inv.Resources)
	}
	if file == nil || file.Type != ResourceFile {
		t.Fatalf("expected a file resource, got %+v", inv.Resources)
	}
	if file.ParentID != "avatars" {
		t.Fatalf("expected file parent to be the bucket ID, got %q", file.ParentID)
	}
	if file.ContentDigest != "5d529fd02b544198ae075bd57c1762bb" {
		t.Fatalf("expected ContentDigest to be the file signature, got %q", file.ContentDigest)
	}
}

func TestCollect_StorageIsNoLongerUnsupported(t *testing.T) {
	srv := storageOnlyServer(t, nil, nil)
	defer srv.Close()

	client := newTestClient(srv.URL)
	inv, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, u := range inv.Unsupported {
		if u == "storage_buckets_files" {
			t.Fatal("storage should no longer be listed as unsupported")
		}
	}
}

func TestCollect_ListBucketsFailureAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tablesdb":
			json.NewEncoder(w).Encode(map[string]any{"total": 0, "databases": []any{}})
		case "/storage/buckets":
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"message": "forbidden", "code": 403})
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, err := Collect(context.Background(), client, srv.URL, "proj1", Options{})
	if err == nil {
		t.Fatal("expected an error when ListBuckets fails")
	}
}
