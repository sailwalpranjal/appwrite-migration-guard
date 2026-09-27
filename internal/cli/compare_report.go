package cli

import (
	"fmt"
	"io"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
)

// exitForCompare maps a compare.Result to amg's process exit codes.
func exitForCompare(res *compare.Result) int {
	switch res.Overall() {
	case compare.SeverityPass:
		return ExitOK
	case compare.SeverityWarn:
		return ExitWarn
	default:
		return ExitBlock
	}
}

// writeCompareTerminal renders a compare.Result as human-readable
// terminal output. Every finding always shows its Rule and Message, so a
// user can see exactly why a check failed — never just a bare PASS/BLOCK
// label (spec section 25).
func writeCompareTerminal(w io.Writer, title string, res *compare.Result) {
	fmt.Fprintln(w, title)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Source:      %s\n", res.SourceLabel)
	fmt.Fprintf(w, "Destination: %s\n", res.DestLabel)
	fmt.Fprintf(w, "Policy:      %s\n", res.PolicyName)
	fmt.Fprintln(w)

	if len(res.Findings) == 0 {
		fmt.Fprintf(w, "%-5s no differences found\n", StatusPass)
	}
	for _, f := range res.Findings {
		fmt.Fprintf(w, "%-5s [%s] %s\n", f.Severity, f.Rule, f.Message)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Result: %s\n", res.Overall())
	fmt.Fprintln(w)
	fmt.Fprintln(w, coverageNote)
}

// coverageNote is printed on every compare/verify/report result,
// PASS included — a PASS here means "no difference found within what
// amg checks," not "this migration is proven correct." Making this
// explicit on every run (not just in docs a reader might not open) is a
// direct response to the most serious criticism an external audit of
// this project raised: that a green result could be over-read as a
// stronger guarantee than the implementation actually provides. See
// docs/comparison-model.md for the full rule table this summarizes.
const coverageNote = `Verification coverage (always applies, this run and every run):
  Always checked:  resource existence, permissions, config, table schema
                   (columns/indexes), file content (via MD5 signature)
  Checked if enabled: row content (--sample-rows; off by default)
  Never checked: relationships between resources, function/site code or deployed
                 runtime behavior (config only), application-level invariants
A PASS here means "no difference found in what amg checks" — not a proof
the migration is complete, consistent, or safe to cut over to.`
