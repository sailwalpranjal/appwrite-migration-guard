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
	Status Status
	Title  string
	Detail string // optional, shown on failure/warning for explainability
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
	if len(c.Checks) == 0 {
		return StatusBlock
	}
	worst := StatusPass
	for _, chk := range c.Checks {
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
	fmt.Fprintln(w, title)
	fmt.Fprintln(w)
	for _, chk := range c.Checks {
		fmt.Fprintf(w, "%-5s %s\n", chk.Status, chk.Title)
		if chk.Detail != "" {
			fmt.Fprintf(w, "      %s\n", chk.Detail)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Result: %s\n", c.Overall())
}
