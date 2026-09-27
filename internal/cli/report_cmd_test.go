package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func writeTestResultFile(t *testing.T, dir string, res *compare.Result) string {
	t.Helper()
	path := filepath.Join(dir, "result.json")
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func sampleResult(overall compare.Severity) *compare.Result {
	res := &compare.Result{SchemaVersion: compare.ResultSchemaVersion, SourceLabel: "source", DestLabel: "destination"}
	switch overall {
	case compare.SeverityBlock:
		res.Findings = []compare.Finding{{Severity: compare.SeverityBlock, Rule: compare.RuleMissingResource, ResourceType: inventory.ResourceTable, ResourceID: "t1", Message: "table \"t1\" is missing"}}
	case compare.SeverityWarn:
		res.Findings = []compare.Finding{{Severity: compare.SeverityWarn, Rule: compare.RuleUnexpectedResource, ResourceType: inventory.ResourceTable, ResourceID: "t1", Message: "unexpected"}}
	}
	return res
}

func TestRunReport_TextFormat_Default(t *testing.T) {
	dir := t.TempDir()
	path := writeTestResultFile(t, dir, sampleResult(compare.SeverityBlock))

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{path}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stderr:\n%s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("missing_resource")) {
		t.Fatalf("expected rule name in text output:\n%s", stdout.String())
	}
}

func TestRunReport_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeTestResultFile(t, dir, sampleResult(compare.SeverityWarn))

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{"--format", "json", path}, &stdout, &stderr)
	if code != ExitWarn {
		t.Fatalf("expected ExitWarn, got %d", code)
	}
	var decoded compare.Result
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("output was not valid JSON: %v\n%s", err, stdout.String())
	}
	if decoded.SourceLabel != "source" {
		t.Fatalf("unexpected decoded result: %+v", decoded)
	}
}

func TestRunReport_HTMLFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeTestResultFile(t, dir, sampleResult(compare.SeverityPass))

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{"--format", "html", path}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stderr:\n%s", code, stderr.String())
	}
	if !bytes.HasPrefix(stdout.Bytes(), []byte("<!doctype html>")) {
		t.Fatalf("expected HTML output, got:\n%s", stdout.String())
	}
}

func TestRunReport_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	inPath := writeTestResultFile(t, dir, sampleResult(compare.SeverityPass))
	outPath := filepath.Join(dir, "nested", "report.html")

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{"--format", "html", "--out", outPath, inPath}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stderr:\n%s", code, stderr.String())
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
	if !bytes.Contains(b, []byte("<!doctype html>")) {
		t.Fatalf("expected HTML content in output file, got:\n%s", string(b))
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Wrote html report to")) {
		t.Fatalf("expected a confirmation message, got:\n%s", stdout.String())
	}
}

func TestRunReport_UnknownFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeTestResultFile(t, dir, sampleResult(compare.SeverityPass))

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{"--format", "xml", path}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
}

func TestRunReport_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{filepath.Join(t.TempDir(), "does-not-exist.json")}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
}

// TestRunReport_TolerantOfLeadingBOM is a regression guard for a real
// bug reproduced live: `./amg verify --json > result.json` on Windows
// PowerShell writes UTF-8 with a leading byte-order mark (PowerShell's
// `>` redirection default, and PowerShell is amg's documented Windows
// shell), and encoding/json treats a BOM as invalid JSON — so the exact
// documented `verify --json > result.json` then `report result.json`
// workflow failed to decode the BOM-prefixed file before this fix.
func TestRunReport_TolerantOfLeadingBOM(t *testing.T) {
	dir := t.TempDir()
	res := sampleResult(compare.SeverityPass)
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "result.json")
	bom := append([]byte{0xEF, 0xBB, 0xBF}, b...)
	if err := os.WriteFile(path, bom, 0o644); err != nil {
		t.Fatalf("write BOM-prefixed result: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{path}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK reading a BOM-prefixed result, got %d; stderr:\n%s", code, stderr.String())
	}
}

