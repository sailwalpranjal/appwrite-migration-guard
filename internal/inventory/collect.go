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

// Collect inventories every TablesDB database/table, Storage bucket/
// file, User, Function, and Site in the project client is configured
// for, plus (optionally) a row count per table. It returns an error
// (and no partial Inventory) if it cannot enumerate a resource type at
// all — that's a hard requirement for the inventory to mean anything.
// Row count failures for individual tables are recorded on that
// Resource instead of aborting the whole run (spec section 21: partial
// verification is a WARN, not a failure of everything else already
// collected).
//
// Legacy Databases (collections/documents) need no separate collection
// step: verified against Appwrite's own server source that they query
// the identical underlying storage TablesDB does — see
// docs/migration-semantics.md and the doc comment on
// unsupportedResourceTypes in inventory.go.
func Collect(ctx context.Context, client *appwrite.Client, endpoint, projectID string, opts Options) (*Inventory, error) {
	opts = opts.withDefaults()
	inv := New(endpoint, projectID)

	if err := collectTablesDB(ctx, client, inv, opts); err != nil {
		return nil, err
	}
	if err := collectStorage(ctx, client, inv, opts); err != nil {
		return nil, err
	}
	if err := collectUsers(ctx, client, inv); err != nil {
		return nil, err
	}
	if err := collectFunctions(ctx, client, inv); err != nil {
		return nil, err
	}
	if err := collectSites(ctx, client, inv); err != nil {
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
				Type:         ResourceTable,
				ID:           tbl.ID,
				ParentID:     db.ID,
				Name:         tbl.Name,
				Permissions:  tbl.Permissions,
				CreatedAt:    tbl.CreatedAt,
				UpdatedAt:    tbl.UpdatedAt,
				SchemaDigest: tbl.SchemaDigest,
				Metadata: map[string]any{
					"enabled":      tbl.Enabled,
					"row_security": tbl.RowSecurity,
					"bytes_max":    tbl.BytesMax,
					"bytes_used":   tbl.BytesUsed,
					"column_count": tbl.ColumnCount,
					"index_count":  tbl.IndexCount,
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

// collectUsers lists every user in the project. Metadata is
// deliberately limited to administrative/verification state
// (enabled/disabled, verification flags, MFA, labels) — email, phone,
// and prefs are never collected, even though the raw Appwrite response
// can include them, because they are personally identifiable or
// arbitrary application data amg has no structural need to read. See
// docs/migration-semantics.md.
func collectUsers(ctx context.Context, client *appwrite.Client, inv *Inventory) error {
	users, err := client.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	for _, u := range users {
		inv.Resources = append(inv.Resources, Resource{
			Type:      ResourceUser,
			ID:        u.ID,
			Name:      u.Name,
			CreatedAt: u.CreatedAt,
			UpdatedAt: u.UpdatedAt,
			Metadata: map[string]any{
				"enabled":            u.Status,
				"email_verification": u.EmailVerification,
				"phone_verification": u.PhoneVerification,
				"mfa":                u.MFA,
				"labels":             u.Labels,
			},
		})
	}
	return nil
}

// collectFunctions lists every function in the project. Environment
// variables ("vars") are never fetched or collected — Appwrite's own
// model documents them as function environment variables, which
// routinely hold secrets (API keys, database passwords, tokens). See
// docs/migration-semantics.md.
func collectFunctions(ctx context.Context, client *appwrite.Client, inv *Inventory) error {
	fns, err := client.ListFunctions(ctx)
	if err != nil {
		return fmt.Errorf("list functions: %w", err)
	}
	for _, f := range fns {
		inv.Resources = append(inv.Resources, Resource{
			Type:        ResourceFunction,
			ID:          f.ID,
			Name:        f.Name,
			Permissions: f.Execute,
			CreatedAt:   f.CreatedAt,
			UpdatedAt:   f.UpdatedAt,
			Metadata: map[string]any{
				"enabled":              f.Enabled,
				"logging":              f.Logging,
				"runtime":              f.Runtime,
				"scopes":               f.Scopes,
				"events":               f.Events,
				"schedule":             f.Schedule,
				"timeout":              f.Timeout,
				"entrypoint":           f.Entrypoint,
				"deployment_retention": f.DeploymentRetention,
				"version":              f.Version,
			},
		})
	}
	return nil
}

// collectSites lists every site in the project. Environment variables
// ("vars") are never fetched or collected, for the same reason as
// Functions: they routinely hold build-time secrets. Sites have no
// $permissions/execute field in Appwrite's model at all (they're
// typically public-facing), so Resource.Permissions is left empty.
func collectSites(ctx context.Context, client *appwrite.Client, inv *Inventory) error {
	sites, err := client.ListSites(ctx)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}
	for _, s := range sites {
		inv.Resources = append(inv.Resources, Resource{
			Type:      ResourceSite,
			ID:        s.ID,
			Name:      s.Name,
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
			Metadata: map[string]any{
				"enabled":              s.Enabled,
				"logging":              s.Logging,
				"framework":            s.Framework,
				"scopes":               s.Scopes,
				"timeout":              s.Timeout,
				"install_command":      s.InstallCommand,
				"build_command":        s.BuildCommand,
				"start_command":        s.StartCommand,
				"output_directory":     s.OutputDirectory,
				"build_runtime":        s.BuildRuntime,
				"adapter":              s.Adapter,
				"fallback_file":        s.FallbackFile,
				"deployment_retention": s.DeploymentRetention,
			},
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
