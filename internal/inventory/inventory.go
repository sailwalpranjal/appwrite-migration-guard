// Package inventory builds a canonical, deterministic inventory of the
// resources in an Appwrite project. It depends only on internal/appwrite
// and internal/errs — it has no knowledge of comparison, manifests, or
// reporting, which are separate stages.
package inventory

import (
	"sort"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/version"
)

// ResourceType identifies the kind of Appwrite resource a Resource
// represents. The set is closed and grows only as amg adds real support
// for a resource type — see Inventory.Unsupported for types amg knows
// about but does not yet collect.
type ResourceType string

const (
	ResourceDatabase ResourceType = "database"
	ResourceTable    ResourceType = "table"
	ResourceBucket   ResourceType = "bucket"
	ResourceFile     ResourceType = "file"
	ResourceUser     ResourceType = "user"
	ResourceFunction ResourceType = "function"
	ResourceSite     ResourceType = "site"
)

// RowCountCap mirrors appwrite.rowCountCap: the point at which Appwrite
// stops computing an exact row total. A Resource with RowCountCapped=true
// has a RowCount that is a floor, not an exact figure — see
// docs/migration-semantics.md.
const RowCountCap = 5000

// Resource is amg's canonical representation of one Appwrite resource,
// independent of the raw Appwrite JSON it was built from (spec section
// 14). Fields that don't apply to a given ResourceType are left zero.
type Resource struct {
	Type        ResourceType   `json:"type"`
	ID          string         `json:"id"`
	ParentID    string         `json:"parent_id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Permissions []string       `json:"permissions,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`

	// RowCount and RowCountCapped only apply to ResourceTable: RowCount is
	// the server-reported total (see appwrite.CountRows), and
	// RowCountCapped is true when Appwrite stopped computing an exact
	// total at 5000 rows — in which case RowCount is a floor, not an
	// exact figure.
	RowCount       int  `json:"row_count,omitempty"`
	RowCountCapped bool `json:"row_count_capped,omitempty"`

	// CountError records that amg listed this resource but could not
	// determine its row count (e.g. a transient failure or a scope the
	// API key lacks). A non-empty CountError means this resource was only
	// partially verified — spec section 21/25: never claim success when a
	// check was skipped.
	CountError string `json:"count_error,omitempty"`

	// ContentDigest is a content hash for resources where Appwrite already
	// provides one without amg needing to download content. Currently only
	// ResourceFile sets this, to Appwrite's own server-computed MD5
	// ("signature") — see appwrite.File. Comparing this field catches
	// changed file content without ever transferring file bytes.
	ContentDigest string `json:"content_digest,omitempty"`

	// RowSamples holds a bounded, deterministic sample of row content
	// fingerprints for a ResourceTable — populated only when sampling is
	// enabled (see Options.SampleRows). This is a best-effort integrity
	// check, not exhaustive verification: a changed row outside the
	// sampled window is not detected. See docs/migration-semantics.md.
	RowSamples []RowSample `json:"row_samples,omitempty"`

	// SampleError records that amg attempted row sampling on this table
	// but could not complete it (e.g. a transient failure or a scope the
	// API key lacks) — partial verification, not a hard failure.
	SampleError string `json:"sample_error,omitempty"`
}

// RowSample is amg's canonical fingerprint of one sampled row: never the
// row's actual content, only its ID, its own row-level permissions
// (independent of the parent table's), and a content digest.
type RowSample struct {
	ID          string   `json:"id"`
	Permissions []string `json:"permissions,omitempty"`
	Digest      string   `json:"digest"`
}

// Inventory is a deterministic snapshot of a project's resources.
type Inventory struct {
	Endpoint    string     `json:"endpoint"`
	ProjectID   string     `json:"project_id"`
	GeneratedAt time.Time  `json:"generated_at"`
	ToolVersion string     `json:"tool_version"`
	Resources   []Resource `json:"resources"`

	// Unsupported lists resource types amg does not yet collect, named
	// explicitly rather than silently omitted (spec section 15).
	Unsupported []string `json:"unsupported"`
}

// unsupportedResourceTypes are Appwrite resource categories amg does not
// yet inventory. Keeping this list explicit means `amg inventory` can
// always tell a user "I did not check X" instead of staying silent.
//
// Empty as of this stage: every resource type from the original spec
// (databases/tables, storage, users, functions, sites) is now covered.
// Legacy "Databases" (collections/documents) never actually needed a
// separate entry here. At the database level this is confirmed live,
// not just by reading source: creating a database via POST /v1/tablesdb
// and then fetching it via GET /v1/databases (the legacy endpoint)
// returned a byte-for-byte identical response. At the table/collection
// level, amg has NOT reproduced the same live proof — the API key used
// for that test lacked the legacy `collections.read` scope (distinct
// from `tables.read`, even though both read the same data) — so that
// layer rests on source-code evidence only: both
// GET /v1/tablesdb/:id/tables and GET /v1/databases/:id/collections
// query the identical `database_{sequence}` store
// (github.com/appwrite/appwrite tag 2.3.0), and the legacy path is
// marked `Deprecated(since: '1.8.0')` in Appwrite's own SDK metadata.
// See docs/migration-semantics.md for the full account, including this
// caveat.
var unsupportedResourceTypes = []string{}

// New creates an empty Inventory stamped with the current tool version
// and generation time.
func New(endpoint, projectID string) *Inventory {
	unsupported := make([]string, len(unsupportedResourceTypes))
	copy(unsupported, unsupportedResourceTypes)
	return &Inventory{
		Endpoint:    endpoint,
		ProjectID:   projectID,
		GeneratedAt: time.Now().UTC(),
		ToolVersion: version.Version,
		Unsupported: unsupported,
	}
}

// Sort orders Resources deterministically by (Type, ParentID, ID), so two
// runs against an unchanged project produce byte-identical JSON (spec
// section 14: "the manifest must be deterministic... do not rely on API
// response ordering").
func (inv *Inventory) Sort() {
	sort.Slice(inv.Resources, func(i, j int) bool {
		a, b := inv.Resources[i], inv.Resources[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.ParentID != b.ParentID {
			return a.ParentID < b.ParentID
		}
		return a.ID < b.ID
	})
}

// CountByType returns the number of resources of each type, useful for
// terminal summaries.
func (inv *Inventory) CountByType() map[ResourceType]int {
	counts := make(map[ResourceType]int)
	for _, r := range inv.Resources {
		counts[r.Type]++
	}
	return counts
}

// PartiallyVerified reports whether any resource has a non-empty
// CountError or SampleError, meaning this inventory is incomplete despite
// not having failed outright.
func (inv *Inventory) PartiallyVerified() bool {
	for _, r := range inv.Resources {
		if r.CountError != "" || r.SampleError != "" {
			return true
		}
	}
	return false
}
