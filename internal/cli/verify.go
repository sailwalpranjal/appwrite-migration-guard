package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

const verifyTimeout = 3 * time.Minute

// RunVerify implements `amg verify`: it inventories both the AMG_SOURCE_*
// and AMG_DEST_* environments live, then runs the same offline comparison
// engine `amg compare` uses. This is the primary "prove the migration is
// safe" workflow step (spec section 7, steps 12-16).
func RunVerify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the comparison result as JSON instead of a terminal summary")
	noCounts := fs.Bool("no-row-counts", false, "skip per-table row counts (metadata only)")
	sampleRows := fs.Int("sample-rows", 0, "fetch up to N rows per table on both sides and compare content digests; 0 disables sampling (default)")
	concurrency := fs.Int("concurrency", inventory.DefaultConcurrency, "maximum concurrent Appwrite requests per side")
	strict := fs.Bool("strict", false, "use the strict policy: an unexpected destination resource is BLOCK instead of WARN (see docs/comparison-model.md#policy)")
	timeout := fs.Duration("timeout", verifyTimeout, "maximum time to allow the whole verify run — raise this for large projects that don't finish within the default")
	resources := fs.String("resources", "", resourcesFlagHelp)
	if err := fs.Parse(args); err != nil {
		return exitForParseError(err)
	}

	_ = config.LoadDotEnv(".env")
	srcEnv := config.Source()
	dstEnv := config.Destination()
	if err := srcEnv.Validate(); err != nil {
		fmt.Fprintln(stderr, "amg verify: source:", err.Error())
		return ExitBlock
	}
	if err := dstEnv.Validate(); err != nil {
		fmt.Fprintln(stderr, "amg verify: destination:", err.Error())
		return ExitBlock
	}

	runCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	opts := inventory.Options{Concurrency: *concurrency, CountRows: !*noCounts, SampleRows: *sampleRows, Resources: parseResourcesFlag(*resources)}

	var srcInv, dstInv *inventory.Inventory
	var srcErr, dstErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		client := appwrite.New(srcEnv)
		srcInv, srcErr = inventory.Collect(runCtx, client, srcEnv.Endpoint, srcEnv.ProjectID, opts)
	}()
	go func() {
		defer wg.Done()
		client := appwrite.New(dstEnv)
		dstInv, dstErr = inventory.Collect(runCtx, client, dstEnv.Endpoint, dstEnv.ProjectID, opts)
	}()
	wg.Wait()

	if srcErr != nil {
		fmt.Fprintln(stderr, "amg verify: source:", srcErr.Error())
		return ExitBlock
	}
	if dstErr != nil {
		fmt.Fprintln(stderr, "amg verify: destination:", dstErr.Error())
		return ExitBlock
	}

	policy := compare.DefaultPolicy()
	if *strict {
		policy = compare.StrictPolicy()
	}
	res := compare.CompareWithPolicy("source", srcInv, "destination", dstInv, policy)

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(stderr, "amg verify: encode JSON:", err.Error())
			return ExitBlock
		}
		return exitForCompare(res)
	}

	writeCompareTerminal(stdout, "Appwrite Migration Guard — verify", res)
	return exitForCompare(res)
}