func TestRunReport_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), nil, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
}

func TestRunReport_NewerSchemaVersionRejected(t *testing.T) {
	dir := t.TempDir()
	res := sampleResult(compare.SeverityPass)
	res.SchemaVersion = compare.ResultSchemaVersion + 1
	path := writeTestResultFile(t, dir, res)

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{path}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock for a from-the-future schema version, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("newer amg")) {
		t.Fatalf("expected an explanatory message, got:\n%s", stderr.String())
	}
}

func TestRunReport_RoundTripFromRealCompare(t *testing.T) {
	// End-to-end within the process: amg compare --json output must be
	// exactly what amg report can read back in.
	dir := t.TempDir()
	a := writeTestManifest(t, dir, "source", inventoryResourceForTest("db1"))
	b := writeTestManifest(t, dir, "dest")

	var compareOut, compareErr bytes.Buffer
	RunCompare(context.Background(), []string{"--json", a, b}, &compareOut, &compareErr)

	resultPath := filepath.Join(dir, "result.json")
	if err := os.WriteFile(resultPath, compareOut.Bytes(), 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	var reportOut, reportErr bytes.Buffer
	code := RunReport(context.Background(), []string{"--format", "html", resultPath}, &reportOut, &reportErr)
	if code != ExitBlock { // the missing database is a real BLOCK finding
		t.Fatalf("expected ExitBlock, got %d; stderr:\n%s", code, reportErr.String())
	}
	if !bytes.Contains(reportOut.Bytes(), []byte("missing_resource")) {
		t.Fatalf("expected the missing_resource rule to survive the round trip:\n%s", reportOut.String())
	}
}

func inventoryResourceForTest(id string) inventory.Resource {
	return inventory.Resource{Type: inventory.ResourceDatabase, ID: id, Name: id}
}

// TestRunReport_RoundTripFromRealDoctor is the ChecklistResult
// counterpart to TestRunReport_RoundTripFromRealCompare: `amg doctor
// --json` output must be exactly what `amg report` can read back in
// and render as HTML — closing the README's previously-documented
// limitation that report's HTML output covered compare/verify only.
func TestRunReport_RoundTripFromRealDoctor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/version":
			json.NewEncoder(w).Encode(map[string]any{"version": "2.3.0"})
		case "/health":
			json.NewEncoder(w).Encode(map[string]any{"name": "http", "status": "pass"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	withEnv(t, srv.URL, "proj1", "key1")

	var doctorOut, doctorErr bytes.Buffer
	code := RunDoctor(context.Background(), []string{"--json"}, &doctorOut, &doctorErr)
	if code != ExitOK {
		t.Fatalf("amg doctor --json: expected ExitOK, got %d; stderr:\n%s", code, doctorErr.String())
	}

	dir := t.TempDir()
	resultPath := filepath.Join(dir, "doctor-result.json")
	if err := os.WriteFile(resultPath, doctorOut.Bytes(), 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	var reportOut, reportErr bytes.Buffer
	reportCode := RunReport(context.Background(), []string{"--format", "html", resultPath}, &reportOut, &reportErr)
	if reportCode != ExitOK {
		t.Fatalf("amg report: expected ExitOK, got %d; stderr:\n%s", reportCode, reportErr.String())
	}
	out := reportOut.String()
	if !strings.Contains(out, "<!doctype html>") {
		t.Fatalf("expected a complete HTML document:\n%s", out)
	}
	if !strings.Contains(out, "doctor") {
		t.Fatalf("expected the command name to appear in the rendered report:\n%s", out)
	}
	if !strings.Contains(out, "badge-PASS") {
		t.Fatalf("expected a PASS badge:\n%s", out)
	}
}

// TestRunReport_ChecklistResult_TextAndJSONFormats proves the same
// ChecklistResult round-trips correctly through the other two formats,
// not just HTML.
func TestRunReport_ChecklistResult_TextAndJSONFormats(t *testing.T) {
	dir := t.TempDir()
	cl := ChecklistResult{SchemaVersion: ChecklistSchemaVersion, Command: "preflight", Overall: StatusBlock,
		Checks: []Check{{Status: StatusBlock, Title: "Destination conflict", Detail: "database \"db1\" already exists"}}}
	b, err := json.Marshal(cl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "result.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var textOut, textErr bytes.Buffer
	code := RunReport(context.Background(), []string{path}, &textOut, &textErr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock for the BLOCK checklist, got %d; stderr:\n%s", code, textErr.String())
	}
	if !bytes.Contains(textOut.Bytes(), []byte("Destination conflict")) {
		t.Fatalf("expected the check title in text output:\n%s", textOut.String())
	}

	var jsonOut, jsonErr bytes.Buffer
	code = RunReport(context.Background(), []string{"--format", "json", path}, &jsonOut, &jsonErr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stderr:\n%s", code, jsonErr.String())
	}
	var roundTripped ChecklistResult
	if err := json.Unmarshal(jsonOut.Bytes(), &roundTripped); err != nil {
		t.Fatalf("expected valid JSON output, got %v:\n%s", err, jsonOut.String())
	}
	if roundTripped.Command != "preflight" || roundTripped.Overall != StatusBlock {
		t.Fatalf("unexpected round-tripped result: %+v", roundTripped)
	}
}

// TestRunReport_UnrecognizedShape_Blocks proves a JSON file that is
// neither a compare.Result nor a ChecklistResult (no "findings" or
// "checks" key) produces a clear, actionable error rather than being
// silently misrendered as one shape or the other.
func TestRunReport_UnrecognizedShape_Blocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-result.json")
	if err := os.WriteFile(path, []byte(`{"hello":"world"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{path}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("does not look like")) {
		t.Fatalf("expected an explanatory message, got:\n%s", stderr.String())
	}
}

// TestRunReport_ChecklistResult_NeverTrustsStoredOverallField is a
// regression guard caught in self-review: renderChecklistResult
// originally switched on the deserialized "overall" JSON field
// verbatim instead of deriving it from "checks", the way
// compare.Result's Overall() is always a derived method, never stored
// data. A hand-edited, corrupted-in-transit, or future/buggy amg's
// ChecklistResult JSON carrying "checks":[...BLOCK...] alongside
// "overall":"PASS" must still exit BLOCK — trusting the mismatched
// field would silently green-light a CI pipeline gated on amg report's
// exit code.
func TestRunReport_ChecklistResult_NeverTrustsStoredOverallField(t *testing.T) {
	dir := t.TempDir()
	// Deliberately inconsistent: a BLOCK check, but overall claims PASS.
	raw := `{"schema_version":1,"command":"doctor","overall":"PASS","checks":[{"status":"BLOCK","title":"Endpoint reachable","detail":"connection refused"}]}`
	path := filepath.Join(dir, "result.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, format := range []string{"text", "json", "html"} {
		t.Run(format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RunReport(context.Background(), []string{"--format", format, path}, &stdout, &stderr)
			if code != ExitBlock {
				t.Fatalf("expected ExitBlock despite the stored overall:\"PASS\" field, got %d; stdout:\n%s", code, stdout.String())
			}
		})
	}
}

// TestRunReport_NewerChecklistSchemaVersionRejected is the
// ChecklistResult counterpart to TestRunReport_NewerSchemaVersionRejected.
func TestRunReport_NewerChecklistSchemaVersionRejected(t *testing.T) {
	dir := t.TempDir()
	cl := ChecklistResult{SchemaVersion: ChecklistSchemaVersion + 1, Command: "doctor", Overall: StatusPass,
		Checks: []Check{{Status: StatusPass, Title: "ok"}}}
	b, err := json.Marshal(cl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "result.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := RunReport(context.Background(), []string{path}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock for a from-the-future schema version, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("newer amg")) {
		t.Fatalf("expected an explanatory message, got:\n%s", stderr.String())
	}
}
