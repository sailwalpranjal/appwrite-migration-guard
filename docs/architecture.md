# Architecture

This document describes what is actually implemented today. It will grow
alongside the codebase; it does not describe aspirational design.

## Packages

### `cmd/amg`

Process entrypoint. Parses `os.Args`, resolves a subcommand via
`internal/cli.Lookup`, wires up a cancelable `context.Context` (SIGINT/
SIGTERM), and maps the command's result to a process exit code.

### `internal/cli`

- `cli.go` — the command registry (`Commands`) and `Usage()`.
- `report.go` — `Checklist`/`Check`/`Status`, the PASS/WARN/BLOCK model.
  `Checklist.Overall()` is BLOCK for an empty checklist by design: an
  incomplete run must never render as success.
- `version.go`, `doctor.go`, `inventory.go`, `snapshot.go`, `compare.go`,
  `verify.go`, `preflight.go`, `report_cmd.go` — real implementations.
  `doctor.go` also holds `explainAuthError`, a shared helper both it and
  `preflight.go` use to tell "API key rejected" apart from "API key
  missing a scope" instead of a bare error string.
- `compare_report.go` — shared terminal renderer for `compare.Result`,
  used by `compare`, `verify`, and `report --format text` so their
  output stays identical.
- `report_cmd.go` — `amg report`: reads a `compare.Result` JSON file (as
  written by `compare --json`/`verify --json`) and re-renders it as
  text, JSON, or HTML (via `internal/report`). Rejects a result whose
  `SchemaVersion` is newer than this build supports, rather than
  guessing at an unknown shape.

There is no more `stub.go` — every registered command is a real
implementation as of this stage.

### `internal/appwrite`

A narrow REST client, not a general SDK:

- `client.go` — request/retry/error-classification. Retries are attempted
  only for `KindConnectivity`, `KindRateLimit`, `KindTimeout`, and
  `KindServer`; authentication/authorization/validation errors are never
  retried (spec: don't retry things that won't succeed on retry).
- `query.go` — builds Appwrite's `queries[]` filter/pagination strings.
  The wire format is verified against `utopia-php/database` (the query
  engine Appwrite itself depends on), not guessed — see the package
  comment for the exact source checked.
- `pagination.go` — generic cursor-pagination walker shared by every list
  endpoint. Detects a non-advancing cursor (malformed/buggy pagination)
  and aborts with `KindPagination` instead of looping forever; checks
  `ctx.Done()` between pages so cancellation stops promptly.
- `tablesdb.go` — TablesDB (databases/tables/rows) methods: `ListDatabases`,
  `ListTables`, `CountRows`. `CountRows` fetches a single row
  (`Limit(1)`) and reads the server-computed `total`, never row content —
  see docs/migration-semantics.md for the 5,000-row cap this is subject
  to.
- `rowsample_test.go` (test) / `SampleRows` in `tablesdb.go` — opt-in row
  content fingerprinting: fetches up to `maxSampleRows` (500) rows
  ordered by `$id`, strips every `$`-prefixed (Appwrite-managed) field,
  and SHA-256 hashes what's left. Row content is discarded immediately
  after hashing — only `RowSample{ID, Permissions, Digest}` is ever
  returned or persisted.
- `storage.go` — Storage (buckets/files) methods: `ListBuckets`,
  `ListFiles`. `File.Signature` is Appwrite's own server-computed MD5 of
  file content, letting amg verify file integrity by comparing a string
  instead of ever downloading file bytes — see docs/migration-semantics.md.
- `users.go` — `ListUsers`. The `User` struct declares only
  administrative/verification fields (`Name`, `Status`, `Labels`,
  verification/MFA flags); it has no field for `password`/`hash`/
  `hashOptions`/`email`/`phone`/`prefs`, so `json.Unmarshal` silently
  drops them even if Appwrite's raw response includes them — there is
  nothing for that data to decode into. See docs/migration-semantics.md.
- `functions.go` — `ListFunctions`. Same pattern: `Function` has no field
  for `vars` (function environment variables, which routinely hold
  secrets), so they are never decoded regardless of what the raw
  response contains.

Every endpoint path and header this package uses is annotated with the
exact file in `github.com/appwrite/appwrite` (tag `2.3.0`) it was verified
against. `Client.Version` deliberately never sends the API key — a real
bug found by testing against a live Appwrite Cloud project, documented on
`requestAuth` and in docs/migration-semantics.md.

### `internal/inventory`

- `inventory.go` — the canonical `Resource`/`Inventory` model (spec
  section 14), independent of Appwrite's JSON shapes. `Inventory.Sort()`
  orders resources by `(Type, ParentID, ID)` so two runs against an
  unchanged project produce identical output regardless of API response
  ordering.
