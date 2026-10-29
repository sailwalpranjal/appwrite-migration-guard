# Add `--fail-on` to control the exit code

**Labels:** type:feature, priority:P0, area:cli

## Problem

Exit codes are fixed by the verdict: 0 for PASS, 1 for WARN, 2 for BLOCK (`internal/cli/cli.go`). Most CI systems fail a step on any non-zero code, so a team that wants WARN to pass and only BLOCK to fail has to wrap the command in shell.

## Acceptance criteria

- [ ] `amg verify` and `amg compare` accept `--fail-on warn|block` (default: the current behaviour).
- [ ] With `--fail-on warn` (default), behaviour is unchanged. With `--fail-on block`, a WARN-only result exits 0 and only BLOCK exits non-zero.
- [ ] The README lists the exit codes in one table.
- [ ] Unit tests cover PASS, WARN and BLOCK under both settings.
