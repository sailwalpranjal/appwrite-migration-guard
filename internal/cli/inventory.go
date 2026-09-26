package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/appwrite"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

const inventoryTimeout = 2 * time.Minute

// RunInventory implements `amg inventory`: it connects to the single
// APPWRITE_* target environment and inventories its TablesDB databases,
// tables, and (by default) per-table row counts. It never writes to the
// Appwrite project.
func RunInventory(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inventory", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the inventory as JSON instead of a terminal summary")
	noCounts := fs.Bool("no-row-counts", false, "skip per-table row counts (metadata only)")
	concurrency := fs.Int("concurrency", inventory.DefaultConcurrency, "maximum concurrent Appwrite requests")
	if err := fs.Parse(args); err != nil {
		return ExitBlock
	}

	_ = config.LoadDotEnv(".env")
	env := config.Target()
	if err := env.Validate(); err != nil {
		fmt.Fprintln(stderr, "amg inventory:", err.Error())
		return ExitBlock
	}

	client := appwrite.New(env)
	runCtx, cancel := context.WithTimeout(ctx, inventoryTimeout)
	defer cancel()

	inv, err := inventory.Collect(runCtx, client, env.Endpoint, env.ProjectID, inventory.Options{
		Concurrency: *concurrency,
		CountRows:   !*noCounts,
	})
	if err != nil {
		fmt.Fprintln(stderr, "amg inventory:", err.Error())
		return ExitBlock
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(inv); err != nil {
			fmt.Fprintln(stderr, "amg inventory: encode JSON:", err.Error())
			return ExitBlock
		}
		return exitForInventory(inv)
	}

	writeInventorySummary(stdout, inv)
	return exitForInventory(inv)
}

func exitForInventory(inv *inventory.Inventory) int {
	if inv.PartiallyVerified() {
		return ExitWarn
	}
	return ExitOK
}

func writeInventorySummary(w io.Writer, inv *inventory.Inventory) {
	fmt.Fprintln(w, "Appwrite Migration Guard — inventory")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Endpoint:  %s\n", inv.Endpoint)
	fmt.Fprintf(w, "Project:   %s\n", inv.ProjectID)
	fmt.Fprintf(w, "Generated: %s\n", inv.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintln(w)

	counts := inv.CountByType()
	fmt.Fprintf(w, "%-5s %d database(s)\n", StatusPass, counts[inventory.ResourceDatabase])
	fmt.Fprintf(w, "%-5s %d table(s)\n", StatusPass, counts[inventory.ResourceTable])
	fmt.Fprintln(w)

	for _, r := range inv.Resources {
		switch r.Type {
		case inventory.ResourceDatabase:
			fmt.Fprintf(w, "database  %-24s %s\n", r.ID, r.Name)
		case inventory.ResourceTable:
			rows := fmt.Sprintf("%d rows", r.RowCount)
			if r.RowCountCapped {
				rows = fmt.Sprintf(">=%d rows (capped)", r.RowCount)
			}
			if r.CountError != "" {
				fmt.Fprintf(w, "  %-5s table   %-22s %-30s row count unavailable: %s\n", StatusWarn, r.ID, r.Name, r.CountError)
			} else {
				fmt.Fprintf(w, "  %-5s table   %-22s %-30s %s\n", StatusPass, r.ID, r.Name, rows)
			}
		}
	}

	if len(inv.Unsupported) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Not yet inventoried by amg (see roadmap):")
		for _, u := range inv.Unsupported {
			fmt.Fprintf(w, "  %-5s %s\n", StatusWarn, u)
		}
	}

	fmt.Fprintln(w)
	if inv.PartiallyVerified() {
		fmt.Fprintf(w, "Result: %s (some row counts were not verified)\n", StatusWarn)
	} else {
		fmt.Fprintf(w, "Result: %s\n", StatusPass)
	}
}
