---
name: Bug report
about: Something amg does is wrong, or a documented claim doesn't hold up
title: ""
labels: bug
---

**What happened**

**What you expected instead**

**Reproduction**
The most useful report is a new scenario for `lab/` (the fault-injection
migration lab) reproducing this deterministically — see CONTRIBUTING.md.
If that's not practical, the next best thing is:

- The exact command you ran (redact `APPWRITE_API_KEY`/`APPWRITE_PROJECT_ID`
  if you're pasting real output — amg itself never logs them, but your
  terminal history might have them nearby).
- `amg version` output.
- Appwrite server version (self-hosted) or "Cloud" (and which region, if
  relevant).
- The relevant `amg` output (terminal, or `--json`, whichever shows the
  problem).

**Is this a false negative or a false positive?**
(i.e. did amg miss something real, or flag something that wasn't
actually a problem?) If it's a false negative not already listed in
[docs/known-false-negatives.md](../../docs/known-false-negatives.md),
that's especially useful to know.
