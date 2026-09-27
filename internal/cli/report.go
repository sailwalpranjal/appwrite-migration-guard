package cli

import (
	"fmt"
	"io"
)

// Status is one of the three verification states used throughout amg.
// There is deliberately no numeric "score" — see docs/comparison-model.md.
type Status string

const (
	StatusPass  Status = "PASS"
	StatusWarn  Status = "WARN"
	StatusBlock Status = "BLOCK"
)

// Check is a single named result line in a terminal report.
type Check struct {
	Status Status `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"` // optional, shown on failure/warning for explainability
}

// ChecklistSchemaVersion is bumped whenever ChecklistResult's JSON shape
// changes in a way that could break a consumer (e.g. `amg report`)
// written against an older version — same versioning discipline as
// compare.ResultSchemaVersion, for the same reason (spec section 26).
const ChecklistSchemaVersion = 1

// ChecklistResult is the --json-serializable form of a Checklist run
// (`amg doctor`/`amg preflight`), analogous to compare.Result for
// `compare`/`verify` — a saved file `amg report` can read back and
// render as text/json/html, the same way it already does for a
// comparison result.
type ChecklistResult struct {
	SchemaVersion int     `json:"schema_version"`
	Command       string  `json:"command"` // "doctor" or "preflight"
	Checks        []Check `json:"checks"`
	Overall       Status  `json:"overall"`
}

// ToResult converts c into its --json-serializable form.
func (c *Checklist) ToResult(command string) ChecklistResult {
	return ChecklistResult{
		SchemaVersion: ChecklistSchemaVersion,
		Command:       command,
		Checks:        c.Checks,
		Overall:       c.Overall(),
	}
}

// Checklist accumulates Checks and derives the overall Status.
type Checklist struct {
	Checks []Check
}

func (c *Checklist) Add(status Status, title string, detail string) {
	c.Checks = append(c.Checks, Check{Status: status, Title: title, Detail: detail})
}

func (c *Checklist) Pass(title string)          { c.Add(StatusPass, title, "") }
func (c *Checklist) Warn(title, detail string)  { c.Add(StatusWarn, title, detail) }
func (c *Checklist) Block(title, detail string) { c.Add(StatusBlock, title, detail) }

// Overall returns the worst status across all checks: BLOCK if any check
// blocked, else WARN if any warned, else PASS. An empty checklist is BLOCK
// — an incomplete run must never be reported as success (spec section 33,
// scenario G).
func (c *Checklist) Overall() Status {
	return overallOfChecks(c.Checks)
}

// overallOfChecks is the single derivation Checklist.Overall() and every
// consumer of a saved ChecklistResult must go through — never trusting a
// deserialized "overall" field verbatim, the same way compare.Result's
// Overall() is always derived from Findings rather than stored/read as
// data. A hand-edited, corrupted-in-transit, or future/buggy amg's
// ChecklistResult JSON could otherwise carry a "checks" array containing
// a BLOCK entry alongside an "overall":"PASS" field that doesn't match
// it, and `amg report` trusting that field would exit 0 while printing a
// BLOCK line — silently green-lighting a CI pipeline gated on the exit
// code.
func overallOfChecks(checks []Check) Status {
	if len(checks) == 0 {
		return StatusBlock
	}
	worst := StatusPass
	for _, chk := range checks {
		switch chk.Status {
		case StatusBlock:
			return StatusBlock
		case StatusWarn:
			worst = StatusWarn
		}
	}
	return worst
}

// ExitCode maps Overall() to the process exit code documented in cli.go.
func (c *Checklist) ExitCode() int {
	switch c.Overall() {
	case StatusPass:
		return ExitOK
	case StatusWarn:
		return ExitWarn
	default:
		return ExitBlock
	}
}

// WriteTerminal renders the checklist as human-readable terminal output.
// Status labels are always printed as text (PASS/WARN/BLOCK), never
// conveyed by color alone, per the accessibility requirement.
func (c *Checklist) WriteTerminal(w io.Writer, title string) {
	writeChecklistTerminal(w, title, c.Checks)
}

// WriteTerminal renders a previously-saved ChecklistResult (loaded by
// `amg report`) identically to how the live Checklist.WriteTerminal
// would have rendered it at the time it was captured. The stored
// "overall" field is never read here — see overallOfChecks.
func (r ChecklistResult) WriteTerminal(w io.Writer, title string) {
	writeChecklistTerminal(w, title, r.Checks)
}

// writeChecklistTerminal is the one place terminal checklist formatting
// is implemented, shared by both the live (Checklist) and saved-and-
// reloaded (ChecklistResult) rendering paths so they can never drift
// apart from each other.
func writeChecklistTerminal(w io.Writer, title string, checks []Check) {
	fmt.Fprintln(w, title)
	fmt.Fprintln(w)
	for _, chk := range checks {
		fmt.Fprintf(w, "%-5s %s\n", chk.Status, chk.Title)
		if chk.Detail != "" {
			fmt.Fprintf(w, "      %s\n", chk.Detail)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Result: %s\n", overallOfChecks(checks))
}
