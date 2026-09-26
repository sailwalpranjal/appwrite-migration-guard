package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/version"
)

// RunVersion implements `amg version`.
func RunVersion(_ context.Context, _ []string, stdout, _ io.Writer) int {
	fmt.Fprintln(stdout, version.String())
	return ExitOK
}