- `collect.go` — `Collect()` lists databases, then (bounded-concurrency,
  fail-fast) tables per database, then storage buckets/files the same
  way, then (bounded-concurrency, best-effort, one pass) a row count
  and/or row-content sample per table depending on `Options`. A table
  whose row count or sampling fails is still included in the inventory
  with `CountError`/`SampleError` set, rather than being dropped or
  aborting the whole run.
- `concurrency.go` — two small worker-pool helpers (`runFailFast`,
  `runBestEffort`), not a generic executor framework — see the spec's
  "avoid premature abstraction" guidance.

### `internal/manifest`

Wraps an `inventory.Inventory` with `RunID`/`Label`/`CapturedAt` and
writes/reads it as JSON under `.amg/runs/<run-id>/manifest.json` (or a
caller-chosen path). `Manifest` has no field that could ever hold a
credential, by construction — there is nothing to redact because there is
nothing to leak.

### `internal/compare`

`Compare(sourceLabel, source, destLabel, dest) *Result` — the engine
behind both `amg compare` (reads two manifest files, no network) and
`amg verify` (inventories both sides live, then calls the exact same
function). See docs/comparison-model.md for the full rule table. Resource
matching is by `(Type, ID)`, not list position or parent, so a resource
that moved to a different parent is still recognized as the same resource
(and reported via `parent_changed`) rather than showing up as an
unrelated missing+unexpected pair. `compareRowSamples` applies the same
kind of matching one level down, by row ID within a table's sampled set —
see its doc comment for why missing/unexpected sampled rows are WARN
(sampling is inherently partial) while a digest or permission mismatch
*within* the sample is BLOCK (that's a confirmed difference).
`Result.SchemaVersion` (`ResultSchemaVersion` constant) supports future
JSON-shape changes without breaking older saved results — see
`internal/report`/`report_cmd.go`.

### `internal/report`

`WriteHTML(w, *compare.Result)` renders a single self-contained HTML
document via Go's `html/template` (auto-escaping every dynamic value —
never `text/template`, and nothing here ever wraps Finding data in
`template.HTML`). No external stylesheet, script, font, or image
reference: CSS is inlined in a `<style>` block, and both a light and dark
palette are defined so the page respects the viewer's OS preference. The
findings table uses scoped `<th>` headers for screen readers, and
severity is always shown as literal text (`PASS`/`WARN`/`BLOCK`), color
only ever a secondary cue. See `html_test.go` for the XSS-escaping and
no-external-resource regression tests this design is checked against.

### `internal/config`

Loads `Environment{Endpoint, ProjectID, APIKey}` from environment
variables, with an optional `.env` file that never overrides a real
environment variable (`LoadDotEnv`). Two loaders exist: `Target()` for
single-environment commands, and `Source()`/`Destination()` for commands
that compare two environments. This split is amg's own interface design
(not an Appwrite concept) — see `.env.example`.

### `internal/errs`

A closed set of `Kind` values (`connectivity`, `authentication`,
`authorization`, `rate_limit`, `not_found`, `validation`, `pagination`,
`timeout`, `server`, `invalid_response`, `comparison`, `configuration`,
`unsupported`) wrapping an underlying error. Callers branch on `Kind` via
`errs.IsKind`/`errs.KindOf`, never on error message text.

### `internal/version`

Build-time metadata (`Version`/`Commit`/`Date`), set via `-ldflags` at
release build time; defaults to `"dev"` for local builds.

### `lab`

Not `internal/` — deliberately a top-level, independently discoverable
package. Eight deterministic fault-injection scenarios (spec section 33,
`lab_test.go`) exercised through the real `appwrite`/`inventory`/
`compare`/`cli` stack against `httptest` servers, not mocked internals.
`WithBackoff` was added to `appwrite.Client`'s options
(`internal/appwrite/client.go`) specifically so this external package
could keep retry-scenario tests fast without needing package-private
access to the `backoff` field the way internal tests already have. Runs
in CI on every push.

## Release automation

`.goreleaser.yaml` + `.github/workflows/release.yml`: on a `vX.Y.Z` tag
push, [GoReleaser](https://goreleaser.com) cross-compiles `amg` for
linux/darwin/windows × amd64/arm64 (Windows/arm64 excluded), injects
`internal/version.{Version,Commit,Date}` via `-ldflags`, archives each
binary with `README.md`/`LICENSE`/`CHANGELOG.md`/`.env.example`,
computes checksums, and publishes a GitHub Release marked `prerelease`
(amg is pre-alpha). Validated locally end-to-end with `goreleaser build
--snapshot` and `goreleaser release --snapshot --skip=publish`: all 5
targets build, archive correctly, and the resulting binary runs and
reports the correct injected version — not just config-checked, actually
executed.

## What is deliberately not here yet

No inventory (and therefore no comparison) of legacy Databases
(collections/documents) or Sites, no exhaustive (non-sampled) row
content comparison, no persistent run history beyond the manifest/result
files a user explicitly saves. These
are staged work — see the README roadmap.
