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
	// SampleRows, when > 0, fetches up to this many rows per table
	// (ordered by $id, capped at appwrite's maxSampleRows) and records a
	// content digest for each instead of the actual content — see
	// appwrite.Client.SampleRows and docs/migration-semantics.md. Zero
	// (the default) disables sampling entirely: unlike row counts, this
	// reads real row data, so it is opt-in, not just cheap-by-default.
	SampleRows int
}

func (o Options) withDefaults() Options {
	if o.Concurrency <= 0 {
		o.Concurrency = DefaultConcurrency
	}
	return o
}

// Collect inventories every TablesDB database/table and every Storage
// bucket/file in the project client is configured for, plus (optionally)
// a row count per table. It returns an error (and no partial Inventory)
// if it cannot enumerate databases, tables, buckets, or files at all —
// those are hard requirements for the inventory to mean anything. Row
// count failures for individual tables are recorded on that Resource
// instead of aborting the whole run (spec section 21: partial
// verification is a WARN, not a failure of everything else already
// collected).
//
// Legacy Databases (collections/documents), Users, Functions, and Sites
// are not yet collected; they are listed in Inventory.Unsupported rather
// than silently omitted.
func Collect(ctx context.Context, client *appwrite.Client, endpoint, projectID string, opts Options) (*Inventory, error) {
	opts = opts.withDefaults()
	inv := New(endpoint, projectID)

	if err := collectTablesDB(ctx, client, inv, opts); err != nil {
		return nil, err
	}
	if err := collectStorage(ctx, client, inv, opts); err != nil {
		return nil, err
	}

	inv.Sort()
	return inv, nil
}

func collectTablesDB(ctx context.Context, client *appwrite.Client, inv *Inventory, opts Options) error {
	dbs, err := client.ListDatabases(ctx)
	if err != nil {
		return fmt.Errorf("list databases: %w", err)
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
		return err
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

	tableIdx := make([]int, 0, len(inv.Resources))
	for i, r := range inv.Resources {
		if r.Type == ResourceTable {
			tableIdx = append(tableIdx, i)
		}
	}

	// CountRows and SampleRows are independent per-table Appwrite calls,
	// so both run inside the same runBestEffort pass — one goroutine per
	// table issues whichever of the two are enabled, back to back —
	// instead of two full sequential passes over every table, which would
	// roughly double wall-clock time for no benefit.
	if opts.CountRows || opts.SampleRows > 0 {
		runBestEffort(ctx, opts.Concurrency, len(tableIdx), func(ctx context.Context, k int) {
			idx := tableIdx[k]
			r := &inv.Resources[idx]

			if opts.CountRows {
				count, capped, err := client.CountRows(ctx, r.ParentID, r.ID)
				if err != nil {
					r.CountError = err.Error()
				} else {
					r.RowCount = count
					r.RowCountCapped = capped
				}
			}

			if opts.SampleRows > 0 {
				samples, err := client.SampleRows(ctx, r.ParentID, r.ID, opts.SampleRows)
				if err != nil {
					r.SampleError = err.Error()
					return
				}
				r.RowSamples = make([]RowSample, len(samples))
				for i, s := range samples {
					r.RowSamples[i] = RowSample{ID: s.ID, Permissions: s.Permissions, Digest: s.Digest}
				}
			}
		})
	}

	return nil
}

func collectStorage(ctx context.Context, client *appwrite.Client, inv *Inventory, opts Options) error {
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("list buckets: %w", err)
	}

	filesByBucket := make([][]appwrite.File, len(buckets))
	err = runFailFast(ctx, opts.Concurrency, len(buckets), func(ctx context.Context, i int) error {
		files, err := client.ListFiles(ctx, buckets[i].ID)
		if err != nil {
			return fmt.Errorf("list files for bucket %q: %w", buckets[i].ID, err)
		}
		filesByBucket[i] = files
		return nil
	})
	if err != nil {
		return err
	}

	for i, b := range buckets {
		inv.Resources = append(inv.Resources, Resource{
			Type:        ResourceBucket,
			ID:          b.ID,
			Name:        b.Name,
			Permissions: b.Permissions,
			CreatedAt:   b.CreatedAt,
			UpdatedAt:   b.UpdatedAt,
			Metadata: map[string]any{
				"enabled":                 b.Enabled,
				"file_security":           b.FileSecurity,
				"maximum_file_size":       b.MaximumFileSize,
				"allowed_file_extensions": b.AllowedFileExtensions,
				"compression":             b.Compression,
				"encryption":              b.Encryption,
				"antivirus":               b.Antivirus,
			},
		})
		for _, f := range filesByBucket[i] {
			inv.Resources = append(inv.Resources, Resource{
				Type:          ResourceFile,
				ID:            f.ID,
				ParentID:      b.ID,
				Name:          f.Name,
				Permissions:   f.Permissions,
				CreatedAt:     f.CreatedAt,
				UpdatedAt:     f.UpdatedAt,
				ContentDigest: f.Signature,
				Metadata: map[string]any{
					"mime_type":     f.MimeType,
					"size_original": f.SizeOriginal,
				},
			})
		}
	}

	return nil
}
