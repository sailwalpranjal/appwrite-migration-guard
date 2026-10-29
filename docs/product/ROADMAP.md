# Roadmap

Scores are my own judgement. RICE = Reach x Impact x Confidence / Effort. Reach is on a 1-10 scale, Impact is 0.25 / 0.5 / 1 / 2 / 3, Confidence is a fraction, Effort is person-weeks. ICE = (Impact + Confidence + Ease) / 3, each out of 10.

The README roadmap is a list of what has been done. This one is what I would do next.

## Now

| Item | Why | Score (my judgement) |
|---|---|---|
| Run the acceptance fixture against a self-hosted Appwrite version | Only Appwrite Cloud 2.3.0 has been tested, and many migrations start from self-hosted | RICE: 7 x 2 x 0.6 / 2 = **4.2** |
| Make the sample window configurable beyond "first N by `$id`" (for example, evenly spaced rows) | The biggest listed false negative is a changed row outside the window | RICE: 6 x 2 x 0.7 / 2 = **4.2** |
| Add a `--fail-on block` option so WARN can exit 0 | WARN exits 1 today, which fails most CI steps | ICE: (6 + 9 + 9) / 3 = **8.0** |
| Publish a one-page "migration checklist" that shows where amg fits | The README explains the tool; it does not show the surrounding steps | ICE: (5 + 8 + 9) / 3 = **7.3** |

## Next

| Item | Why | Score (my judgement) |
|---|---|---|
| More expected-transformation rules, added only when a real migration shows them | One rule exists; guessing more would invent behaviour | RICE: 5 x 1 x 0.4 / 2 = **1.0** |
| Relationship reference check (opt-in) | A listed gap; currently out of scope by design | RICE: 4 x 2 x 0.5 / 4 = **1.0** |
| Package managers (Homebrew tap, scoop manifest) | Install friction for non-Go users | RICE: 5 x 1 x 0.8 / 2 = **2.0** |

## Later

| Item | Why | Score (my judgement) |
|---|---|---|
| Function and site code fingerprinting | Config-only comparison misses code changes | ICE: (6 + 4 + 3) / 3 = **4.3** |
| Backup and rollback verification | Different product; listed as a non-goal | ICE: (5 + 2 + 1) / 3 = **2.7** |
