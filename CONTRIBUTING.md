# Contributing

Appwrite Migration Guard is a solo-maintained, dependency-light Go
project. Contributions are welcome, especially:

- Verified Appwrite API behavior (with a link to the source/docs you
  checked) for resource types not yet covered.
- Bug reports with a reproduction (ideally against the migration lab
  fixtures once they exist).
- Tests for edge cases in pagination, retry, or normalization logic.

## Ground rules

- **No invented Appwrite behavior.** If you're implementing a check
  against an Appwrite endpoint, link the exact source (server code,
  official docs, or OpenAPI spec) you verified it against, the way
  existing code in `internal/appwrite` does in its package comment.
- **No new dependencies without justification.** Check the standard
  library first. If a dependency is genuinely needed, say why in the PR
  description.
- **Every behavioral change needs a test.** Especially normalization
  rules — an "expected transformation" rule without a test explaining
  *why* it's expected will be rejected.
- **No AI/ML features.** This is a deliberate project boundary, not an
  oversight — see the README's "What amg is not" section.

## Local development

```bash
go build ./...
go vet ./...
go test ./...
```

## Commit / PR style

Small, focused PRs. Explain the *why* in the description, not just the
*what* — the diff already shows the what.
