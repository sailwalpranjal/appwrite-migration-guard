# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added

- `--resources tables,storage,users,functions,sites` on `inventory`,
  `snapshot`, `verify`, and `preflight`, restricting collection to
  specific categories — a category outside the filter is never
  requested from Appwrite at all, so a narrowly-scoped API key (e.g.
  only `databases.read`/`tables.read`/`rows.read`, exactly what the
  README recommends granting for a tables-only check) doesn't hit a
  hard authorization failure on a category it was never granted and
  never asked to check. Previously every command required all scopes
  or failed completely, in direct tension with the README's own
  least-privilege advice. `Inventory.Collected` records which
  categories a run actually requested (persisted in manifests);
  `amg inventory`'s terminal summary marks a skipped category
  explicitly ("not requested — see --resources") instead of printing
  an indistinguishable "0" a genuinely empty project would also show;
  `compare.Result.CoverageMismatch()` (surfaced as a WARN note in
  terminal/HTML output, separate from Findings) flags when source and
  destination requested different categories, since a resource type
  present on only the unfiltered side isn't real drift.
  Self code-reviewed before landing; the review caught two real
  issues, both fixed: `preflight`'s "No destination resource ID
  conflicts" PASS gave no indication the check was narrowed by
  `--resources`, so a real collision in an unchecked category would
  have gone completely unflagged with an unqualified all-clear (fixed
  by disclosing the requested categories in that PASS message); and
  `--resources tables,tables` produced an `Inventory.Collected` with a
  duplicate entry, which could spuriously trip `CoverageMismatch()`
  against an equivalent run whose flag happened not to repeat itself
  (fixed by deduping in `Options.withDefaults()`, not just at the CLI
  flag-parsing layer, so any caller is protected).
  Verified live against a real Appwrite Cloud project: `amg inventory
  --resources=tables` and `--resources=bogus` (confirming the loud
  configuration-error path) both behaved correctly; attempting to
  provision a genuinely narrowly-scoped API key to prove the "avoids a
  hard authorization failure" half live was blocked by a real Appwrite
  Cloud constraint — API keys cannot be created from a request
  authenticated with another API key (session-based console auth is
  required), so that specific half rests on the httptest-level proof
  in `TestCollect_ResourcesFilter_NeverRequestsExcludedCategories`
  instead, which is disclosed here rather than left unstated.
- `--timeout <duration>` on every live-network command (`doctor`,
  `inventory`, `snapshot`, `verify`, `preflight`) — the deadline was
  previously a hardcoded constant (10s-3min) with no override, a real
  gap for a tool meant to run against arbitrarily large real projects.
