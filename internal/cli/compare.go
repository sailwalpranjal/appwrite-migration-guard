package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/manifest"
)

// RunCompare implements `amg compare <manifest-a> <manifest-b>`: a fully
// offline comparison between two previously written manifests (spec
// section 31). It makes no network calls and requires no Appwrite
// credentials at all.
func RunCompare(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the comparison result as JSON instead of a terminal summary")
	strict := fs.Bool("strict", false, "use the strict policy: an unexpected destination resource is BLOCK instead of WARN (see docs/comparison-model.md#policy)")
	if err := fs.Parse(args); err != nil {
		return exitForParseError(err)
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, "amg compare: expected exactly 2 arguments: <manifest-a> <manifest-b>")
		return ExitBlock
	}

	a, err := manifest.Read(rest[0])
	if err != nil {
		fmt.Fprintln(stderr, "amg compare: reading", rest[0]+":", err.Error())
		return ExitBlock
	}
	b, err := manifest.Read(rest[1])
	if err != nil {
		fmt.Fprintln(stderr, "amg compare: reading", rest[1]+":", err.Error())
		return ExitBlock
	}

	policy := compare.DefaultPolicy()
	if *strict {
		policy = compare.StrictPolicy()
	}
	res := compare.CompareWithPolicy(a.Label, a.Inventory, b.Label, b.Inventory, policy)

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(stderr, "amg compare: encode JSON:", err.Error())
			return ExitBlock
		}
		return exitForCompare(res)
	}

	writeCompareTerminal(stdout, "Appwrite Migration Guard — compare", res)
	return exitForCompare(res)
}
