# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added

- Functions inventory and comparison (config only: runtime, schedule,
  timeout, execute permissions, logging, scopes, deployment retention,
  version — never behavior/code). `appwrite.Function` deliberately has
  no field for `vars` (function environment variables), which routinely
  hold secrets — same pattern as User's excluded fields.
  Self code-reviewed before landing; fixed 4 findings: a stale doc
  comment on `Collect` still claiming Functions were unsupported, two
  collected-but-never-compared metadata fields
  (`deployment_retention`/`version` were in `Resource.Metadata` but
  missing from `comparedMetadataKeys`, so real config drift there would
  have silently passed), and a stale "not yet" line in
  docs/comparison-model.md.
  Verified live against Appwrite Cloud: created a real function (whose
  raw API response included an explicit `"vars":[]` field, confirming
  the shape this exclusion is designed against), changed its schedule
  and execute permissions, and `amg compare` correctly reported both —
  grepping both manifests for `vars`/secret-shaped content found
  nothing.
- Users inventory and comparison. `appwrite.User` deliberately has no
  field for `password`/`hash`/`hashOptions`/`email`/`phone`/`prefs` —
  even though Appwrite's raw response can include all of them — so
  `json.Unmarshal` silently drops them; there is nothing for that data
  to decode into. Only administrative/verification state is collected
  (enabled/disabled, email/phone verification, MFA, labels) and
  compared (`config_changed`).
  Self code-reviewed before landing; fixed a real bug it found:
  list-valued metadata (a user's `labels`, a bucket's
  `allowed_file_extensions`) was compared with a literal string
  comparison, which is order-sensitive — Appwrite doesn't guarantee list
  ordering, so semantically identical lists returned in a different
  order would have produced a spurious BLOCK. Fixed with an
  order-independent comparison (`equalMetadataValue`) that also handles
  the `[]any` shape a list takes after a manifest round-trips through
  JSON. The review also caught that this stage's docs didn't actually
  explain the PII-omission rationale despite code comments pointing to
  them — fixed in docs/migration-semantics.md.
  Verified live against Appwrite Cloud: created a real user (confirmed
  by inspecting the raw API response directly that it included a live
  argon2 password hash and an email address), ran `amg inventory --json`
  and grepped the output and the saved manifest file for the
  hash/email/phone — zero matches in either. Disabled the user and added
  a label, snapshotted again, and `amg compare` correctly reported both
  changes with no PII anywhere in either manifest.
- `amg report <result.json>`: renders a saved `compare`/`verify --json`
  result as text (default), pretty-printed JSON, or a self-contained
  HTML file — no external stylesheet, script, or network request, safe
  to open offline. Built on Go's `html/template` (auto-escaping), with a
  dedicated test proving resource names/messages containing `<script>`
  tags render as inert escaped text, not executable markup. Every
  registered `amg` command is now a real implementation — `stub.go` and
  the "not yet implemented" placeholder are gone.
  `compare.Result` gained a `SchemaVersion` field (`ResultSchemaVersion`
  constant) so a future JSON-shape change won't be silently
  misinterpreted by an older `amg report`.
  Verified live: piped a real `amg compare --json` result (from a
  deliberately changed live table) through `amg report --format html`
  and inspected the output file directly — correct badge, correct
  findings table, correctly escaped content, zero external references.

### Fixed

- Caught and corrected a real mistake mid-implementation: this stage's
  first draft of `amg report` was written to `internal/cli/report.go`,
  silently overwriting the pre-existing `Checklist`/`Status` types that
  file already held (used by `doctor`/`preflight`) instead of extending
  them. Caught immediately via a failed build, restored the original
  file from git, and moved the new command to `report_cmd.go`. No data
  or history was lost; noted here because it's the kind of mistake that
  should be visible, not quietly swept under a squashed commit.

- Opt-in TablesDB row content verification (`--sample-rows N` on
  `inventory`/`snapshot`/`verify`), directly addressing the previously
  documented "row content comparison: only counts" limitation.
  `appwrite.Client.SampleRows` fetches up to N rows (capped at 500,
  ordered by `$id` for determinism) and fingerprints each one — a
  SHA-256 digest of user-defined column values (every Appwrite-managed
  field is `$`-prefixed and excluded) plus the row's own `$permissions`
  — never storing or transmitting row content itself. New comparison
  rules: `row_content_changed`/`row_permission_changed` (BLOCK, a
  confirmed difference within the sample) and
  `row_sample_missing`/`row_sample_unexpected`/`row_sample_unverified`
  (WARN — sampling is inherently partial, so absence from a sample is
  never treated as confirmed loss).
  Self code-reviewed before landing; fixed two findings: CountRows and
  SampleRows ran as two full sequential passes over every table instead
  of one combined pass (roughly doubling wall-clock time), and a
  row-sampling failure was misreported in the terminal summary as "row
  counts were not verified" even when counting succeeded fine.
  Verified live: created a table with two rows, snapshotted with
  `--sample-rows 10`, edited one row's content, snapshotted again, and
  `amg compare` correctly reported `row_content_changed` for exactly the
  changed row — with `$updatedAt` drift on both rows correctly producing
  no finding.
