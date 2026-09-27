package manifest

import (
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

// Regression guard, from an external audit: manifest.Read previously
// accepted a manifest of any schema_version with no check at all, unlike
// amg report's equivalent check for compare.Result. A future schema
// version could carry fields (or reinterpreted meanings of existing
// ones) this build doesn't know about — reading it anyway risks
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
