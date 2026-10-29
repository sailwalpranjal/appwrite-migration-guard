# Decision log

Decisions visible in the code and docs. I did not date them at the time.

## 1. A three-state verdict instead of a score

**Context.** People want one number that says whether a migration is safe.
**Options.** A 0-100 confidence score; pass/fail; PASS, WARN or BLOCK with a reason.
**Decision.** PASS, WARN or BLOCK, and every non-PASS finding carries a reason and a finding code.
**Consequences.** The output never implies a precision the tool does not have. Users who want a single number will not get one. `docs/assurance-boundary.md` spells out that PASS means "no difference found in these checks", not "safe".

## 2. Read-only, and secrets never decoded

**Context.** The tool needs an API key with read scope, and Appwrite's responses include password hashes, emails and environment variables.
**Options.** Collect everything and redact on output; collect only what is compared.
**Decision.** The Go structs have no fields for password, hash, or `vars`, so the data has nowhere to land (`internal/appwrite/users.go`, `functions.go`, `sites.go`). A unit test checks that password, hash, email and phone in a raw user response never reach the output.
**Consequences.** Leaks are structurally unlikely. The cost is that user email or function code changes cannot be detected; both are in `docs/known-false-negatives.md`.

## 3. Deterministic manifests, comparison offline

**Context.** I wanted comparison to be repeatable and shareable.
**Options.** Compare two live projects directly; snapshot each to a file and compare files.
**Decision.** `amg snapshot` writes a deterministic manifest (`internal/manifest/manifest.go`); `amg compare` works on two files with no network.
**Consequences.** Comparisons can be rerun, attached to a ticket, and tested without credentials. A snapshot is a point in time, so a PASS is a statement about two moments.

## 4. Row content sampling is opt-in

**Context.** Comparing row content reads real data, and large tables are expensive.
**Options.** Always compare all rows; never compare rows; compare counts by default and a sample on request.
**Decision.** Counts by default; `--sample-rows N` fingerprints the first N rows by `$id`.
**Consequences.** The default run reads no row data. A change outside the window passes, and that is documented as the first known false negative.

## 5. Name the blind spots in the repository

**Context.** A verification tool that overstates its coverage is worse than none.
**Options.** Mention limits in a footnote; document them in dedicated files and print a note on every result.
**Decision.** `docs/assurance-boundary.md`, `docs/known-false-negatives.md`, and a coverage note printed after every result, including PASS (`coverageNote` in `internal/cli/compare_report.go`).
**Consequences.** The documentation reads as less impressive and is more believable. It also creates an obligation to keep the list current.

## 6. A fault-injection lab that runs in CI

**Context.** "It catches migration problems" needs evidence that does not depend on a live account.
**Options.** Manual testing only; mock HTTP; a lab that plants known changes and checks each is caught.
**Decision.** `lab/` plants changes and asserts the expected findings fire; it runs in CI with `go test -race`.
**Consequences.** Regressions in detection fail the build. The lab shows detection of the cases I thought of, not of cases I did not.
