package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
