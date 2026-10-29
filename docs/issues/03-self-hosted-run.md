# Run the acceptance fixture against a self-hosted Appwrite

**Labels:** type:chore, priority:P1, area:inventory

## Problem

Everything has been verified against Appwrite Cloud 2.3.0. Many migrations start from a self-hosted install, and API shapes can differ between versions.

## Acceptance criteria

- [ ] `docs/acceptance-test-fixture.md` is followed against a self-hosted Appwrite (record the version).
- [ ] The doctor, inventory, preflight and verify baseline is recorded in the README testing table with the version.
- [ ] Any difference in API behaviour is written to `docs/migration-semantics.md` and, if it breaks a collector, filed as its own issue.
