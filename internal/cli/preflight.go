package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

const preflightTimeout = 3 * time.Minute

// RunPreflight implements `amg preflight`: it checks, before a migration
// happens, whether the source and destination environments look ready
// (spec section 19). Unlike `amg verify` — which expects source and
// destination to already match — preflight runs *before* the migration,
// so a resource ID that exists on both sides is treated as a collision
// risk, not a success.
//
// It never writes to either Appwrite project.
func RunPreflight(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.SetOutput(stderr)
	concurrency := fs.Int("concurrency", inventory.DefaultConcurrency, "maximum concurrent Appwrite requests per side")
	timeout := fs.Duration("timeout", preflightTimeout, "maximum time to allow the whole preflight run — raise this for large projects that don't finish within the default")
	resources := fs.String("resources", "", resourcesFlagHelp)
	if err := fs.Parse(args); err != nil {
		return exitForParseError(err)
	}

	_ = config.LoadDotEnv(".env")
	srcEnv := config.Source()
	dstEnv := config.Destination()

	var list Checklist

	if err := srcEnv.Validate(); err != nil {
		list.Block("Source configuration", err.Error())
	} else {
		list.Pass("Source configuration present")
	}
	if err := dstEnv.Validate(); err != nil {
		list.Block("Destination configuration", err.Error())
	} else {
		list.Pass("Destination configuration present")
	}
	if list.Overall() == StatusBlock {
		// Nothing further can run without credentials on both sides.
		list.WriteTerminal(stdout, "Appwrite Migration Guard — preflight")
		return list.ExitCode()
	}

	runCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	srcClient := appwrite.New(srcEnv)
	dstClient := appwrite.New(dstEnv)

	srcVersion, srcReachable := checkReachable(runCtx, &list, "Source", srcClient)
	dstVersion, dstReachable := checkReachable(runCtx, &list, "Destination", dstClient)

	if srcReachable && dstReachable {
		if srcVersion == dstVersion {
			list.Pass(fmt.Sprintf("Appwrite version matches (%s)", srcVersion))
		} else {
			list.Warn("Appwrite version match",
				fmt.Sprintf("source is running %s, destination is running %s — behavior can differ between versions", srcVersion, dstVersion))
		}
	}

	srcAuthed := srcReachable && checkAuthenticated(runCtx, &list, "Source", srcClient)
	dstAuthed := dstReachable && checkAuthenticated(runCtx, &list, "Destination", dstClient)

	if srcAuthed && dstAuthed {
		checkInventoryAndConflicts(runCtx, &list, srcClient, dstClient, srcEnv, dstEnv, *concurrency, parseResourcesFlag(*resources))
	}

	list.WriteTerminal(stdout, "Appwrite Migration Guard — preflight")
	return list.ExitCode()
}

func checkReachable(ctx context.Context, list *Checklist, label string, client *appwrite.Client) (version string, ok bool) {
	v, err := client.Version(ctx)
	if err != nil {
		list.Block(label+" endpoint reachable", explain(err))
		return "", false
	}
	list.Pass(fmt.Sprintf("%s endpoint reachable (Appwrite %s)", label, v.Version))
	return v.Version, true
}

func checkAuthenticated(ctx context.Context, list *Checklist, label string, client *appwrite.Client) bool {
	if _, err := client.Health(ctx); err != nil {
		list.Block(label+" authentication", explainAuthError(err))
		return false
	}
	list.Pass(label + " authentication")
	return true
}

// checkInventoryAndConflicts inventories both environments concurrently
// (they share preflightTimeout's deadline, so running them serially could
// starve the second side's budget on a large source project — see
// RunVerify for the identical pattern) and looks for resource IDs that
// already exist on the destination before any migration has run — a real
// "destination conflict" risk (spec section 19), distinct from the
// post-migration comparison amg verify does.
func checkInventoryAndConflicts(ctx context.Context, list *Checklist, srcClient, dstClient *appwrite.Client, srcEnv, dstEnv config.Environment, concurrency int, resources []string) {
	opts := inventory.Options{Concurrency: concurrency, CountRows: false, Resources: resources}

	var srcInv, dstInv *inventory.Inventory
	var srcErr, dstErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		srcInv, srcErr = inventory.Collect(ctx, srcClient, srcEnv.Endpoint, srcEnv.ProjectID, opts)
	}()
	go func() {
		defer wg.Done()
		dstInv, dstErr = inventory.Collect(ctx, dstClient, dstEnv.Endpoint, dstEnv.ProjectID, opts)
	}()
	wg.Wait()

	if srcErr != nil {
		list.Block("Source resource inventory", srcErr.Error())
	} else {
		list.Pass(fmt.Sprintf("Source resource inventory (%d resources)", len(srcInv.Resources)))
	}
	if dstErr != nil {
		list.Block("Destination resource inventory", dstErr.Error())
	} else {
		list.Pass(fmt.Sprintf("Destination resource inventory (%d resources)", len(dstInv.Resources)))
	}
	if srcErr != nil || dstErr != nil {
		return
	}

	if conflicts := findIDConflicts(srcInv, dstInv); len(conflicts) > 0 {
		for _, r := range conflicts {
			list.Block("Destination conflict",
				fmt.Sprintf("%s %q already exists in the destination project — migrating could fail or overwrite it", r.Type, r.ID))
		}
	} else if len(resources) > 0 {
		// A --resources-narrowed run can only find conflicts in the
		// categories it actually collected — an unqualified PASS here
		// would otherwise read as "no conflicts anywhere," when
		// categories outside the filter (which could hold a real
		// colliding ID) were never inventoried on either side at all.
		list.Pass(fmt.Sprintf("No destination resource ID conflicts in requested categories (%s) — other categories were not checked", strings.Join(resources, ", ")))
	} else {
		list.Pass("No destination resource ID conflicts")
	}

	if len(srcInv.Unsupported) > 0 {
		list.Warn("Resource types requiring manual verification",
			fmt.Sprintf("amg does not yet inventory: %s", strings.Join(srcInv.Unsupported, ", ")))
	}
}

// findIDConflicts returns every source resource whose (Type, ID) already
// exists in the destination inventory.
func findIDConflicts(src, dst *inventory.Inventory) []inventory.Resource {
	existing := make(map[string]bool, len(dst.Resources))
	for _, r := range dst.Resources {
		existing[string(r.Type)+":"+r.ID] = true
	}
	var conflicts []inventory.Resource
	for _, r := range src.Resources {
		if existing[string(r.Type)+":"+r.ID] {
			conflicts = append(conflicts, r)
		}
	}
	return conflicts
}
