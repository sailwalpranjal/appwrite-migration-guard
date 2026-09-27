# Comparison model

`internal/compare` is amg's offline diff engine (`amg compare`, and the
live wrapper `amg verify`). It takes two `inventory.Inventory` snapshots
— never live Appwrite connections — and produces a list of `Finding`s,
each with a `Severity` (`PASS`/`WARN`/`BLOCK`), a stable `Rule` name, and a
human-readable `Message` that always says exactly what differed. There is
no numeric "safety score".

`Result.Overall()` is `BLOCK` if any finding is `BLOCK`, else `WARN` if
any finding is `WARN`, else `PASS`. An empty finding list is `PASS` —
unlike `cli.Checklist` (used for operational checks like `doctor`),
`Compare` only ever returns once it has fully finished, so there is no
"incomplete run" case to guard against here.

## Policy

Comparison *facts* (a resource is missing, a permission differs) and the
*policy* decision about how severely to treat a fact are deliberately
separated (`compare.Policy`). Earlier versions hard-coded a specific
policy choice directly into the comparison engine, with no way for a
caller operating under different constraints to disagree.

This is intentionally narrow: only `unexpected_resource` and its
row-level analogue `row_sample_unexpected` currently vary by policy,
because "the destination has something the source didn't" is the one
question in the table above with a genuinely debatable default — that
content may be intentional pre-existing content (a generic diff context)
or leaked/contaminated data (a pre-cutover production gate) — and it's
the same question at two resource granularities, so both move together
under one policy field rather than needing to be set independently.
Every other rule (`missing_resource`, `permission_changed`,
`schema_changed`, etc.) stays fixed at its listed severity: there is no
context this project can currently justify where a resource silently
vanishing, or its permissions silently changing, should be anything
other than BLOCK, so those didn't grow a policy knob with no demonstrated
use case behind it.

| Policy | `unexpected_resource` / `row_sample_unexpected` severity |
|---|---|
| `default` (`compare.DefaultPolicy()`) | WARN |
| `strict` (`compare.StrictPolicy()`, `--strict` on `compare`/`verify`) | BLOCK |

`Result.PolicyName` records which policy produced a given result, so a
saved `--json` result (and `amg report`) is self-describing about which
policy classified its findings, rather than requiring the reader to
already know how the run was invoked.

## Resource category coverage

`--resources` (on `inventory`/`snapshot`/`verify`/`preflight`) restricts
which resource categories (`tables`, `storage`, `users`, `functions`,
`sites`) a run collects at all — a category outside the filter is never
requested from Appwrite, so a narrowly-scoped API key (e.g. only
`databases.read`/`tables.read`/`rows.read`) doesn't hit a hard
authorization failure on a category it was never granted and the caller
never asked to check.

Comparing two inventories collected with different `--resources` filters
is meaningful but requires care: a resource type present on only the
unfiltered side isn't real drift, it's a side that was never asked to
check that category. `Result.SourceCollected`/`DestCollected` record
what each side actually requested, and `Result.CoverageMismatch()`
reports when they differ — surfaced as an explicit WARN-level note in
both terminal and HTML output, separate from the Findings list, so it's
never confused with an actual finding.

## Matching

Resources are matched between source and destination by `(Type, ID)`
only — not by list position, and not by `ParentID`. A resource that moved
to a different parent (e.g. a table now under a different database) is
still matched and reported via `parent_changed`, rather than appearing as
two unrelated "missing" + "unexpected" resources.

## Rules

