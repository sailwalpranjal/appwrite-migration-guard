package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/manifest"
)

const snapshotTimeout = 2 * time.Minute

// RunSnapshot implements `amg snapshot`: it inventories the APPWRITE_*
// target environment (identical collection to `amg inventory`) and
// persists the result as a manifest under .amg/runs/<run-id>/manifest.json
// (or --out), so it can later be compared fully offline with
// `amg compare`.
func RunSnapshot(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	label := fs.String("label", "target", "label recorded in the manifest (e.g. \"source\", \"destination\")")
	out := fs.String("out", "", "manifest output path (default: .amg/runs/<run-id>/manifest.json)")
	noCounts := fs.Bool("no-row-counts", false, "skip per-table row counts (metadata only)")
	sampleRows := fs.Int("sample-rows", 0, "fetch up to N rows per table and record a content digest for each; 0 disables sampling (default)")
	concurrency := fs.Int("concurrency", inventory.DefaultConcurrency, "maximum concurrent Appwrite requests")
	if err := fs.Parse(args); err != nil {
		return ExitBlock
	}

	_ = config.LoadDotEnv(".env")
	env := config.Target()
	if err := env.Validate(); err != nil {
		fmt.Fprintln(stderr, "amg snapshot:", err.Error())
		return ExitBlock
	}

	client := appwrite.New(env)
	runCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	inv, err := inventory.Collect(runCtx, client, env.Endpoint, env.ProjectID, inventory.Options{
		Concurrency: *concurrency,
		CountRows:   !*noCounts,
		SampleRows:  *sampleRows,
	})
	if err != nil {
		fmt.Fprintln(stderr, "amg snapshot:", err.Error())
		return ExitBlock
	}

	m, err := manifest.New(*label, inv)
	if err != nil {
		fmt.Fprintln(stderr, "amg snapshot:", err.Error())
		return ExitBlock
	}

	path := *out
	if path == "" {
		path = manifest.DefaultRunDir(".", m.RunID) + "/manifest.json"
	}
	if err := manifest.Write(path, m); err != nil {
		fmt.Fprintln(stderr, "amg snapshot:", err.Error())
		return ExitBlock
	}

	fmt.Fprintf(stdout, "Wrote manifest for %q (%d resources) to %s\n", *label, len(inv.Resources), path)
	return exitForInventory(inv)
}
