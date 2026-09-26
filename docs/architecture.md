# Architecture (foundation stage)

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
- `version.go`, `doctor.go` — real implementations.
- `stub.go` — `inventory`/`preflight`/`snapshot`/`verify`/`report`/
  `compare` currently return an explicit "not implemented" BLOCK rather
  than silently doing nothing.

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

Every endpoint path and header this package uses is annotated with the
exact file in `github.com/appwrite/appwrite` (tag `2.3.0`) it was verified
against.

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

No manifest format, no normalization rules, no comparison engine, no
resource-specific inventory (tables, buckets, users, functions), no JSON/
HTML report renderers, no persistent run history. These are staged work —
see the README roadmap.
