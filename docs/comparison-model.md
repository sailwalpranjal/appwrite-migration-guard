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

## What this does not do yet

No content/row diffing (rows are only ever counted, never fetched), no
legacy Databases/Storage/Users/Functions/Sites comparison (they aren't
inventoried yet — see `Inventory.Unsupported`), no persisted
run-to-run history beyond the manifest files themselves.
