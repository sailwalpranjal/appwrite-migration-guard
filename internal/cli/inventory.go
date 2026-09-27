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
// APPWRITE_* target environment and inventories its TablesDB databases
// and tables (with, by default, a per-table row count), its Storage
// buckets and files (including each file's MD5 content signature), and
// its Users (administrative/verification state only — never email,
// phone, prefs, or credential material), and its Functions (config only
// — never environment variables, which routinely hold secrets; see
// docs/migration-semantics.md). It never writes to the Appwrite project
// and never downloads file content.
func RunInventory(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inventory", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the inventory as JSON instead of a terminal summary")
	noCounts := fs.Bool("no-row-counts", false, "skip per-table row counts (metadata only)")
	sampleRows := fs.Int("sample-rows", 0, "fetch up to N rows per table (ordered by $id) and record a content digest for each; 0 disables sampling (default). This reads real row data — opt in deliberately.")
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
		SampleRows:  *sampleRows,
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
	fmt.Fprintf(w, "%-5s %d bucket(s)\n", StatusPass, counts[inventory.ResourceBucket])
	fmt.Fprintf(w, "%-5s %d file(s)\n", StatusPass, counts[inventory.ResourceFile])
	fmt.Fprintf(w, "%-5s %d user(s)\n", StatusPass, counts[inventory.ResourceUser])
	fmt.Fprintf(w, "%-5s %d function(s)\n", StatusPass, counts[inventory.ResourceFunction])
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
			if r.SampleError != "" {
				fmt.Fprintf(w, "         %-5s row sample unavailable: %s\n", StatusWarn, r.SampleError)
			} else if len(r.RowSamples) > 0 {
				fmt.Fprintf(w, "         %-5s %d row(s) sampled and fingerprinted\n", StatusPass, len(r.RowSamples))
			}
		case inventory.ResourceBucket:
			fmt.Fprintf(w, "bucket    %-24s %s\n", r.ID, r.Name)
		case inventory.ResourceFile:
			fmt.Fprintf(w, "  %-5s file    %-22s %-30s md5:%s\n", StatusPass, r.ID, r.Name, r.ContentDigest)
		case inventory.ResourceUser:
			status := "disabled"
			if enabled, _ := r.Metadata["enabled"].(bool); enabled {
				status = "enabled"
			}
			fmt.Fprintf(w, "user      %-24s %-30s %s\n", r.ID, r.Name, status)
		case inventory.ResourceFunction:
			runtime, _ := r.Metadata["runtime"].(string)
			fmt.Fprintf(w, "function  %-24s %-30s %s\n", r.ID, r.Name, runtime)
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
	if reason := partialVerificationReason(inv); reason != "" {
		fmt.Fprintf(w, "Result: %s (%s)\n", StatusWarn, reason)
	} else {
		fmt.Fprintf(w, "Result: %s\n", StatusPass)
	}
}

// partialVerificationReason describes *what* was only partially verified
// — a row-count failure and a row-sampling failure are different
// problems with different causes, so the summary must not claim one
// happened when it was really the other.
func partialVerificationReason(inv *inventory.Inventory) string {
	var countFailed, sampleFailed bool
	for _, r := range inv.Resources {
		if r.CountError != "" {
			countFailed = true
		}
		if r.SampleError != "" {
			sampleFailed = true
		}
	}
	switch {
	case countFailed && sampleFailed:
		return "some row counts and row samples were not verified"
	case countFailed:
		return "some row counts were not verified"
	case sampleFailed:
		return "some row samples were not verified"
	default:
		return ""
	}
}
