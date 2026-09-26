package inventory

import (
	"context"
	"fmt"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
)

// DefaultConcurrency bounds how many Appwrite requests amg issues at once
// during collection. Kept conservative by default to respect API rate
// limits on shared/free-tier projects (spec section 23).
const DefaultConcurrency = 4

// Options configures a Collect run.
type Options struct {
	// Concurrency bounds in-flight requests per collection phase. <= 0
	// uses DefaultConcurrency.
	Concurrency int
	// CountRows controls whether amg fetches a row count per table. This
	// is metadata-only (a single "total" field), never row content — see
	// appwrite.Client.CountRows. Disable for very large projects where
	// even this is undesirable.
	CountRows bool
}

func (o Options) withDefaults() Options {
	if o.Concurrency <= 0 {
		o.Concurrency = DefaultConcurrency
	}
	return o
}

// Collect inventories every TablesDB database, table, and (optionally) row
// count in the project client is configured for. It returns an error (and
// no partial Inventory) if it cannot enumerate databases or tables at all
// — those are hard requirements for the inventory to mean anything. Row
// count failures for individual tables are recorded on that Resource
// instead of aborting the whole run (spec section 21: partial
// verification is a WARN, not a failure of everything else already
// collected).
//
// Legacy Databases (collections/documents), Storage, Users, Functions, and
// Sites are not yet collected; they are listed in Inventory.Unsupported
// rather than silently omitted.
func Collect(ctx context.Context, client *appwrite.Client, endpoint, projectID string, opts Options) (*Inventory, error) {
	opts = opts.withDefaults()
	inv := New(endpoint, projectID)

	dbs, err := client.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}

	tablesByDB := make([][]appwrite.Table, len(dbs))
	err = runFailFast(ctx, opts.Concurrency, len(dbs), func(ctx context.Context, i int) error {
		tables, err := client.ListTables(ctx, dbs[i].ID)
		if err != nil {
			return fmt.Errorf("list tables for database %q: %w", dbs[i].ID, err)
		}
		tablesByDB[i] = tables
		return nil
	})
	if err != nil {
		return nil, err
	}

	for i, db := range dbs {
		inv.Resources = append(inv.Resources, Resource{
			Type:      ResourceDatabase,
			ID:        db.ID,
			Name:      db.Name,
			CreatedAt: db.CreatedAt,
			UpdatedAt: db.UpdatedAt,
			Metadata: map[string]any{
				"enabled": db.Enabled,
				"type":    db.Type,
				"status":  db.Status,
			},
		})
		for _, tbl := range tablesByDB[i] {
			inv.Resources = append(inv.Resources, Resource{
				Type:        ResourceTable,
				ID:          tbl.ID,
				ParentID:    db.ID,
				Name:        tbl.Name,
				Permissions: tbl.Permissions,
				CreatedAt:   tbl.CreatedAt,
				UpdatedAt:   tbl.UpdatedAt,
				Metadata: map[string]any{
					"enabled":      tbl.Enabled,
					"row_security": tbl.RowSecurity,
					"bytes_max":    tbl.BytesMax,
					"bytes_used":   tbl.BytesUsed,
				},
			})
		}
	}

	if opts.CountRows {
		tableIdx := make([]int, 0, len(inv.Resources))
		for i, r := range inv.Resources {
			if r.Type == ResourceTable {
				tableIdx = append(tableIdx, i)
			}
		}
		runBestEffort(ctx, opts.Concurrency, len(tableIdx), func(ctx context.Context, k int) {
			idx := tableIdx[k]
			r := &inv.Resources[idx]
			count, capped, err := client.CountRows(ctx, r.ParentID, r.ID)
			if err != nil {
				r.CountError = err.Error()
				return
			}
			r.RowCount = count
			r.RowCountCapped = capped
		})
	}

	inv.Sort()
	return inv, nil
}
