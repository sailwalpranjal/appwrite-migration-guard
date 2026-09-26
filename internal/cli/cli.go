// Package cli implements amg's subcommands. Each command is a small
// function of (args []string, stdout, stderr io.Writer) -> exit code, kept
// free of global state so it can be tested directly.
package cli

import (
	"context"
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
// display order.
var Commands = []Command{
	{Name: "version", Summary: "Print amg's version", Run: RunVersion},
	{Name: "doctor", Summary: "Check that amg itself is configured correctly", Run: RunDoctor},
	{Name: "inventory", Summary: "Inventory resources in a single Appwrite project", Run: RunInventory},
	{Name: "preflight", Summary: "Check source/destination compatibility before a migration", Run: RunPreflight},
	{Name: "snapshot", Summary: "Write a deterministic manifest of a project's resources", Run: RunSnapshot},
	{Name: "verify", Summary: "Compare source, expected, and destination state after a migration", Run: RunVerify},
	{Name: "report", Summary: "Render a run's results as terminal/JSON/HTML output", Run: RunReport},
	{Name: "compare", Summary: "Compare two local manifests without any network access", Run: RunCompare},
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
