package appwrite

import (
	"context"
	"fmt"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

// pageSize is the number of items requested per page. Appwrite's list
// endpoints accept a "limit" query up to 5000, but a smaller page keeps
// memory bounded while inventorying large projects (spec section 22:
// process incrementally, never load an entire large dataset into RAM at
// once). 100 is a conservative default; it is not tuned for throughput.
const pageSize = 100

// fetchPage retrieves one page of resources of type T, given the cursor
// (empty for the first page). It must return items ordered consistently
// (callers pass OrderAsc("$id") in their query set) so cursorAfter
// pagination is well-defined.
type fetchPage[T any] func(ctx context.Context, cursor string) ([]T, error)

// paginate walks every page returned by fetch until a short page (fewer
// than pageSize items) or an empty page is seen. idOf extracts the
// resource ID used as the next cursor.
//
// It never silently truncates or duplicates results (spec section 16):
// every item ID seen across every page is tracked, not just each page's
// last item, so a server bug that repeats a resource on a later page is
// reported as a KindPagination error instead of silently producing a
// manifest with a duplicate or an undercount. This single check also
// covers the "stalled cursor" failure mode (a server that ignores
// cursorAfter and keeps returning the same page): the very first item of
// the repeated page is already a duplicate of something seen on the
// prior page, so it's caught before a second, separate "did the cursor
// advance" check would ever need to run — there is deliberately only one
// duplicate-detection path here, not two independent ones, to avoid the
// two silently drifting out of sync. Context cancellation is checked
// between pages.
func paginate[T any](ctx context.Context, op string, idOf func(T) string, fetch fetchPage[T]) ([]T, error) {
	var all []T
	cursor := ""
	seenIDs := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			return all, errs.New(errs.KindTimeout, op, ctx.Err())
		default:
		}

		page, err := fetch(ctx, cursor)
		if err != nil {
			return all, err
		}
		if len(page) == 0 {
			return all, nil
		}

		for _, item := range page {
			id := idOf(item)
			if id == "" || seenIDs[id] {
				return all, errs.New(errs.KindPagination, op,
					fmt.Errorf("pagination returned a duplicate or empty resource ID %q; aborting rather than producing an incomplete or duplicated manifest", id))
			}
			seenIDs[id] = true
		}
		all = append(all, page...)
		cursor = idOf(page[len(page)-1])

		if len(page) < pageSize {
			return all, nil
		}
	}
}
