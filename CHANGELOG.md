# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added

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
