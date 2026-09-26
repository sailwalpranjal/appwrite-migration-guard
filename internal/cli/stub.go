package cli

import (
	"context"
	"fmt"
	"io"
)

// notImplemented prints an explicit, honest "not built yet" message and
// returns ExitBlock. amg must never claim success for work it did not do
// (spec section 33/47): a command that doesn't exist yet fails loudly
// rather than exiting 0.
func notImplemented(name string) func(context.Context, []string, io.Writer, io.Writer) int {
	return func(_ context.Context, _ []string, _ io.Writer, stderr io.Writer) int {
		fmt.Fprintf(stderr, "amg %s: not yet implemented in this build\n", name)
		fmt.Fprintln(stderr, "See https://github.com/sailwalpranjal/appwrite-migration-guard for roadmap status.")
		return ExitBlock
	}
}

var (
	RunInventory = notImplemented("inventory")
	RunPreflight = notImplemented("preflight")
	RunSnapshot  = notImplemented("snapshot")
	RunVerify    = notImplemented("verify")
	RunReport    = notImplemented("report")
	RunCompare   = notImplemented("compare")
)
