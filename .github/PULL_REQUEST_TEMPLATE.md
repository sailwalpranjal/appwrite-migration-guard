**What this changes, and why**
(The diff already shows *what*; explain *why* — per CONTRIBUTING.md.)

**Checklist**
- [ ] `gofmt -l .` is empty
- [ ] `go vet ./...` is clean
- [ ] `go test -race ./...` passes
- [ ] Every behavioral change has a test (CONTRIBUTING.md: "an 'expected
      transformation' rule without a test explaining *why* it's expected
      will be rejected" — the same standard applies to any new rule)
- [ ] If this claims new/verified Appwrite API behavior, the source
      (server code, official docs, or a live reproduction) is cited —
      no invented behavior
- [ ] No new dependency, or the dependency's justification is in this
      description (standard library was checked first)
- [ ] Docs updated if this changes what amg checks, doesn't check, or
      claims (docs/comparison-model.md, docs/known-false-negatives.md,
      docs/assurance-boundary.md, README, as applicable)
