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
// It never silently truncates results (spec section 16): if the cursor
// fails to advance — which would otherwise spin forever re-fetching the
// same page — it returns a KindPagination error instead of looping, and
// context cancellation is checked between pages.
func paginate[T any](ctx context.Context, op string, idOf func(T) string, fetch fetchPage[T]) ([]T, error) {
	var all []T
	cursor := ""
	seen := make(map[string]bool)

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

		all = append(all, page...)

		last := idOf(page[len(page)-1])
		if last == "" || last == cursor || seen[last] {
			return all, errs.New(errs.KindPagination, op,
				fmt.Errorf("pagination cursor did not advance (got %q); aborting to avoid an infinite loop", last))
		}
		seen[last] = true
		cursor = last

		if len(page) < pageSize {
			return all, nil
		}
	}
}
