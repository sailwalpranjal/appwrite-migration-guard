# Metrics

amg is a local CLI with no telemetry, and I want to keep it that way. This is a plan for how I would learn whether it helps, using opt-in signals and conversation rather than silent tracking.

## Questions

1. Does anyone run amg before a migration, or only after something broke?
2. When it reports WARN or BLOCK, was the finding real?
3. Which findings are noise?

## Signals, all opt-in

| Signal | How it would be collected | Question |
|---|---|---|
| Downloads per release | GitHub release asset download counts (already available) | 1, rough |
| Stars and issues opened | GitHub | 1, rough |
| `finding_code` and verdict of a run | A `--share-summary` flag that prints a small JSON blob (verdict, finding codes and counts, amg version, Appwrite server version, no IDs or names) that the user may paste into an issue | 2, 3 |
| "Was this finding useful?" | A line in the HTML report linking to an issue template with the finding code pre-filled | 2, 3 |

Nothing is sent automatically.

## Queries

Counting finding codes across pasted summaries (once there are any), with the summaries collected in a CSV:

```sql
SELECT finding_code,
       COUNT(*) AS reports,
       AVG(was_real) AS real_share
FROM summaries
GROUP BY finding_code
ORDER BY reports DESC;
```

Release downloads, from the GitHub API:

```
gh api repos/sailwalpranjal/appwrite-migration-guard/releases --jq '.[] | {tag: .tag_name, downloads: ([.assets[].download_count] | add)}'
```

## Guardrails that exist today

- `go test ./...` runs 225 top-level tests and the fault-injection lab; CI also runs `gofmt`, `go vet` and `go test -race`.
- `internal/appwrite/users_test.go` checks that password, hash, email and phone in a raw user response never reach the output. The README's live runs also grepped real manifests for them.
