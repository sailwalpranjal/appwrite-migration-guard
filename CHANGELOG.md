# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added

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
