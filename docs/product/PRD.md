# PRD: Appwrite Migration Guard (amg)

**Status:** Retrospective PRD, written after the build. It records the product decisions the code already reflects. It is not a requirements document that existed before the first commit.

## Problem

Appwrite has its own migration tooling and recommends checking permissions and data integrity afterwards, because migrations can produce partial transfers, permission drift and silently skipped resources. The common check is by hand, or none. A problem is found when an application starts failing.

## Target user

Primary user: a developer migrating an Appwrite project (Cloud to Cloud, or self-hosted to Cloud, or across a self-hosted upgrade) who wants a second opinion on whether the destination matches the source. I have not interviewed anyone doing this, and the project has no recorded users. It is self-declared pre-alpha.

## Jobs to be done

- Before a migration, I want to know whether the destination already contains resource IDs that will collide, so I can fix it before I start.
- After a migration, I want to know which resources are missing, changed or unexpected, so I can fix them before users notice.
- When I report the result to a teammate, I want a file they can open without installing anything, so they can see the same findings.

## Goals

1. Return one of three verdicts, PASS, WARN or BLOCK, with a reason for every finding.
2. Be read-only against Appwrite.
3. Never collect secrets or personal data: no passwords, emails, phone numbers, or function and site environment variables.
4. State the limits of a PASS and the known false negatives in the documentation, and print the coverage note with every result.
5. Work fully offline for comparison, from saved manifests.

## Non-goals

- Performing or repairing a migration.
- A numeric safety score. It would imply precision the tool does not have.
- Relationship-integrity checks, runtime probes of functions and sites, application-level invariants, backup and rollback verification (see "Product boundary" in the README).
- A hosted service.

## User stories and acceptance criteria

| Story | Acceptance criteria |
|---|---|
| As a developer I can take a deterministic snapshot of a project. | `amg snapshot` writes a manifest; two snapshots of an unchanged project compare as PASS |
| As a developer I can compare two manifests offline. | `amg compare a.json b.json` needs no network and no credentials, and names each difference with a finding code (for example `schema_changed`, `missing_resource`) |
| As a developer I can check compatibility before migrating. | `amg preflight` returns BLOCK for an ID that already exists on the destination, and a specific reason for bad credentials |
| As a developer I can trust the output not to leak secrets. | A unit test (`internal/appwrite/users_test.go`) feeds a raw response containing a password, hash, email and phone and asserts none of them reach the output |
| As a reviewer I can read the result in a browser. | `amg report --format html` produces a self-contained file with no external resource references |

## Success metrics

Nobody outside the project is known to use it, so there is no adoption metric. What is measured:

- North star (proposed): migrations where amg ran before and after, and the number of real problems it found that the person had not already found. Not measured.
- Input metrics that exist: 225 top-level Go tests passing (`go test ./...`); 8 commands; 14 live verification passes against Appwrite Cloud recorded in the README.
- Guardrails: the secret-leak test in `users_test.go` stays green; the `--strict` policy makes unexpected resources BLOCK.

See [METRICS.md](METRICS.md).

## Risks and open questions

- **False negatives.** A changed row outside the sampled window, or changed function code with unchanged config, passes. Both are listed in `docs/known-false-negatives.md`.
- **Appwrite API drift.** It has been verified against one server version (2.3.0 on Appwrite Cloud). Self-hosted versions are untested.
- **Unknown expected transformations.** Only one normalisation rule exists (`$createdAt` and `$updatedAt` are ignored). Real migrations may rewrite more.
- **No users yet.** Open question: does a "second opinion" tool get used, or does it only get run after something has already broken?