- Two `lab/` fault-injection scenarios modeled on real Appwrite bugs:
  a table that loses a column mid-migration (appwrite/appwrite#12770)
  now triggers `schema_changed`, and a masked server validation error
  (appwrite/appwrite#13477) is proven not to get further masked by
  amg's own error classification (`TestHealth_ValidationError_SurfacesRealMessage`).
- HTML report redesign: filled status badges, zebra-striped/hoverable
  table rows, a sticky client-side filter/search toolbar (vanilla JS,
  degrades cleanly with JS off), an explicit light/dark toggle
  (persisted via `localStorage`) alongside automatic
  `prefers-color-scheme`, a per-resource-type findings breakdown table,
  dark-mode-appropriate WARN/BLOCK colors, and a horizontal-scroll
  wrapper so the findings table doesn't break on narrow viewports.
- Fixed `amg <cmd> --help`/`-h` exiting 2 instead of 0 for every
  subcommand — it printed correct usage text but reported the same exit
  code as a real failure. A bad flag still exits BLOCK as before.
- Reordered `amg --help`'s command list to match the README's
  walkthrough order (doctor, inventory, snapshot, compare, verify,
  preflight, report).
- `.github/ISSUE_TEMPLATE/`, `.github/PULL_REQUEST_TEMPLATE.md`,
  `CODE_OF_CONDUCT.md` (Contributor Covenant v2.1).
- README: documented `go install .../cmd/amg@main` (the only install
  path that works before the first tag), and a new section on getting a
  real Appwrite Cloud or self-hosted project/API key to test against.
- `compare.Policy`: severity for "the destination has something the
  source didn't" is now a policy choice, not hard-coded.
  `DefaultPolicy` keeps the original WARN behavior;
  `StrictPolicy` (`--strict` on `compare`/`verify`) raises
  `unexpected_resource` and its row-level counterpart
  `row_sample_unexpected` to BLOCK, for a pre-cutover gate where
  unexpected destination content should stop the run.
  `compare.Result.PolicyName` records which policy produced a result.
- `docs/known-false-negatives.md`: specific differences amg's current
  checks won't catch, in table form.
- `docs/assurance-boundary.md`: what a PASS result does and doesn't
  prove, and a SOURCE/LIVE/TEST/ASSUMPTION classification for every
  "verified" claim in this repository.
- Table schema (column/index) drift detection. `appwrite.ListTables`
  computes a `SchemaDigest` per table — a canonicalized SHA-256 hash of
  its columns and indexes — and `compare` emits `schema_changed`
  (BLOCK) or `schema_unverified` (WARN, digest missing on one side).
  See docs/comparison-model.md#schema-verification.
- `amg` now honors a server's `Retry-After` header on HTTP 429 instead
  of always falling back to its own shorter backoff (delay-seconds
  form only, capped at 2 minutes).
- `manifest.Read` fails closed on a manifest with a newer
  `schema_version` than the running build supports, matching the check
  `amg report` already had for `compare.Result`.
- `amg compare`/`verify`/`report` print a verification-coverage summary
  after every result (PASS included): what's checked, what's checked
  only if enabled, what's never checked.
- README "Product boundary" section: relationship-integrity
  verification, runtime behavior probes, application-level invariants,
  policy/approval workflows, and backup/rollback verification are
  explicitly out of scope, not oversights.
- Sites inventory and comparison (config only — framework, build/
  install/start commands, output directory, timeout; never `vars`).
- Legacy Databases (collections/documents) need no separate collector:
  `POST /v1/tablesdb` and `GET /v1/databases` return byte-for-byte
  identical data for the same resource, and the table/collection layer
  queries the same internal storage in both endpoints' controllers
  (Appwrite marks the legacy API `Deprecated(since: '1.8.0')`).
  `Inventory.Unsupported` is now empty — every resource type from the
  original spec is inventoried.
- Functions inventory and comparison (config only — runtime, schedule,
  timeout, execute permissions, logging, scopes, deployment retention,
  version; never `vars`, which routinely hold secrets).
- Users inventory and comparison: only administrative/verification
  state (enabled/disabled, verification flags, MFA, labels).
  `appwrite.User` has no field for password/hash/email/phone/prefs, so
  `json.Unmarshal` silently drops them from Appwrite's raw response —
  there's nowhere for that data to decode into.
- `amg report <result.json>`: renders a saved `compare`/`verify --json`
  result as text, JSON, or a self-contained HTML file (Go's
  `html/template`, auto-escaping — a resource named `<script>...</script>`
  renders as inert text). `compare.Result` gained `SchemaVersion` so a
  future JSON-shape change won't be silently misread by an older
  `amg report`.
- `.goreleaser.yaml` + `.github/workflows/release.yml`: Linux/macOS/
  Windows binaries (amd64 + arm64, minus Windows/arm64) on every
  `vX.Y.Z` tag, version/commit/date injected via `-ldflags`,
  checksummed, published as a GitHub prerelease.
- `lab/`: fault-injection migration lab, 8 deterministic scenarios
  (missing table, missing file, permission change, expected timestamp
  transformation, transient failure/retry, persistent failure/BLOCK,
  interrupted run/never-OK at both the `inventory.Collect` and
  `amg doctor` layers) run through the real client/pagination/retry/
  collection/comparison stack against `httptest` servers. Runs in CI on
  every push; CI now also fails on unformatted code (`gofmt -l`).
- Opt-in TablesDB row content verification (`--sample-rows N`):
  fingerprints up to N rows (capped at 500, ordered by `$id`) — a
  SHA-256 digest of user-defined column values plus the row's own
  `$permissions` — never storing or transmitting row content itself.
  New rules: `row_content_changed`/`row_permission_changed` (BLOCK),
  `row_sample_missing`/`row_sample_unexpected`/`row_sample_unverified`
  (WARN — sampling is partial, so absence from a sample is never a
  confirmed loss).
- `amg preflight`: pre-migration readiness — connectivity,
  authentication, Appwrite version match, and a `Destination conflict`
  check for resource IDs that already exist on the destination before
  any migration runs.
- Storage inventory: buckets and files. Files carry their
  Appwrite-computed MD5 `signature` as `Resource.ContentDigest`,
  compared via `content_changed` — file content integrity verified
  without amg ever downloading a file.
- `internal/manifest`: deterministic on-disk snapshot format
  (`.amg/runs/<run-id>/manifest.json`), no credential field ever.
- `internal/compare`: offline PASS/WARN/BLOCK comparison engine.
- `amg snapshot`, `amg compare <a> <b>` (fully offline), `amg verify`
  (live, both sides concurrently, same comparison engine as `compare`).
- `amg inventory`: TablesDB inventory (databases, tables, per-table row
  counts) with `--json`, `--no-row-counts`, `--concurrency`.
- `internal/inventory`: canonical `Resource`/`Inventory` model,
  deterministic sorting, bounded-concurrency collector.
- `internal/appwrite`: `ListDatabases`/`ListTables`/`CountRows`, shared
  cursor-pagination with malformed-cursor and cancellation handling.
- CLI skeleton (`cmd/amg`): subcommand dispatch, exit codes
  (0=PASS, 1=WARN, 2=BLOCK). `amg version`, `amg doctor`. Minimal
  Appwrite REST client: auth headers, bounded retries with backoff,
  typed error classification. Environment-variable configuration
  loading, including source/destination pairs and `.env` support.

### Fixed

- `.goreleaser.yaml` used `release.prerelease: auto`, which only marks a
  tag as a GitHub prerelease when the tag itself has a semver
  pre-release suffix (e.g. `v0.1.0-rc1`) — a plain `v0.1.0` tag, the
  natural first release name, would NOT have been marked prerelease,
  contradicting the project's own stated pre-alpha status. Changed to
  `prerelease: true`, which forces it regardless of tag format.
  Re-validated the full release process against the current codebase:
  `goreleaser check` (config valid), `goreleaser build --snapshot`
  (all 5 targets compile), `goreleaser release --snapshot --skip=publish`
  (archives + checksums produced correctly, including README/LICENSE/
  CHANGELOG/.env.example in each archive) — then ran the resulting
  Windows binary directly and confirmed `amg version` reports the
  correct injected version/commit/date.
- `amg verify --json > result.json` followed by `amg report result.json`
  failed on Windows PowerShell: `>` redirection writes UTF-8 with a
  leading byte-order mark, which `encoding/json` treats as invalid.
  Found by running the documented workflow end-to-end on real Windows
  PowerShell against a live project, not synthetically. Fixed by
  tolerating a leading BOM in every file amg reads: `manifest.Read`,
  `report`'s result reader, and `config.LoadDotEnv` (a BOM-prefixed
  `.env`, e.g. from Notepad, silently broke the first variable's key).
- `TestWrite_DeterministicAsideFromRunIDAndTimestamp` was flaky on
  Linux: it built two `inventory.Inventory` values via separate
  `inventory.New()` calls, each stamping its own `GeneratedAt` via
  `time.Now()`, but only normalized `Manifest.RunID`/`CapturedAt`
  before comparing — not `Inventory.GeneratedAt`. Passed locally on
  Windows often enough to miss, failed reliably in CI on Linux.
- `errs.RetryAfterOf` used `RetryAfter > 0` to detect a server-supplied
  delay, which treated a legitimate `Retry-After: 0` the same as no
  header at all. Fixed with an explicit presence flag.
- The coverage note grouped Storage file content (checked
  unconditionally) under "checked if enabled" alongside row sampling
  (genuinely opt-in). Reworded to "Always checked" / "Checked if
  enabled".
- `StrictPolicy` only affected `unexpected_resource`, leaving its
  row-level equivalent `row_sample_unexpected` fixed at WARN. Both now
  share one `Policy.UnexpectedSeverity` field.
- Pagination now tracks every resource ID seen across every page, not
  just each page's last item, so a server that repeats a resource on a
  later page is refused rather than silently accepted. Proven against a
  10,001-resource, ~101-page synthetic dataset
  (`TestListDatabases_LargeDataset_NoPageLossOrDuplication`,
  `TestListDatabases_DuplicateAcrossPages`).
- Removed the README's Go Report Card badge — the service was sunset
  entirely.
- Two "interrupted run" lab tests deadlocked on cleanup: a channel
  meant to unblock a stuck HTTP handler was closed by a `defer` that
  ran after `httptest.Server.Close()`, which itself waits for in-flight
  handlers to return. Fixed by sleeping past the test's deadline
  instead of coordinating through a channel.
- List-valued metadata (a user's `labels`, a bucket's
  `allowed_file_extensions`) was compared with a literal string
  comparison, which is order-sensitive; Appwrite doesn't guarantee list
  ordering. Fixed with an order-independent comparison that also
  handles the `[]any` shape a list takes after a manifest round-trips
  through JSON.
- `CountRows` and row sampling ran as two full sequential passes over
  every table instead of one combined pass.
- A row-sampling failure was misreported in the terminal summary as
  "row counts were not verified" even when counting succeeded.
- Source/destination inventory in `amg preflight` ran serially instead
  of concurrently, and an unreachable side was re-dialed a second time
  for an authentication check it could never pass.
- Added `.gitattributes` forcing LF line endings for Go source — fixed
  `gofmt -l` spuriously flagging every file as unformatted on Windows
  checkouts with no actual content difference.
- `config_changed` comparison is now scoped per resource type
  (`comparedMetadataKeys`) — previously a resource's config keys could
  be checked against another resource type's key list.
- `Client.Version` (`GET /health/version`) no longer sends the API key:
  Appwrite evaluates a request carrying `X-Appwrite-Key` under that
  key's role and rejects `scope: public` endpoints for lacking a
  `"public"` scope no key can hold.
- Context cancellation while reading an HTTP response body was
  misclassified as `KindInvalidResponse` instead of `KindTimeout`.

### Changed

- Reformatted the README's live-testing section from a dense paragraph
  into a table, and trimmed repeated self-rebutting phrasing ("but
  every command is real...") in favor of plain statements with links
  to the evidence.

### Security

- Confirmed no `internal/appwrite` struct has a field named
  password/hash/email/phone/prefs/vars/secret/token, `CountRows`
  discards the row it fetches for counting, and no `%+v`/`%#v`
  struct-dump formatting exists anywhere that could print an `APIKey`
  field.
- `govulncheck` runs in CI on every push (zero direct dependencies, so
  this mainly checks the standard library).
- Rewrote SECURITY.md to match the current secret/PII exclusion
  boundaries per resource type, the HTML report's escaping, and the
  dependency/supply-chain posture.

### Documentation

- Added CI/pkg.go.dev/License badges to the README.
- Compressed the top status blurb, moved verification evidence into
  the Testing section.
- Added a real screenshot of `amg report --format html`'s output to
  the README's Example report section.
- `docs/migration-semantics.md`, `docs/comparison-model.md`: documented
  Appwrite behaviors amg's inventory and comparison depend on,
  including the 5,000-row count cap.

Every live-verification claim in this file (created a real resource,
changed it, confirmed amg's output) was performed against a real
Appwrite Cloud project during development, not simulated — see
[docs/assurance-boundary.md](docs/assurance-boundary.md) for how to
tell a live-verified claim from a source-inference or test-only one
anywhere else in this repository.
