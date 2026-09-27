package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/report"
)

// RunReport implements `amg report <result.json>`: it re-renders a
// compare.Result previously saved via `amg compare --json` or
// `amg verify --json` as text, JSON, or a self-contained HTML document.
// It makes no network calls and requires no Appwrite credentials — the
// input is already-captured JSON, not a live connection.
func RunReport(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text, json, or html")
	out := fs.String("out", "", "write the report to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return exitForParseError(err)
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "amg report: expected exactly 1 argument: <result.json> (from `amg compare --json` or `amg verify --json`)")
		return ExitBlock
	}

	res, err := readResult(rest[0])
	if err != nil {
		fmt.Fprintln(stderr, "amg report: reading", rest[0]+":", err.Error())
		return ExitBlock
	}
	if res.SchemaVersion > compare.ResultSchemaVersion {
		fmt.Fprintf(stderr, "amg report: %s was written by a newer amg (schema version %d > %d this build supports); upgrade amg before rendering it\n", rest[0], res.SchemaVersion, compare.ResultSchemaVersion)
		return ExitBlock
	}

	var buf bytes.Buffer
	switch *format {
	case "text":
		writeCompareTerminal(&buf, "Appwrite Migration Guard — report", res)
	case "json":
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(stderr, "amg report: encode JSON:", err.Error())
			return ExitBlock
		}
	case "html":
		if err := report.WriteHTML(&buf, res); err != nil {
			fmt.Fprintln(stderr, "amg report: render HTML:", err.Error())
			return ExitBlock
		}
	default:
		fmt.Fprintf(stderr, "amg report: unknown --format %q (want text, json, or html)\n", *format)
		return ExitBlock
	}

	if *out == "" {
		stdout.Write(buf.Bytes())
		return exitForCompare(res)
	}

	if err := writeReportFile(*out, buf.Bytes()); err != nil {
		fmt.Fprintln(stderr, "amg report:", err.Error())
		return ExitBlock
	}
	fmt.Fprintf(stdout, "Wrote %s report to %s\n", *format, *out)
	return exitForCompare(res)
}

// readResult loads and decodes a compare.Result previously written by
// `amg compare --json` / `amg verify --json`.
func readResult(path string) (*compare.Result, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Tolerate a leading UTF-8 BOM: encoding/json treats it as invalid
	// JSON, but it's a real thing this file will have in practice — on
	// Windows, PowerShell's `>` redirection (the natural way to capture
	// `amg compare --json`/`amg verify --json`'s output, and PowerShell
	// is amg's documented Windows shell) writes UTF-8 with a BOM by
	// default. Reproduced live: `amg verify --json > result.json` in
	// PowerShell followed by `amg report result.json` failed to decode
	// the BOM-prefixed file before this fix.
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var res compare.Result
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &res, nil
}

// writeReportFile writes b to path, creating parent directories as
// needed. amg is a local, single-user CLI operating with the invoking
// user's own filesystem permissions (not a hosted, multi-tenant
// service), so the relevant safety property is "don't silently clobber
// something unexpected," not sandboxing against a remote attacker: path
// is used as given (cleaned by the OS/filepath layer as normal), and the
// destination directory is created rather than assumed to exist.
func writeReportFile(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", dir, err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
