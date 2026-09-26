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
	fmt.Fprintln(w)

	if len(res.Findings) == 0 {
		fmt.Fprintf(w, "%-5s no differences found\n", StatusPass)
	}
	for _, f := range res.Findings {
		fmt.Fprintf(w, "%-5s [%s] %s\n", f.Severity, f.Rule, f.Message)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Result: %s\n", res.Overall())
}
