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
// compare.Result (from `amg compare --json`/`amg verify --json`) or a
// ChecklistResult (from `amg doctor --json`/`amg preflight --json`) as
// text, JSON, or a self-contained HTML document. It makes no network
// calls and requires no Appwrite credentials — the input is
// already-captured JSON, not a live connection.
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
		fmt.Fprintln(stderr, "amg report: expected exactly 1 argument: <result.json> (from `amg compare --json`, `amg verify --json`, `amg doctor --json`, or `amg preflight --json`)")
		return ExitBlock
	}

	b, err := os.ReadFile(rest[0])
	if err != nil {
		fmt.Fprintln(stderr, "amg report: reading", rest[0]+":", err.Error())
		return ExitBlock
	}
	// Tolerate a leading UTF-8 BOM: encoding/json treats it as invalid
	// JSON, but it's a real thing this file will have in practice — see
	// internal/manifest's identical fix for why (Windows PowerShell's
	// `>` redirection).
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})

	kind, err := detectResultKind(b)
	if err != nil {
		fmt.Fprintln(stderr, "amg report: reading", rest[0]+":", err.Error())
		return ExitBlock
	}

	var buf bytes.Buffer
	var exitCode int
	switch kind {
	case resultKindCompare:
		exitCode = renderCompareResult(&buf, b, rest[0], *format, stderr)
	case resultKindChecklist:
		exitCode = renderChecklistResult(&buf, b, rest[0], *format, stderr)
	default:
		fmt.Fprintf(stderr, "amg report: %s does not look like a compare/verify result or a doctor/preflight checklist (no \"findings\" or \"checks\" field found)\n", rest[0])
		return ExitBlock
	}
	if buf.Len() == 0 {
		// render{Compare,Checklist}Result already printed a specific
		// error (bad schema version, decode failure, unknown --format)
		// and reset buf; exitCode is ExitBlock in every such case.
		return exitCode
	}

	if *out == "" {
		stdout.Write(buf.Bytes())
		return exitCode
	}
	if err := writeReportFile(*out, buf.Bytes()); err != nil {
		fmt.Fprintln(stderr, "amg report:", err.Error())
		return ExitBlock
	}
	fmt.Fprintf(stdout, "Wrote %s report to %s\n", *format, *out)
	return exitCode
}

const (
	resultKindCompare   = "compare"
	resultKindChecklist = "checklist"
	resultKindUnknown   = ""
)

// detectResultKind inspects b's top-level JSON keys to tell a
// compare.Result apart from a ChecklistResult, without guessing from
// file naming or a CLI flag the caller would have to remember to pass:
// compare.Result always serializes a "findings" key (possibly `null`/
// `[]`, but present), ChecklistResult always serializes "checks". Both
// keys are checked for actual presence in the decoded map, not merely
// unmarshaled into a lenient struct, so a file that happens to satisfy
// neither shape is reported as unrecognized rather than silently
// misrendered as one or the other.
func detectResultKind(b []byte) (string, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return resultKindUnknown, fmt.Errorf("decode: %w", err)
	}
	if _, ok := probe["findings"]; ok {
		return resultKindCompare, nil
	}
	if _, ok := probe["checks"]; ok {
		return resultKindChecklist, nil
	}
	return resultKindUnknown, nil
}

// renderCompareResult renders a compare.Result in the requested format
// into buf, returning the exit code exitForCompare would give, or
// ExitBlock (with buf left empty) if decoding/version-checking/
// rendering failed.
func renderCompareResult(buf *bytes.Buffer, b []byte, path, format string, stderr io.Writer) int {
	var res compare.Result
	if err := json.Unmarshal(b, &res); err != nil {
		fmt.Fprintln(stderr, "amg report: reading", path+":", "decode:", err.Error())
		return ExitBlock
	}
	if res.SchemaVersion > compare.ResultSchemaVersion {
		fmt.Fprintf(stderr, "amg report: %s was written by a newer amg (schema version %d > %d this build supports); upgrade amg before rendering it\n", path, res.SchemaVersion, compare.ResultSchemaVersion)
		return ExitBlock
	}

	switch format {
	case "text":
		writeCompareTerminal(buf, "Appwrite Migration Guard — report", &res)
	case "json":
		enc := json.NewEncoder(buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(&res); err != nil {
			fmt.Fprintln(stderr, "amg report: encode JSON:", err.Error())
			buf.Reset()
			return ExitBlock
		}
	case "html":
		if err := report.WriteHTML(buf, &res); err != nil {
			fmt.Fprintln(stderr, "amg report: render HTML:", err.Error())
			buf.Reset()
			return ExitBlock
		}
	default:
		fmt.Fprintf(stderr, "amg report: unknown --format %q (want text, json, or html)\n", format)
		buf.Reset()
		return ExitBlock
	}
	return exitForCompare(&res)
}

// renderChecklistResult is renderCompareResult's counterpart for a
// ChecklistResult (`amg doctor --json` / `amg preflight --json`).
func renderChecklistResult(buf *bytes.Buffer, b []byte, path, format string, stderr io.Writer) int {
	var res ChecklistResult
	if err := json.Unmarshal(b, &res); err != nil {
		fmt.Fprintln(stderr, "amg report: reading", path+":", "decode:", err.Error())
		return ExitBlock
	}
	if res.SchemaVersion > ChecklistSchemaVersion {
		fmt.Fprintf(stderr, "amg report: %s was written by a newer amg (schema version %d > %d this build supports); upgrade amg before rendering it\n", path, res.SchemaVersion, ChecklistSchemaVersion)
		return ExitBlock
	}

	title := fmt.Sprintf("Appwrite Migration Guard — report (%s)", res.Command)
	switch format {
	case "text":
		res.WriteTerminal(buf, title)
	case "json":
		enc := json.NewEncoder(buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(&res); err != nil {
			fmt.Fprintln(stderr, "amg report: encode JSON:", err.Error())
			buf.Reset()
			return ExitBlock
		}
	case "html":
		if err := writeChecklistHTML(buf, res); err != nil {
			fmt.Fprintln(stderr, "amg report: render HTML:", err.Error())
			buf.Reset()
			return ExitBlock
		}
	default:
		fmt.Fprintf(stderr, "amg report: unknown --format %q (want text, json, or html)\n", format)
		buf.Reset()
		return ExitBlock
	}

	// Derived from Checks, never the deserialized "overall" field — see
	// overallOfChecks's doc comment for why trusting stored data here
	// would be a real correctness gap.
	switch overallOfChecks(res.Checks) {
	case StatusPass:
		return ExitOK
	case StatusWarn:
		return ExitWarn
	default:
		return ExitBlock
	}
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
