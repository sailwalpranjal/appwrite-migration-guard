# Migration semantics

Documented Appwrite behavior that amg's inventory and (future)
normalization/comparison logic depend on. Every claim here is sourced from
`github.com/appwrite/appwrite` at tag `2.3.0` or from a live call against
an Appwrite Cloud project — not assumption.

## TablesDB vs. legacy Databases

Appwrite 2.x's current database API is **TablesDB** (`/v1/tablesdb/...`,
tables/rows). An older **Databases** API (`/v1/databases/...`,
collections/documents) still exists in the server source
(`src/Appwrite/Platform/Modules/Databases/Http/Databases/`) for backward
compatibility. amg's inventory currently covers TablesDB only; the legacy
API is listed under `Inventory.Unsupported` as
`legacy_databases_collections_documents` rather than silently skipped.

If your project still uses the legacy collections/documents API, `amg
inventory` will currently under-report your resources — this is a known
gap, not a silent failure (the JSON output always names it explicitly).

## Row counts are capped at 5,000

`GET /v1/tablesdb/:databaseId/tables/:tableId/rows` (and the equivalent
list endpoints elsewhere) stop computing an exact `total` once it exceeds
5,000 and return the literal value `5000` instead
(`src/Appwrite/Utopia/Response/Model/BaseList.php`: *"When the total
number of ... rows available is greater than 5000, total returned will be
capped at 5000, and cursor pagination should be used."*).

amg's `CountRows` surfaces this as `RowCountCapped: true` — a
`RowCount` of exactly 5000 is a **floor**, not an exact count. Any future
comparison logic must treat two capped counts as "cannot confirm equal,"
never as "confirmed equal."

## Public vs. authenticated endpoints — a real finding

`GET /v1/health/version` is declared `scope: public` in Appwrite's source,
meaning it's documented as requiring no authentication. Verified live
against an Appwrite Cloud project: sending an `X-Appwrite-Key` header on
this request does **not** get silently ignored — Appwrite evaluates the
request under that key's `applications` role and rejects it (401) for
lacking a literal `"public"` scope, which no API key can ever hold.

amg's reachability check (`Client.Version`) therefore sends no API key at
all, by design — see the comment on `requestAuth` in
`internal/appwrite/client.go`. This was caught by testing against a real
project, not anticipated in advance; if you find another Appwrite
endpoint with surprising auth-header sensitivity, please open an issue.

## File content verification without downloading files

Appwrite's File model (`Model/File.php`) includes a `signature` field —
"File MD5 signature" — computed and stored by the server itself when the
file is uploaded. amg reads this field as `Resource.ContentDigest` and
compares it directly (`content_changed` rule), which means file content
integrity can be verified across a migration without amg ever
downloading file bytes. This is deliberately different from — and safer
for large datasets than — the "sampled/full content hashing" approach
described in the project's own design notes, because Appwrite already
did the hashing.

Caveat: this only detects that content differs, not what changed, and it
depends on Appwrite's signature being recomputed correctly by whatever
migration path moved the file — amg has not independently verified that
claim beyond confirming the field exists and is populated on upload.

## Row content sampling assumes row IDs survive migration

`amg`'s opt-in row-content sampling (`--sample-rows N`, see
docs/comparison-model.md) compares the first N rows *by `$id`* on each
side. This only produces a meaningful comparison if a migration preserves
row IDs rather than reassigning new ones (e.g. via `ID.unique()`) — the
same assumption every other resource comparison in amg already makes
implicitly (databases/tables/buckets/files are all matched by ID too).
amg has not independently verified that any particular Appwrite migration
path preserves row IDs; if yours doesn't, sampled comparisons will show
spurious `row_sample_missing`/`row_sample_unexpected` findings rather
than real content differences.

## What is not yet defined

Only one *structural* normalization rule exists ($createdAt/$updatedAt
excluded — see docs/comparison-model.md), grounded in the TablesDB API
itself accepting no client-supplied timestamp. Normalization rules about
*specific migration tools'* behavior (e.g. "Appwrite's own migration
feature does X to permissions") do not exist yet, because amg has not
verified any specific migration tool's behavior against real before/after
data. This document will grow one rule at a time, each with its own
source citation and test, per the project's own rule against guessing
Appwrite behavior.