- `amg preflight`: pre-migration readiness checks against
  `AMG_SOURCE_*`/`AMG_DEST_*` — connectivity, authentication, Appwrite
  version match, and a `Destination conflict` check for resource IDs that
  already exist on the destination *before* any migration (a collision
  risk, unlike `verify`'s post-migration "should already match"
  semantics). Source/destination inventory now runs concurrently (fixed
  during self-review — it was serial and could starve the second side's
  timeout budget on a large source project). Also fixed during
  self-review: an unreachable side was re-dialed a second time for an
  authentication check it could never pass, producing a duplicate,
  confusing finding.
  Verified live: pointed source and destination at the same project with
  an existing database and confirmed `amg preflight` reported a
  `Destination conflict` BLOCK naming the exact colliding resource;
  confirmed bad credentials produce the specific "API key was rejected"
  reason via a new `explainAuthError` helper shared with `amg doctor`.
- Added `.gitattributes` forcing LF line endings for Go source — fixes
  `gofmt -l` spuriously flagging every file as unformatted on Windows
  checkouts with no actual content difference.
- Storage inventory: `amg inventory`/`snapshot`/`compare`/`verify` now
  cover buckets and files, in addition to TablesDB. Files carry their
  Appwrite-computed MD5 `signature` as `Resource.ContentDigest`, compared
  via a new `content_changed` rule — file content integrity is verified
  without amg ever downloading a file. `config_changed` comparison is now
  type-scoped per resource type (`comparedMetadataKeys`), fixing a latent
  bug where a bucket's config keys could have been silently checked
  against unrelated resource types.
  Verified live: uploaded a file, snapshotted it, replaced its content
  under the same file ID, snapshotted again, and `amg compare` correctly
  reported the exact signature change.
- `internal/manifest`: deterministic on-disk snapshot format
  (`.amg/runs/<run-id>/manifest.json`), no credential field ever.
- `internal/compare`: offline PASS/WARN/BLOCK comparison engine — 9 rules
  (`missing_resource`, `unexpected_resource`, `parent_changed`,
  `name_changed`, `permission_changed`, `config_changed`,
  `row_count_mismatch`, `row_count_unconfirmed`, `row_count_unverified`),
  each with its own test. See docs/comparison-model.md.
- `amg snapshot`: persists an inventory run as a manifest.
- `amg compare <a> <b>`: fully offline comparison between two manifests —
  no Appwrite credentials needed at all.
- `amg verify`: live comparison — inventories `AMG_SOURCE_*` and
  `AMG_DEST_*` concurrently, then runs the same comparison engine as
  `compare`. This is the primary "did my migration work" command.
- `docs/comparison-model.md`: the full rule table.
- Full loop verified against a live Appwrite Cloud project: snapshot,
  mutate (permission change, config change, delete a table), snapshot
  again, `amg compare` correctly reported each change and nothing else;
  `amg verify` run live against the same project as both sides correctly
  reported PASS.

- `amg inventory`: real TablesDB inventory (databases, tables, per-table
  row counts) with `--json`, `--no-row-counts`, `--concurrency` flags.
  Verified against a live Appwrite Cloud project (server version 2.3.0),
  not just mocked HTTP.
- `internal/inventory`: canonical `Resource`/`Inventory` model,
  deterministic sorting, bounded-concurrency collector with fail-fast
  listing and best-effort per-table row counting.
- `internal/appwrite`: `ListDatabases`/`ListTables`/`CountRows`, a shared
  cursor-pagination walker with malformed-cursor and cancellation
  handling, tests for one-page/multi-page/empty-page/stuck-cursor/
  transient-mid-pagination-failure/cancellation scenarios.
- `docs/migration-semantics.md`: documented Appwrite behaviors amg's
  inventory depends on, including the 5,000-row count cap and a real bug
  found during live testing (see Fixed).

### Fixed

- `Client.Version` (`GET /health/version`) no longer sends the API key.
  Found by testing against a live Appwrite Cloud project: Appwrite
  evaluates a request carrying `X-Appwrite-Key` under that key's role and
  rejects "scope: public" endpoints for lacking a `"public"` scope no key
  can hold — so a public reachability check must never send a key.
- Context cancellation while reading an HTTP response body was
  misclassified as `KindInvalidResponse` instead of `KindTimeout`.

- CLI skeleton (`cmd/amg`) with subcommand dispatch and exit codes
  (0=PASS, 1=WARN, 2=BLOCK).
- `amg version` and `amg doctor` (real implementations).
- `amg inventory`/`preflight`/`snapshot`/`verify`/`report`/`compare`
  registered as commands but explicitly not yet implemented.
- Minimal Appwrite REST client (`internal/appwrite`): auth headers,
  bounded retries with backoff, typed error classification, health/
  version checks, Appwrite query-string builder.
- Environment-variable configuration loading (`internal/config`),
  including source/destination pairs and optional `.env` support.
- Typed error taxonomy (`internal/errs`).
- PASS/WARN/BLOCK reporting model (`internal/cli/report.go`).
