# Known false negatives

This document exists because an external audit specifically asked for
it, and refusing to write it would itself be a product-maturity problem:
a verification tool that won't name its own blind spots is asking to be
trusted more than its evidence supports.

A **false negative** here means: a real, meaningful difference between
source and destination that amg's current checks will not flag — the run
reports `PASS` (or omits a finding) when a careful human, looking at the
same two states, would consider something wrong. This is a different
(and narrower) list than [comparison-model.md#what-this-does-not-do](comparison-model.md#what-this-does-not-do),
which covers entire categories amg makes no attempt at; everything below
is a gap *within* a category amg does check.

| Case | Detected? | Why | Severity if it happened | Mitigation today | Roadmap |
|---|---|---|---|---|---|
| A row outside the sampled window changes | No | `--sample-rows N` only fingerprints the first N rows by `$id` (see comparison-model.md#row-content-sampling-opt-in) | Would be a real, silent data change | Increase `N`; sampling is off by default specifically so this limitation isn't hidden behind an always-on false sense of coverage | Exhaustive (non-sampled) row verification is unscoped — see "What this does not do" |
| A row is deleted and a different row is inserted with a new `$id`, keeping the table's row count unchanged | No — row *count* stays equal, so `row_count_mismatch` does not fire; row *sampling* keys on `$id`, so the "missing" original ID and "new" replacement ID would only be flagged (as WARN, not BLOCK — see `row_sample_missing`/`row_sample_unexpected`) if both happen to fall inside the sampled window | Sampling and counting both key on `$id`/count, neither of which changes here; sampling additionally assumes migration paths preserve row IDs (see comparison-model.md's row-content-sampling section) | A silent record substitution with no count signal | Enable `--sample-rows` with a large enough `N` to cover the affected window; still probabilistic, not guaranteed | None planned; detecting this class in general is a migration-provenance question, out of this tool's scope (see assurance-boundary.md) |
| A column type changes in a way that happens to produce the same canonicalized digest | Effectively impossible for real drift — `schemaDigest` hashes every non-transient field per column/index — but see the caveat below for nested object fields | See internal/appwrite/tablesdb.go's `canonicalizeComponents` | Would be a real schema mismatch | None currently | If Appwrite starts returning nested JSON objects (not flat scalars) inside a column definition with unstable internal key order, canonicalization only sorts the top level; flagged in self-review, not yet observed in practice against 2.3.0's flat column model |
| A relationship column's referenced row is deleted | No | amg has no relationship-integrity check at all — this is a category gap, not a sampling gap; see [README#product-boundary](../README.md#product-boundary) | Could mean orphaned application data | None | Deliberately out of scope — this is application-level data integrity, not Appwrite resource-state verification |
| Two source rows have IDs that collide with two *different* destination rows after ID reassignment by a migration | No | Same ID-preservation assumption as above | Data could be silently swapped between records | None | Same as above |
| A function or site's deployed code changes but its config (runtime, schedule, timeout, etc.) doesn't | No | Function/Site comparison is config-only by design — amg never executes or fingerprints deployed code | Code behavior could differ even though every checked field matches | None | Deliberately out of scope — see README#product-boundary ("runtime behavior probes") |
| A user's email, phone, or prefs changes | No | User inventory deliberately excludes PII/arbitrary application data — see comparison-model.md and migration-semantics.md | Could be a real data-integrity issue for user-facing fields | None | Deliberately out of scope for privacy reasons, not an oversight |
| The *same* row content encoded with different JSON key ordering inside a value that isn't itself a flat map | Detected correctly — `encoding/json` sorts map keys before hashing | N/A, listed here to distinguish from the schema-digest nested-object caveat above | N/A | N/A | N/A |

## What would need to change to close these

The sampling and relationship gaps share a common shape: they require
either (a) exhaustive verification, which trades off against the
explicit non-goal of keeping amg fast and cheap to run against large
projects, or (b) a model of application-level semantics amg has no way
to infer generically. Closing (a) is a config/roadmap question
(`--sample-rows` already exists as the escape hatch, uncapped sampling
could be added if a user asks for it). Closing (b) would require amg to
become a different, larger product — see
[README#product-boundary](../README.md#product-boundary) for why that
tradeoff is deliberate, not accidental.
