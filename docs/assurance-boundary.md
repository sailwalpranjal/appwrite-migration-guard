# Assurance boundary

This document exists because an external audit specifically asked for
it: "what exactly does PASS prove, and what does it explicitly not
prove?" That question deserves a direct, written answer instead of one
assembled by a reader piecing together the README, comparison-model.md,
and source comments.

## What a PASS proves

For the two inventories amg actually captured, at the moment each was
captured: no difference was found in any of the checks listed in
[comparison-model.md](comparison-model.md#rules) — resource existence
(by `(Type, ID)`), name, parent, permissions, the resource-type-specific
config keys in `comparedMetadataKeys`, table schema (columns/indexes, via
digest), row counts (unless capped), file content (via Appwrite's MD5
signature), and — only if `--sample-rows` was passed — sampled row
content and row-level permissions within the sampled window.

That is the entire claim. It is a statement about two point-in-time
snapshots and a fixed set of checks, not about a migration process.

## What a PASS does not prove

- **That the migration is complete, consistent, or safe to cut over
  to.** This exact sentence is printed after every result (PASS
  included) by the CLI and HTML report — see `coverageNote` in
  `internal/cli/compare_report.go` — specifically so this isn't only
  discoverable by reading documentation.
- **That the manifest itself is complete.** A manifest is amg's own
  observation of the project via the Appwrite API, mediated by
  pagination. `internal/appwrite/pagination.go`'s `paginate` helper
  tracks every resource ID seen across every page (not just the last
  item of each page) and refuses to proceed silently if a page loses,
  duplicates, or fails to advance past a resource — see
  `TestListDatabases_LargeDataset_NoPageLossOrDuplication` and
  `TestListDatabases_DuplicateAcrossPages` in
  `internal/appwrite/pagination_test.go` for the regression tests this
  claim rests on. That closes the specific "silent incomplete page"
  failure mode an audit raised, but the deeper claim — "the Appwrite API
  itself returned everything that exists" — is trusted, not
  independently verified; amg has no way to audit Appwrite's own storage
  layer.
- **That amg's own collection run didn't partially fail.** `Collect`
  aborts (rather than returning a partial manifest) on a hard failure
  for resource types it must fully enumerate, and records per-resource
  soft failures (`CountError`, `SampleError`) that `compare` turns into
  WARN findings rather than treating a failed check as if it had
  passed — see `internal/inventory/collect.go`'s doc comment.
- **That the migration mechanism itself is safe**, independent of
  whether its output happens to look correct for the specific project
  state captured. amg compares source state to destination state; it
  has no visibility into the migration operation that produced the
  destination — no migration logs, retry counts, duration, or method.
  See [README#product-boundary](../README.md#product-boundary).
- **Application-level correctness**: referential integrity between
  resources, business-level uniqueness constraints, tenant-level data
  distribution, or anything specific to what a particular application
  means by its data. See
  [known-false-negatives.md](known-false-negatives.md) for concrete
  cases this implies.
- **That every Appwrite version behaves identically.** REST shapes are
  verified against `appwrite/appwrite` tag `2.3.0` — a snapshot, not a
  compatibility matrix across versions. A materially different
  self-hosted version may not match; see README#limitations.
- **Byte-for-byte independent content verification for files.** amg uses
  Appwrite's own server-computed MD5 signature rather than downloading
  and hashing file content itself (see comparison-model.md). This is a
  real optimization with a real cost: amg is trusting Appwrite's
  signature computation, not independently establishing byte equality.

## How to classify a specific claim in this repository

For any statement elsewhere in this repo phrased as "verified against
server source," "confirmed live," or similar, the underlying evidence is
one of:

- **SOURCE** — grounded in reading `appwrite/appwrite`'s PHP source at
  tag `2.3.0` (cited by file/class in a code comment near the claim).
- **LIVE** — reproduced against a real Appwrite Cloud project during
  this project's development (see `CHANGELOG.md` for when).
- **TEST** — proven by an automated test in this repository (unit,
  fixture-driven, or the fault-injection `lab/` package), runnable by
  anyone, not just asserted.
- **ASSUMPTION** — a stated precondition (e.g. "migration paths preserve
  row IDs") that amg relies on but does not itself verify.

Claims combining SOURCE and LIVE are the strongest (source explains
*why*, live confirms *that*); SOURCE alone is weaker (a documented
inference, not a demonstrated behavior) and is always labeled as such
where it appears — e.g. `docs/migration-semantics.md`'s note that the
legacy-Databases/TablesDB storage equivalence rests partly on
source-code inference for the collection/document level, because the
test API key used during development lacked the legacy `collections.read`
scope needed to reproduce that part live.

## Provenance recorded in a result

As of this stage, a saved `compare`/`verify` result
(`compare.Result`) records `SchemaVersion` and `PolicyName` — so a
reader knows which result-format version and which severity policy
(see comparison-model.md#policy) produced it. It does not yet record the
Appwrite server version of either side, or a hash of the manifest inputs
that produced it; both are legitimate, currently-unimplemented
provenance gaps rather than claims this document is making.
