package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

// TestListDatabases_LargeDataset_NoPageLossOrDuplication is a regression
// guard against the single most dangerous inventory bug class: a manifest
// that looks complete but silently lost or duplicated a page. It walks
// 10,001 resources — well past any small "a few pages" fixture — across
// ~101 pages and verifies every single ID is accounted for exactly once,
// in order, with no gaps.
func TestListDatabases_LargeDataset_NoPageLossOrDuplication(t *testing.T) {
	const total = 10_001
	all := make([]Database, total)
	for i := range all {
		all[i] = Database{ID: fmt.Sprintf("db%06d", i), Name: fmt.Sprintf("db-%d", i)}
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		queries := mustDecodeQueries(t, r)
		cursor := ""
		for _, q := range queries {
			if strings.Contains(q, `"method":"cursorAfter"`) {
				var parsed struct {
					Values []string `json:"values"`
				}
				json.Unmarshal([]byte(q), &parsed)
				if len(parsed.Values) > 0 {
					cursor = parsed.Values[0]
				}
			}
		}
		start := 0
		if cursor != "" {
			for i, d := range all {
				if d.ID == cursor {
					start = i + 1
					break
				}
			}
		}
		end := start + pageSize
		if end > len(all) {
			end = len(all)
		}
		json.NewEncoder(w).Encode(databaseListResponse{Total: len(all), Databases: all[start:end]})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	got, err := c.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != total {
		t.Fatalf("expected %d resources, got %d (page loss or truncation)", total, len(got))
	}
	wantPages := (total + pageSize - 1) / pageSize
	if requests != wantPages {
		t.Fatalf("expected %d page requests, got %d", wantPages, requests)
	}

	seen := make(map[string]bool, total)
	for i, d := range got {
		if d.ID != all[i].ID {
			t.Fatalf("result out of order at index %d: got %s want %s", i, d.ID, all[i].ID)
		}
		if seen[d.ID] {
			t.Fatalf("duplicate resource ID %q in result", d.ID)
		}
		seen[d.ID] = true
	}
}

// TestListDatabases_DuplicateAcrossPages covers a server bug that repeats
// a resource on a later page while still advancing the cursor correctly
// (e.g. a cursor implementation with an off-by-one, or a resource that
// briefly reappeared after being re-indexed mid-list). The old pagination
// guard only checked whether the *last* item of each page repeated the
// previous cursor — it would miss this entirely and silently double-count
// the resource in the manifest. amg must instead refuse the manifest.
func TestListDatabases_DuplicateAcrossPages(t *testing.T) {
	page1 := make([]Database, pageSize)
	for i := range page1 {
		page1[i] = Database{ID: fmt.Sprintf("db%04d", i)}
	}
	// Page 2 repeats db0005 (from page 1) instead of continuing after
	// the page-1 cursor, then continues normally — the cursor itself
	// still advances at the end of the page, so the old "did the last
	// item change" guard alone would not catch this.
	page2 := append([]Database{{ID: "db0005"}}, func() []Database {
		rest := make([]Database, pageSize-1)
		for i := range rest {
			rest[i] = Database{ID: fmt.Sprintf("db1%04d", i)}
		}
		return rest
	}()...)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			json.NewEncoder(w).Encode(databaseListResponse{Total: 999, Databases: page1})
			return
		}
		json.NewEncoder(w).Encode(databaseListResponse{Total: 999, Databases: page2})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	_, err := c.ListDatabases(context.Background())
	if err == nil {
		t.Fatal("expected a pagination error for a duplicated resource ID across pages")
	}
	if !errs.IsKind(err, errs.KindPagination) {
		t.Fatalf("expected KindPagination, got %v", err)
	}
}
