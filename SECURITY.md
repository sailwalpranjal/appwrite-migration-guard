# Security Policy

Appwrite Migration Guard (amg) is a local CLI that reads from Appwrite
projects using credentials you supply. It does not run as a hosted
service and does not transmit your data anywhere except to the Appwrite
endpoints you configure.

## Reporting a vulnerability

Please open a private security advisory on GitHub
(`Security` tab → `Report a vulnerability`) rather than a public issue.
Include:

- amg version (`amg version`)
- Appwrite version/target (Cloud or self-hosted version)
- Steps to reproduce
- Impact you believe it has

## Handling of credentials

- API keys are read only from environment variables or a local `.env`
  file that you control.
- Keys are sent only as the `X-Appwrite-Key` HTTP header on requests you
  initiate; they are never written to manifests, JSON/HTML reports, or
  log output. `GET /health/version` (the reachability check `doctor`/
  `preflight` use) is called with **no** key at all — see
  `internal/appwrite/client.go`'s `requestAuth` doc comment for why
  sending a key there is actively wrong, not just unnecessary.
- amg does not phone home, does not send telemetry, and does not require
  any account or hosted backend of its own.

## Data amg deliberately never collects

amg inventories Appwrite resources for structural/administrative
comparison, not full content replication. Several fields are excluded
by construction — the Go struct in `internal/appwrite` simply has no
field for them, so `encoding/json` silently drops them on decode even
when Appwrite's raw API response includes them:

- **Users** (`internal/appwrite/users.go`): no `password`, `hash`,
  `hashOptions`, `email`, `phone`, or `prefs`. Only administrative state
  (enabled/disabled, verification flags, MFA, labels) is collected.
- **Functions** (`internal/appwrite/functions.go`): no `vars` (function
  environment variables, which routinely hold API keys/DB credentials/
  tokens). Only configuration (runtime, schedule, timeout, execute
  permissions, ...) is collected.
- **TablesDB rows**: row content is never collected by default.
  `--sample-rows N` (opt-in) fingerprints up to N rows with a SHA-256
  digest of their column values — the content itself is hashed and
  discarded in the same function call, never stored, logged, or
  returned (see `appwrite.RowSample`, `rowSampleFrom`).
- **Storage files**: file content is never downloaded. Content integrity
  is verified via Appwrite's own server-computed MD5 `signature` field.

See `docs/migration-semantics.md` for the full reasoning per resource
type. Each of these boundaries has a dedicated regression test asserting
the excluded data cannot appear in a decoded value even when present in
the raw API response (e.g. `TestListUsers_NeverDecodesSensitiveFields`,
`TestListFunctions_NeverDecodesVars`).

## Report rendering

`amg report --format html` renders findings via Go's `html/template`
(auto-escaping), never `text/template` and never `template.HTML` around
data amg didn't author itself — a resource literally named
`<script>...</script>` renders as inert text. No external stylesheet,
script, font, or image is referenced, so a generated report works fully
offline. See `internal/report/html_test.go` for the escaping regression
tests.

## Dependencies and supply chain

amg has zero direct external Go dependencies as of this writing — the
entire tool is built on the standard library. `go run
golang.org/x/vuln/cmd/govulncheck` runs in CI on every push
(`.github/workflows/ci.yml`), checking both any future dependencies and
the standard library itself against the official Go vulnerability
database.

## Scope

In scope: amg's own code (`cmd/`, `internal/`, `lab/`). Out of scope:
Appwrite itself — please report Appwrite server/SDK vulnerabilities to
the [Appwrite project](https://github.com/appwrite/appwrite/security).
