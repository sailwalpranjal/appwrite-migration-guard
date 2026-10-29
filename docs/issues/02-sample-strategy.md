# Choose which rows `--sample-rows` fingerprints

**Labels:** type:feature, priority:P0, area:inventory

## Problem

`--sample-rows N` fingerprints the first N rows ordered by `$id`. A change anywhere else is invisible, and `docs/known-false-negatives.md` lists this as the first gap.

## Acceptance criteria

- [ ] A `--sample-strategy first|spread` flag; `spread` picks N rows spaced evenly through the id-ordered table.
- [ ] Default stays `first`, so existing manifests remain comparable.
- [ ] The manifest records the strategy, and `amg compare` warns when the two manifests used different strategies.
- [ ] A test edits a row near the end of a 100-row table and shows `first` misses it and `spread` catches it.
- [ ] `docs/known-false-negatives.md` is updated to describe what remains.
