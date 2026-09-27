package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/manifest"
)

func writeTestManifest(t *testing.T, dir, label string, resources ...inventory.Resource) string {
	t.Helper()
	inv := inventory.New("https://example.com/v1", "proj1")
	inv.Resources = resources
	inv.Sort()
	m, err := manifest.New(label, inv)
	if err != nil {
		t.Fatalf("manifest.New: %v", err)
	}
	path := filepath.Join(dir, label+".json")
	if err := manifest.Write(path, m); err != nil {
		t.Fatalf("manifest.Write: %v", err)
	}
	return path
}

func TestRunCompare_IdenticalManifests_Pass(t *testing.T) {
	dir := t.TempDir()
	res := inventory.Resource{Type: inventory.ResourceDatabase, ID: "db1", Name: "Main"}
	a := writeTestManifest(t, dir, "source", res)
	b := writeTestManifest(t, dir, "dest", res)

	var stdout, stderr bytes.Buffer
	code := RunCompare(context.Background(), []string{a, b}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d; stdout:\n%s stderr:\n%s", code, stdout.String(), stderr.String())
	}
}

func TestRunCompare_MissingResource_Blocks(t *testing.T) {
	dir := t.TempDir()
	res := inventory.Resource{Type: inventory.ResourceDatabase, ID: "db1", Name: "Main"}
	a := writeTestManifest(t, dir, "source", res)
	b := writeTestManifest(t, dir, "dest")

	var stdout, stderr bytes.Buffer
	code := RunCompare(context.Background(), []string{a, b}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d; stdout:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("missing_resource")) {
		t.Fatalf("expected missing_resource rule in output:\n%s", stdout.String())
	}
}

func TestRunCompare_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCompare(context.Background(), []string{"only-one.json"}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
}

func TestRunCompare_NewerSchemaVersionManifest_Blocks(t *testing.T) {
	dir := t.TempDir()
	inv := inventory.New("https://example.com/v1", "proj1")
	m, err := manifest.New("source", inv)
	if err != nil {
		t.Fatalf("manifest.New: %v", err)
	}
	m.SchemaVersion = manifest.SchemaVersion + 1
	futurePath := filepath.Join(dir, "future.json")
	if err := manifest.Write(futurePath, m); err != nil {
		t.Fatalf("manifest.Write: %v", err)
	}
	b := writeTestManifest(t, dir, "dest")

	var stdout, stderr bytes.Buffer
	code := RunCompare(context.Background(), []string{futurePath, b}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock reading a from-the-future manifest, got %d; stderr:\n%s", code, stderr.String())
	}
}

func TestRunCompare_NonexistentManifest(t *testing.T) {
	dir := t.TempDir()
	a := writeTestManifest(t, dir, "source")

	var stdout, stderr bytes.Buffer
	code := RunCompare(context.Background(), []string{a, filepath.Join(dir, "does-not-exist.json")}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock, got %d", code)
	}
}

func TestRunCompare_Strict_BlocksUnexpectedResource(t *testing.T) {
	dir := t.TempDir()
	extra := inventory.Resource{Type: inventory.ResourceDatabase, ID: "db1", Name: "Main"}
	a := writeTestManifest(t, dir, "source")
	b := writeTestManifest(t, dir, "dest", extra)

	var stdout, stderr bytes.Buffer
	code := RunCompare(context.Background(), []string{a, b}, &stdout, &stderr)
	if code != ExitWarn {
		t.Fatalf("expected ExitWarn without --strict, got %d; stdout:\n%s", code, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = RunCompare(context.Background(), []string{"--strict", a, b}, &stdout, &stderr)
	if code != ExitBlock {
		t.Fatalf("expected ExitBlock with --strict, got %d; stdout:\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("BLOCK")) || !bytes.Contains(stdout.Bytes(), []byte("unexpected_resource")) {
		t.Fatalf("expected a BLOCK unexpected_resource finding under --strict:\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Policy:      strict")) {
		t.Fatalf("expected the terminal output to record the strict policy:\n%s", stdout.String())
	}
}

func TestRunCompare_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	a := writeTestManifest(t, dir, "source")
	b := writeTestManifest(t, dir, "dest")

	var stdout, stderr bytes.Buffer
	code := RunCompare(context.Background(), []string{"--json", a, b}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ExitOK, got %d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"source_label"`)) {
		t.Fatalf("expected JSON output, got:\n%s", stdout.String())
	}
}
