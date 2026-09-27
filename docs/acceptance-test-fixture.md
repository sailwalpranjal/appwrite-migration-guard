# Acceptance-test fixture: "TaskFlow"

A dedicated Appwrite Cloud project, provisioned once via the real API and
kept in place (not deleted after use, unlike the ad-hoc dev projects
referenced elsewhere in this repo) specifically so amg's behavior can be
re-verified against a realistic, mid-sized project rather than a single
table and a single file. This document is the reproducible walkthrough:
what was created, what was mutated, and what amg was expected — and
observed — to report at each step.

## Shape

One database (`main`, "TaskFlow Main") modeling a small project-
management app:

| Table | Columns | Indexes | Rows |
|---|---|---|---|
| `users_profile` | `name` (string), `email` (string), `role` (string, default `member`), `active` (boolean, default `true`) | `idx_email` on `email` | 3 |
| `projects` | `name` (string), `description` (string), `ownerId` (string) | `idx_owner` on `ownerId` | 2 |
| `tasks` | `title` (string), `status` (string, default `todo`), `projectId` (string), `assigneeId` (string) | `idx_project_status` on `(projectId, status)` | 4 |

Plus: one Storage bucket (`avatars`) with 2 files; 3 real Users (one
later disabled, one later labeled `betatester`) — created through
`POST /v1/users`, so their raw API responses include a live argon2
password hash, matching real user-creation traffic rather than a
synthetic fixture; one Function (`notify-task-complete`, scheduled);
one Site (`taskflow-web`).

`tasks.projectId` and `tasks.assigneeId` model relationships informally
(plain string columns holding another table's row ID) rather than as
Appwrite relationship columns — deliberately, so the fixture also
demonstrates the documented relationship-integrity gap (see
[known-false-negatives.md](known-false-negatives.md)) without depending
on relationship-column API behavior that wasn't otherwise in scope for
this stage.

## What was verified against it

1. **Baseline**: `amg doctor`, `amg inventory`, `amg verify` (source =
   destination = this project) — all PASS, 12 resources inventoried, zero
   PII in the JSON output (grepped for password hashes, emails, phone
   numbers).
2. **Preflight collision detection**: `amg preflight` with
   `AMG_SOURCE_*`/`AMG_DEST_*` both pointed at this project — BLOCKed on
   all 12 resource IDs as destination conflicts, none missed.
3. **Regression batch**: ten independent mutations applied in one pass
   (widen a column, change table permissions, edit a row, delete a
   table, replace a file's content, reschedule a function, clear then
   change a site's build/install commands, insert an extra row, create
   an extra function), snapshotted before and after, `amg compare` —
   exactly the eleven corresponding findings fired (one mutation, the
   file replacement, produces two findings: `content_changed` and
   `name_changed`, since re-uploading under the same file ID with a
   different local filename changes both), nothing else. See the
   Testing section of the main README for the full row-by-row mapping.
4. **Policy behavior**: the same "extra function" scenario compared once
   under the default policy (WARN) and once with `--strict` (BLOCK) —
   confirms severity actually changes with the flag, not just the rule
   name.

## Reproducing this fixture

The provisioning is plain `curl` against documented Appwrite REST
endpoints (`POST /v1/tablesdb`, `.../tables`, `.../columns/string`,
`.../columns/boolean`, `.../indexes`, `.../rows`, `POST
/v1/storage/buckets` + `.../files`, `POST /v1/users`, `POST
/v1/functions`, `POST /v1/sites`) — no amg-specific setup. Column and
index creation are asynchronous on Appwrite's side (`"status":
"processing"`); a short delay (a few seconds) before creating rows or
indexes that depend on them avoids racing that processing step.

## Why this project isn't torn down

Every other live-testing row in this repository's history used a
single-purpose dev project, deleted immediately after the specific
check it existed for. This fixture is different by design: a realistic
multi-resource-type project is expensive to reconstruct by hand for
every future regression pass, and Appwrite Cloud's free tier comfortably
holds a project this size indefinitely, so keeping it removes friction
from re-verifying amg after future changes.