| Rule | Severity | Meaning |
|---|---|---|
| `missing_resource` | BLOCK | Present in source, absent in destination. |
| `unexpected_resource` | WARN | Present in destination, absent in source — may be intentional pre-existing destination content, so it's flagged, not blocked. |
| `parent_changed` | BLOCK | Same resource ID, different parent (e.g. table moved to a different database). |
| `name_changed` | BLOCK | Same resource ID, different `name`. |
| `permission_changed` | BLOCK | `$permissions` differ (compared as sets — order doesn't matter). |
| `config_changed` | BLOCK | One of `enabled`/`row_security`/`type`/`status` metadata differs. |
| `schema_changed` | BLOCK | A table's column/index digest differs between source and destination — see [Schema verification](#schema-verification) below. |
| `schema_unverified` | WARN | A table's schema digest is missing on at least one side, so schema equality could not be confirmed either way. |
| `row_count_mismatch` | BLOCK | Table row counts differ and neither side hit Appwrite's count cap. |
| `row_count_unconfirmed` | WARN | Row counts differ (or can't be compared meaningfully) because at least one side hit Appwrite's 5,000-row count cap — see docs/migration-semantics.md. Equal capped counts on both sides produce no finding at all. |
| `row_count_unverified` | WARN | amg couldn't determine the row count on at least one side during inventory (see `Resource.CountError`) — a partial-verification case, not a hard failure. |
| `content_changed` | BLOCK | A file's content signature (Appwrite's own server-computed MD5) differs between source and destination — detected without downloading either file. |
| `content_unverified` | WARN | A file's content signature is missing on at least one side, so content equality could not be confirmed either way — never silently treated as a match. |
| `row_content_changed` | BLOCK | A sampled row's content digest differs between source and destination — see [Row content sampling](#row-content-sampling-optin) below. |
| `row_permission_changed` | BLOCK | A sampled row's own `$permissions` differ (rows have permissions independent of their table's). |
| `row_sample_missing` | WARN | A sampled source row's ID wasn't found in the destination's sample — could mean the row is genuinely gone, or just fell outside the sampled window. Not a confirmed loss. |
| `row_sample_unexpected` | WARN (BLOCK under `strict`) | The reverse: a destination row's ID wasn't in the source sample — policy-controlled, see [Policy](#policy). |
| `row_sample_unverified` | WARN | Row sampling itself failed on at least one side (`Resource.SampleError`) — partial verification, not a hard failure. |

`config_changed` compares different metadata keys depending on resource
type (`comparedMetadataKeys` in `compare.go`), so a bucket's
`file_security` is never compared against a table's `row_security`, etc:

| Resource type | Compared metadata keys |
|---|---|
| `database` | `enabled`, `type`, `status` |
| `table` | `enabled`, `row_security` |
| `bucket` | `enabled`, `file_security`, `maximum_file_size`, `allowed_file_extensions`, `compression`, `encryption`, `antivirus` |
| `file` | `mime_type` |
| `user` | `enabled`, `email_verification`, `phone_verification`, `mfa`, `labels` |
| `function` | `enabled`, `logging`, `runtime`, `scopes`, `events`, `schedule`, `timeout`, `entrypoint`, `deployment_retention`, `version` |

List-valued metadata (`allowed_file_extensions`, `labels`) is compared as
an order-independent set (`equalMetadataValue`), not a literal string —
Appwrite does not guarantee list ordering, so two semantically identical
lists returned in a different order must not produce a spurious
`config_changed` finding. This is checked in both a value's native
`[]string` form and its `[]any` form after a manifest round-trips through
JSON, since those decode differently.

## Deliberately excluded from comparison

`$createdAt`/`$updatedAt` are never compared. Appwrite's TablesDB create
endpoints (verified against `Create.php` for both databases and tables)
accept no client-supplied timestamp — every resource gets a
server-assigned creation time. Two independently created resources are
therefore *guaranteed* to differ here regardless of whether the migration
was correct, so comparing them would only ever produce noise, never
signal.

This is the project's only "expected transformation" rule so far — it is
structural (grounded in the API's own parameter list, not in assumptions
about how any particular migration tool behaves), and it is covered by
`TestCompare_TimestampsNeverCompared`. Future rules about *content*
transformations (e.g. what a specific migration path does to permissions
or IDs) will only be added once verified against that migration path's
actual behavior, each with its own source citation and test — never
guessed.

## Schema verification

Every table snapshot includes a `SchemaDigest`: a SHA-256 hash of that
table's columns and indexes, computed by `appwrite.ListTables` for every
table, always (not opt-in — unlike row sampling below). Appwrite's column
model is polymorphic across roughly 18 types (string, integer, enum,
relationship, etc., each with different fields), so rather than modeling
every variant explicitly, amg canonicalizes: for each column and index,
it strips transient/server-managed fields (`$id`, `$createdAt`,
`$updatedAt`, `status`, `error`), sorts entries by `key`, marshals with
Go's `encoding/json` (which sorts map keys), and hashes the result. This
means the digest changes if and only if something about the actual
column/index definitions changed — type, size, required-ness, default,
array-ness, a changed or removed index — and is stable across reordering,
re-fetching, or a column simply moving between "processing" and "available"
status mid-migration.

A `schema_changed` finding does not say *what* changed, only *that*
something did — the digest is one-way. This mirrors the same
digest-first, non-exhaustive-field-modeling approach already used for
file content (`content_changed`, via Appwrite's own MD5) and row content
(`row_content_changed`, below): amg detects drift reliably without
committing to modeling every field of every Appwrite resource type ahead
of time, which would both be a large surface to keep in sync with
Appwrite's own evolving API and a poor way to spend a $0-budget,
stdlib-only project's effort.

## Row content sampling (opt-in)

By default, TablesDB rows are only *counted* (`amg.CountRows`), never
inspected. Passing `--sample-rows N` to `inventory`/`snapshot`/`verify`
additionally fetches the first N rows per table (ordered by `$id`,
capped at 500 — see `appwrite.maxSampleRows`) and fingerprints each one
(`appwrite.RowSample`): its own row-level `$permissions`, and a SHA-256
digest of its user-defined column values. Appwrite prefixes every
system-managed field with `$` ($id, $sequence, $tableId, $databaseId,
$createdAt, $updatedAt, $permissions — confirmed across `Model/Row.php`
and its siblings), so stripping `$`-prefixed keys before hashing
reliably isolates real column data; the digest is computed with Go's
`encoding/json`, which sorts map keys, so it doesn't depend on the order
Appwrite returned fields in.

amg never stores or transmits the row content itself — only the
resulting digest and permission list end up in a manifest.

This is deliberately a **best-effort, non-exhaustive** check: because
only the first N rows (by ID) are sampled, a changed row outside that
window is invisible to it. Sampling is ordered by `$id` specifically so
the *same* rows are sampled on both sides of an unchanged table (making
comparison meaningful) — this assumes row IDs are preserved by whatever
migration path moved the data, the same assumption every other
ID-matched comparison in this document already makes.

## What this does not do

This list is deliberate, not a backlog — see the "Product boundary"
section of the README for why each of these is out of scope rather than
merely unimplemented.

No comparison of function or site *behavior* (only config: schedule,
runtime, execute permissions, etc. — not what a function's code actually
does, and not a runtime probe of either). No relationship-integrity
verification between resources (e.g. that a relationship column's
referenced rows still exist) — amg compares each resource's own state,
not cross-resource invariants. No application-level invariant checking
(anything specific to what your app *means* by its data). No persisted
run-to-run history beyond the manifest files themselves. No exhaustive
(non-sampled) row content verification — see Row content sampling below.
Legacy Databases (collections/documents) need no separate comparison
path: verified against Appwrite's server source that they query the
identical underlying storage TablesDB does, so comparing tables/rows
already covers them (see `Collect`'s doc comment in
`internal/inventory/collect.go` and docs/migration-semantics.md).
