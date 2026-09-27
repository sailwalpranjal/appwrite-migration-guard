package cli

import (
	"bytes"
	"context"
	"testing"
)

// TestSubcommands_Help_ExitsOK is a regression guard for a P0 bug an
// external PM-level release-readiness review found: every subcommand
// using flag.FlagSet(..., flag.ContinueOnError) returned ExitBlock
// unconditionally on any fs.Parse error, including -h/--help — so
// `amg inventory --help` printed correct usage text but exited 2,
// identical to a real failure, and indistinguishable from one by any
// script or CI step checking the exit code. Only commands that actually
// parse flags are exercised here (doctor and version take none).
func TestSubcommands_Help_ExitsOK(t *testing.T) {
	for _, name := range []string{"compare", "inventory", "preflight", "report", "snapshot", "verify"} {
		cmd, ok := Lookup(name)
		if !ok {
			t.Fatalf("command %q not found in registry", name)
		}
		for _, helpFlag := range []string{"--help", "-h"} {
			var stdout, stderr bytes.Buffer
			code := cmd.Run(context.Background(), []string{helpFlag}, &stdout, &stderr)
			if code != ExitOK {
				t.Errorf("amg %s %s: expected ExitOK, got %d (stdout=%q stderr=%q)", name, helpFlag, code, stdout.String(), stderr.String())
			}
		}
	}
}

// TestSubcommands_UnknownFlag_StillBlocks is the flip side: a genuine
// parse failure (not a help request) must still exit ExitBlock, proving
// exitForParseError didn't accidentally make every parse error succeed.
func TestSubcommands_UnknownFlag_StillBlocks(t *testing.T) {
	for _, name := range []string{"compare", "inventory", "preflight", "report", "snapshot", "verify"} {
		cmd, _ := Lookup(name)
		var stdout, stderr bytes.Buffer
		code := cmd.Run(context.Background(), []string{"--this-flag-does-not-exist"}, &stdout, &stderr)
		if code != ExitBlock {
			t.Errorf("amg %s --this-flag-does-not-exist: expected ExitBlock, got %d", name, code)
		}
	}
}

// TestCommands_OrderMatchesReadmeWorkflow is a light regression guard
// against the two orderings (amg --help's command list, and the
// README's Quick Start/Example workflow walkthrough) silently drifting
// apart again — a PM review flagged the previous mismatch as an
// avoidable "did I miss a step" moment for a first-time user.
func TestCommands_OrderMatchesReadmeWorkflow(t *testing.T) {
	want := []string{"doctor", "inventory", "snapshot", "compare", "verify", "preflight", "report", "version"}
	if len(Commands) != len(want) {
		t.Fatalf("expected %d commands, got %d", len(want), len(Commands))
	}
	for i, name := range want {
		if Commands[i].Name != name {
			t.Fatalf("Commands[%d] = %q, want %q (want order: %v)", i, Commands[i].Name, name, want)
		}
	}
}
