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
| `row_count_mismatch` | BLOCK | Table row counts differ and neither side hit Appwrite's count cap. |
| `row_count_unconfirmed` | WARN | Row counts differ (or can't be compared meaningfully) because at least one side hit Appwrite's 5,000-row count cap — see docs/migration-semantics.md. Equal capped counts on both sides produce no finding at all. |
| `row_count_unverified` | WARN | amg couldn't determine the row count on at least one side during inventory (see `Resource.CountError`) — a partial-verification case, not a hard failure. |
| `content_changed` | BLOCK | A file's content signature (Appwrite's own server-computed MD5) differs between source and destination — detected without downloading either file. |
| `content_unverified` | WARN | A file's content signature is missing on at least one side, so content equality could not be confirmed either way — never silently treated as a match. |
| `row_content_changed` | BLOCK | A sampled row's content digest differs between source and destination — see [Row content sampling](#row-content-sampling-optin) below. |
| `row_permission_changed` | BLOCK | A sampled row's own `$permissions` differ (rows have permissions independent of their table's). |
| `row_sample_missing` | WARN | A sampled source row's ID wasn't found in the destination's sample — could mean the row is genuinely gone, or just fell outside the sampled window. Not a confirmed loss. |
| `row_sample_unexpected` | WARN | The reverse: a destination row's ID wasn't in the source sample. |
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

## What this does not do yet

No comparison for legacy Databases/Users/Functions/Sites (they aren't
inventoried yet — see `Inventory.Unsupported`), no persisted run-to-run
history beyond the manifest files themselves, no exhaustive (non-sampled)
row content verification.
