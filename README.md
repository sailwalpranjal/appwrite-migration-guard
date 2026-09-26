# Appwrite Migration Guard (amg)

**Verify Appwrite changes before they become incidents.**

> **Status: early stage, real inventory working.** CLI skeleton,
> configuration, the Appwrite REST client, `version`, `doctor`, and
> `inventory` (TablesDB: databases, tables, row counts) are implemented,
> tested, and verified against a live Appwrite Cloud project. `preflight`,
> `snapshot`, `verify`, `report`, and `compare` are registered commands
> that currently exit with an explicit "not yet implemented" error — see
> [Roadmap](#roadmap). This README describes what exists today, not the
> finished product.

## The problem

Appwrite already provides migration tooling, and its own documentation
recommends validating permissions and data integrity after a migration or
self-hosted upgrade — because migrations can produce partial transfers,
permission drift, or silently-skipped resources. Most teams currently
validate this by hand, or not at all, and only discover a problem once an
application starts failing in production.

## What amg is

An independent, local-first CLI that:

- inventories resources in an Appwrite project (source and/or destination),
- builds a deterministic manifest of that inventory,
- applies documented compatibility/normalization rules to tell real
  differences apart from expected migration transformations,
- compares source, expected, and destination state,
- reports the result as **PASS / WARN / BLOCK** — never a fabricated
  numeric "safety score" — with an explicit reason for every failure.

## What amg is not

- Not a migration engine. It never moves or writes data in your Appwrite
  projects — it only reads, for inventory and verification.
- Not an Appwrite clone, BaaS, or admin-console replacement.
- Not a hosted SaaS, dashboard, or multi-tenant service.
- Not AI/ML-based. There is no model, no LLM, no generated analysis —
  every check is an explicit, documented rule.

## Supported Appwrite environments

- **Appwrite Cloud**, when `APPWRITE_ENDPOINT`/`AMG_*_ENDPOINT` points at
  `https://cloud.appwrite.io/v1` (or your region's endpoint) with a valid
  project ID and API key.
- **Self-hosted Appwrite**, when pointed at your own endpoint.

REST endpoints and request/response shapes used by amg are verified
against the [appwrite/appwrite](https://github.com/appwrite/appwrite)
server source at tag `2.3.0` (see comments in `internal/appwrite`), not
guessed. If you run a materially older self-hosted version and hit a
mismatch, please open an issue.

## Installation

Requires [Go 1.21+](https://go.dev/dl/).

```bash
git clone https://github.com/sailwalpranjal/appwrite-migration-guard
cd appwrite-migration-guard
go build -o amg ./cmd/amg
```

## Quick start

```bash
cp .env.example .env
# edit .env with your Appwrite endpoint/project/API key

./amg doctor
```

`doctor` checks, in order: that required environment variables are set,
that the endpoint is reachable (`GET /health/version`, unauthenticated),
and that the configured API key is valid and has the `health.read` scope
(`GET /health`). It never mutates anything.

```bash
./amg inventory
```

`inventory` lists every TablesDB database and table in the configured
project, plus a per-table row count (a single cheap `total` lookup, never
row content). Add `--json` for machine-readable output, `--no-row-counts`
to skip counts entirely, or `--concurrency N` to change how many Appwrite
requests run at once (default 4). It never writes to your project.

## Configuration

amg reads configuration from environment variables (and an optional local
`.env` file, which never overrides a real environment variable). See
[.env.example](.env.example) for the full list:

- `APPWRITE_ENDPOINT` / `APPWRITE_PROJECT_ID` / `APPWRITE_API_KEY` — a
  single target environment, used by commands that operate on one project
  (`inventory`, `snapshot`, `doctor`).
- `AMG_SOURCE_*` / `AMG_DEST_*` — a source/destination pair, used by
  commands that compare two environments (`preflight`, `verify`).

API keys are never logged, never written to manifests or reports, and are
only ever sent as the `X-Appwrite-Key` HTTP header.

## Example workflow (target shape — not all steps exist yet)

```text
amg doctor                # confirm connectivity + auth on both sides
amg inventory              # inventory the source project
amg preflight               # check source/destination compatibility
# ... you run the Appwrite migration or upgrade yourself ...
amg snapshot                # inventory the destination project
amg verify                  # compare source, expected, and destination
amg report --format html    # render the result
```

## Architecture

```text
cmd/amg/            CLI entrypoint (argument parsing, dispatch)
internal/cli/       Subcommand implementations, PASS/WARN/BLOCK reporting
internal/appwrite/   Minimal Appwrite REST client (auth, retries, pagination, TablesDB)
internal/inventory/  Canonical resource model + bounded-concurrency collector
internal/config/     Environment-variable configuration loading
internal/errs/       Typed error taxonomy (connectivity/auth/rate-limit/...)
internal/version/    Build-time version metadata
```

No database, no message broker, no background services. Comparison is
designed to run from local JSON manifests so it works fully offline (see
[docs/comparison-model.md](docs/comparison-model.md), to be written
alongside the comparison engine).

## Comparison semantics

Every check resolves to exactly one of:

- **PASS** — verified and matches expectations (including documented,
  expected migration transformations).
- **WARN** — verified only partially (e.g. metadata-only file check), or a
  resource type is not yet supported by amg.
- **BLOCK** — a real, unexplained difference, or amg could not complete
  verification at all. An incomplete run is always BLOCK, never a silent
  pass.

## Security

- No secrets are ever written to logs, manifests, JSON reports, or HTML
  reports.
- API keys are sent only as request headers, never as query parameters or
  in request/response bodies that get persisted.
- See [SECURITY.md](SECURITY.md) for the reporting process.

## Testing

```bash
go build ./...
go vet ./...
go test ./...
```

All claims of "supported" or "tested" in this repository are backed by the
tests in the corresponding package — see `*_test.go` files next to the
code they test. `inventory` has additionally been run against a live
Appwrite Cloud project (create database → create table → `amg inventory`
→ delete both), not just mocked HTTP servers.

## Migration lab

Not yet built. The plan is a set of deterministic fixtures (missing
document, missing file, changed permission, expected timestamp
transformation, transient/persistent API failure, interrupted run) used to
prove amg's comparison engine actually detects real problems — see
[Roadmap](#roadmap).

## Limitations

- Pre-alpha: `version`, `doctor`, and `inventory` do real work; everything
  else is a stub.
- Inventory covers TablesDB (databases/tables/row counts) only. Legacy
  Databases (collections/documents), Storage, Users, Functions, and Sites
  are explicitly marked `UNSUPPORTED`, not silently skipped — see
  [docs/migration-semantics.md](docs/migration-semantics.md).
- Row counts above 5,000 are capped by Appwrite itself and reported as
  such (`RowCountCapped`), not silently rounded or hidden.
- No manifest snapshot format, no normalization, no comparison engine yet.
- Not tested against every self-hosted Appwrite version — verified so far
  against Appwrite Cloud running server version 2.3.0.
- Windows/macOS/Linux binaries are not yet published; build from source.

## Roadmap

1. ~~Inventory: TablesDB (tables/rows)~~ — done. Next: legacy Databases
   (collections/documents), Storage buckets/files, Users, Functions.
2. Deterministic manifest format + `amg snapshot` (persist an Inventory
   run to `.amg/runs/<id>/manifest.json`).
3. Normalization + comparison engine + `amg compare` (fully offline).
4. `amg preflight` / `amg verify` wired to the comparison engine.
5. Terminal, JSON, and static HTML reporting for comparisons.
6. Fault-injection migration lab + CI.
7. Cross-platform release binaries.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Issues and PRs welcome; please
open an issue before large changes.

## License

[MIT](LICENSE)
