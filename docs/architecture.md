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
- `version.go`, `doctor.go`, `inventory.go` — real implementations.
- `stub.go` — `preflight`/`snapshot`/`verify`/`report`/`compare` currently
  return an explicit "not implemented" BLOCK rather than silently doing
  nothing.

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
  fail-fast) tables per database, then (bounded-concurrency, best-effort)
  a row count per table. A table whose row count fails is still included
  in the inventory with `CountError` set, rather than being dropped or
  aborting the whole run.
- `concurrency.go` — two small worker-pool helpers (`runFailFast`,
  `runBestEffort`), not a generic executor framework — see the spec's
  "avoid premature abstraction" guidance.

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

## What is deliberately not here yet

No manifest/snapshot format, no normalization rules, no comparison
engine, no inventory of legacy Databases/Storage/Users/Functions/Sites,
no JSON/HTML report renderers for comparisons, no persistent run history.
These are staged work — see the README roadmap.
