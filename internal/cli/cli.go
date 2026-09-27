// Package cli implements amg's subcommands. Each command is a small
// function of (args []string, stdout, stderr io.Writer) -> exit code, kept
// free of global state so it can be tested directly.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

// ExitCode mirrors the process exit codes amg promises in its docs.
// PASS/no findings = 0. WARN-only = 1. BLOCK or unhandled error = 2.
const (
	ExitOK    = 0
	ExitWarn  = 1
	ExitBlock = 2
)

// Command is one amg subcommand.
type Command struct {
	Name    string
	Summary string
	Run     func(ctx context.Context, args []string, stdout, stderr io.Writer) int
}

// Commands is the registry of all top-level amg subcommands, in help
// display order. This order deliberately matches the README's Quick
// Start / Example workflow walkthrough (doctor -> inventory -> snapshot
// -> compare -> verify -> preflight -> report), not alphabetical or
// registration order — a PM-level release-readiness review flagged the
// previous mismatch as an avoidable "did I miss a step" moment for
// someone running `amg --help` right after cloning the repo and then
// following the README tutorial. version is listed last since it's not
// part of the migration-verification workflow itself.
var Commands = []Command{
	{Name: "doctor", Summary: "Check that amg itself is configured correctly", Run: RunDoctor},
	{Name: "inventory", Summary: "Inventory resources in a single Appwrite project", Run: RunInventory},
	{Name: "snapshot", Summary: "Write a deterministic manifest of a project's resources", Run: RunSnapshot},
	{Name: "compare", Summary: "Compare two local manifests without any network access", Run: RunCompare},
	{Name: "verify", Summary: "Compare source, expected, and destination state after a migration", Run: RunVerify},
	{Name: "preflight", Summary: "Check source/destination compatibility before a migration", Run: RunPreflight},
	{Name: "report", Summary: "Render a saved compare/verify result as text, JSON, or HTML", Run: RunReport},
	{Name: "version", Summary: "Print amg's version", Run: RunVersion},
}

// exitForParseError maps a flag.FlagSet.Parse error to an exit code.
// -h/--help intentionally exits ExitOK: it's a successful request for
// usage text, not a failure, matching what the top-level `amg --help`
// already does in cmd/amg/main.go. Before this, every subcommand's
// `fs.Parse` error path returned ExitBlock unconditionally, so
// `amg inventory --help` printed correct usage text but exited 2 —
// identical to a real failure, and indistinguishable from one by any
// script or CI step checking the exit code. Caught by a PM-level
// release-readiness review, not by any prior test, because no test
// asserted on --help's exit code specifically.
func exitForParseError(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	return ExitBlock
}

// Lookup returns the command named name, if any.
func Lookup(name string) (Command, bool) {
	for _, c := range Commands {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// Usage writes top-level usage text to w.
func Usage(w io.Writer) {
	fmt.Fprintln(w, "amg — Appwrite Migration Guard")
	fmt.Fprintln(w, "Verify Appwrite changes before they become incidents.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  amg <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range Commands {
		fmt.Fprintf(w, "  %-10s %s\n", c.Name, c.Summary)
	}
}
