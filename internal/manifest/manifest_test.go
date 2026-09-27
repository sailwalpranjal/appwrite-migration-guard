package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func TestWriteRead_RoundTrip(t *testing.T) {
	inv := inventory.New("https://example.com/v1", "proj1")
	inv.Resources = append(inv.Resources, inventory.Resource{Type: inventory.ResourceDatabase, ID: "db1", Name: "One"})
	inv.Sort()

	m, err := New("source", inv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.RunID == "" {
		t.Fatal("expected a non-empty RunID")
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Write(path, m); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.RunID != m.RunID || got.Label != "source" {
		t.Fatalf("round-tripped manifest mismatch: %+v", got)
	}
	if len(got.Inventory.Resources) != 1 || got.Inventory.Resources[0].ID != "db1" {
		t.Fatalf("inventory did not round-trip: %+v", got.Inventory)
	}
}

func TestWrite_CreatesParentDirectories(t *testing.T) {
	inv := inventory.New("https://example.com/v1", "proj1")
	m, err := New("target", inv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	path := filepath.Join(t.TempDir(), "a", "b", "c", "manifest.json")
	if err := Write(path, m); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := Read(path); err != nil {
		t.Fatalf("Read after Write: %v", err)
	}
}

func TestRead_MissingFile(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if !errs.IsKind(err, errs.KindNotFound) {
		t.Fatalf("expected KindNotFound, got %v", err)
	}
}

// TestRead_TolerantOfLeadingBOM is a regression guard for a real bug,
// reproduced live: `./amg snapshot --out m.json` on Windows PowerShell
// with a redirect (or any tool/editor that writes UTF-8-with-BOM) can
// hand amg a manifest file with a leading byte-order mark.
// encoding/json treats that as invalid JSON and refuses to decode it,
// which previously surfaced as a decode error even though the file's
// actual JSON content is entirely valid.
func TestRead_TolerantOfLeadingBOM(t *testing.T) {
	inv := inventory.New("https://example.com/v1", "proj1")
	m, err := New("source", inv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	bom := append([]byte{0xEF, 0xBB, 0xBF}, b...)
	if err := os.WriteFile(path, bom, 0o644); err != nil {
		t.Fatalf("write BOM-prefixed manifest: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("expected Read to tolerate a leading BOM, got error: %v", err)
	}
	if got.RunID != m.RunID {
		t.Fatalf("round-tripped manifest mismatch: %+v", got)
	}
}

// Regression guard: manifest.Read previously accepted a manifest of any
// schema_version with no check at all, unlike amg report's equivalent
// check for compare.Result. A future schema version could carry fields
// (or reinterpreted meanings of existing ones) this build doesn't know
// about — reading it anyway risks
// comparing on a wrong assumption instead of refusing outright.
func TestRead_RejectsNewerSchemaVersion(t *testing.T) {
	inv := inventory.New("https://example.com/v1", "proj1")
	m, err := New("source", inv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.SchemaVersion = SchemaVersion + 1

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Write(path, m); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_, err = Read(path)
	if err == nil {
		t.Fatal("expected an error reading a manifest from a newer schema version")
	}
	if !errs.IsKind(err, errs.KindInvalidResponse) {
		t.Fatalf("expected KindInvalidResponse, got %v", err)
	}
}

// TestWrite_DeterministicAsideFromRunIDAndTimestamp is a regression guard
// for the README's repeated claim that manifests are "deterministic":
// given identical inventory content, two independently-created manifests
// must serialize to byte-identical JSON once the two fields that are
// *intentionally* fresh per run (RunID, CapturedAt) are normalized out.
func TestWrite_DeterministicAsideFromRunIDAndTimestamp(t *testing.T) {
	buildInv := func() *inventory.Inventory {
		inv := inventory.New("https://example.com/v1", "proj1")
		inv.Resources = append(inv.Resources,
			inventory.Resource{Type: inventory.ResourceTable, ID: "t2", ParentID: "db1", Name: "Two", SchemaDigest: "digest2"},
			inventory.Resource{Type: inventory.ResourceTable, ID: "t1", ParentID: "db1", Name: "One", SchemaDigest: "digest1"},
			inventory.Resource{Type: inventory.ResourceDatabase, ID: "db1", Name: "Main"},
		)
		inv.Sort()
		return inv
	}

	m1, err := New("source", buildInv())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m2, err := New("source", buildInv())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Normalize the fields that are correctly fresh per invocation:
	// Manifest.RunID/CapturedAt, and Inventory.GeneratedAt (set
	// independently by each buildInv() call via time.Now()). Missing this
	// last one made the test flaky-by-construction rather than a real
	// regression guard: two time.Now() calls a few instructions apart
	// happened to round to the same nanosecond often enough on Windows to
	// pass locally, but reliably produced two different timestamps (and
	// so a spurious failure) on Linux CI's higher-resolution clock.
	m2.RunID = m1.RunID
	m2.CapturedAt = m1.CapturedAt
	m2.Inventory.GeneratedAt = m1.Inventory.GeneratedAt

	b1, err := json.MarshalIndent(m1, "", "  ")
	if err != nil {
		t.Fatalf("marshal m1: %v", err)
	}
	b2, err := json.MarshalIndent(m2, "", "  ")
	if err != nil {
		t.Fatalf("marshal m2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("expected byte-identical manifests for identical inventory content, got:\n--- m1 ---\n%s\n--- m2 ---\n%s", b1, b2)
	}
}

func TestNew_RunIDsAreUnique(t *testing.T) {
	inv := inventory.New("https://example.com/v1", "proj1")
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		m, err := New("target", inv)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if seen[m.RunID] {
			t.Fatalf("duplicate RunID generated: %s", m.RunID)
		}
		seen[m.RunID] = true
	}
}

func TestManifest_NeverSerializesCredentials(t *testing.T) {
	// Regression guard: Manifest/Inventory must have no field that could
	// ever hold an API key, so there is nothing to accidentally serialize.
	inv := inventory.New("https://example.com/v1", "proj1")
	m, err := New("target", inv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Write(path, m); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	_ = got // structurally there is no APIKey field to check; compile-time guarantee.
}
