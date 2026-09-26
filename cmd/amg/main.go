// Command amg is Appwrite Migration Guard's CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		cli.Usage(stderr)
		return cli.ExitBlock
	}

	name := args[0]
	if name == "-h" || name == "--help" || name == "help" {
		cli.Usage(stdout)
		return cli.ExitOK
	}

	cmd, ok := cli.Lookup(name)
	if !ok {
		fmt.Fprintf(stderr, "amg: unknown command %q\n\n", name)
		cli.Usage(stderr)
		return cli.ExitBlock
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return cmd.Run(ctx, args[1:], stdout, stderr)
}
