# Write a one-page migration checklist showing where amg fits

**Labels:** type:docs, priority:P1, area:docs

## Problem

The README explains what amg does. It does not show the steps around it, so a new user cannot tell when to run which command.

## Acceptance criteria

- [ ] `docs/migration-checklist.md` fits on one page and lists: before (`doctor`, `preflight`, `snapshot` of source), during (the Appwrite migration itself, out of scope), after (`snapshot` of destination, `verify`, `report`).
- [ ] Each step names the exact command and the verdict that should make the user stop.
- [ ] It links `docs/assurance-boundary.md` and says plainly that PASS is not a guarantee.
- [ ] The README links to it from "Quick start".
