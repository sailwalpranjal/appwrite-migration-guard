package compare

import (
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func inv(resources ...inventory.Resource) *inventory.Inventory {
	i := inventory.New("https://example.com/v1", "proj1")
	i.Resources = resources
	i.Sort()
	return i
}

func table(id, dbID, name string, perms []string, meta map[string]any, rowCount int) inventory.Resource {
	return inventory.Resource{
		Type: inventory.ResourceTable, ID: id, ParentID: dbID, Name: name,
		Permissions: perms, Metadata: meta, RowCount: rowCount,
	}
}

func findRule(findings []Finding, rule string) *Finding {
	for i := range findings {
		if findings[i].Rule == rule {
			return &findings[i]
		}
	}
	return nil
}

// Scenario A (spec section 33): destination missing a resource -> BLOCK.
func TestCompare_MissingResource_Blocks(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, nil, 5))
	dst := inv()

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	f := findRule(res.Findings, RuleMissingResource)
	if f == nil {
		t.Fatal("expected a missing_resource finding")
	}
}

func TestCompare_UnexpectedResource_Warns(t *testing.T) {
	src := inv()
	dst := inv(table("t1", "db1", "Widgets", nil, nil, 5))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleUnexpectedResource) == nil {
		t.Fatal("expected an unexpected_resource finding")
	}
}

// Scenario C: permission changed -> BLOCK.
func TestCompare_PermissionChanged_Blocks(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", []string{"read(\"any\")"}, nil, 5))
	dst := inv(table("t1", "db1", "Widgets", []string{"read(\"users\")"}, nil, 5))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RulePermissionChanged) == nil {
		t.Fatal("expected a permission_changed finding")
	}
}

func TestCompare_PermissionOrderDoesNotMatter(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", []string{"read(\"any\")", "write(\"any\")"}, nil, 0))
	dst := inv(table("t1", "db1", "Widgets", []string{"write(\"any\")", "read(\"any\")"}, nil, 0))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS (reordered permissions are equal), got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_ConfigChanged_Blocks(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, map[string]any{"enabled": true}, 0))
	dst := inv(table("t1", "db1", "Widgets", nil, map[string]any{"enabled": false}, 0))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleConfigChanged) == nil {
		t.Fatal("expected a config_changed finding")
	}
}

func TestCompare_ParentChanged_Blocks(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, nil, 0))
	dst := inv(table("t1", "db2", "Widgets", nil, nil, 0))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleParentChanged) == nil {
		t.Fatal("expected a parent_changed finding")
	}
}

// Scenario D: an expected transformation must not be flagged at all.
// $createdAt/$updatedAt differ structurally between independently
// created resources and are never compared.
func TestCompare_TimestampsNeverCompared(t *testing.T) {
	src := inventory.Resource{Type: inventory.ResourceTable, ID: "t1", ParentID: "db1", Name: "Widgets", CreatedAt: "2020-01-01T00:00:00Z", UpdatedAt: "2020-01-01T00:00:00Z"}
	dst := inventory.Resource{Type: inventory.ResourceTable, ID: "t1", ParentID: "db1", Name: "Widgets", CreatedAt: "2026-09-27T00:00:00Z", UpdatedAt: "2026-09-27T00:00:00Z"}

	res := Compare("source", inv(src), "dest", inv(dst))
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS: differing timestamps alone must not produce findings, got %s (%+v)", res.Overall(), res.Findings)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("expected zero findings, got %+v", res.Findings)
	}
}

func TestCompare_RowCountMismatch_Blocks(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, nil, 100))
	dst := inv(table("t1", "db1", "Widgets", nil, nil, 90))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowCountMismatch) == nil {
		t.Fatal("expected a row_count_mismatch finding")
	}
}

func TestCompare_RowCountCapped_WarnsNotBlocks(t *testing.T) {
	s := table("t1", "db1", "Widgets", nil, nil, inventory.RowCountCap)
	s.RowCountCapped = true
	d := table("t1", "db1", "Widgets", nil, nil, inventory.RowCountCap)
	d.RowCountCapped = true
	// Even when both sides are capped, if the numeric counts happen to
	// be equal there is nothing meaningful to report.

	res := Compare("source", inv(s), "dest", inv(d))
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS when capped counts are numerically equal, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_RowCountCappedAndDiffering_Warns(t *testing.T) {
	s := table("t1", "db1", "Widgets", nil, nil, inventory.RowCountCap)
	s.RowCountCapped = true
	d := table("t1", "db1", "Widgets", nil, nil, 4000)

	res := Compare("source", inv(s), "dest", inv(d))
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowCountUnconfirmed) == nil {
		t.Fatal("expected a row_count_unconfirmed finding")
	}
}

func TestCompare_RowCountVerificationFailed_Warns(t *testing.T) {
	s := table("t1", "db1", "Widgets", nil, nil, 0)
	s.CountError = "permission denied"
	d := table("t1", "db1", "Widgets", nil, nil, 5)

	res := Compare("source", inv(s), "dest", inv(d))
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowCountUnverified) == nil {
		t.Fatal("expected a row_count_unverified finding")
	}
}

func TestCompare_IdenticalInventories_Pass(t *testing.T) {
	mk := func() *inventory.Inventory {
		return inv(
			inventory.Resource{Type: inventory.ResourceDatabase, ID: "db1", Name: "Main"},
			table("t1", "db1", "Widgets", []string{"read(\"any\")"}, map[string]any{"enabled": true}, 10),
		)
	}
	res := Compare("source", mk(), "dest", mk())
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("expected zero findings for identical inventories, got %+v", res.Findings)
	}
}

func TestCompare_Deterministic(t *testing.T) {
	src := inv(
		table("t2", "db1", "B", nil, nil, 1),
		table("t1", "db1", "A", nil, nil, 2),
	)
	dst := inv() // both missing -> two missing_resource findings, order must be stable

	r1 := Compare("source", src, "dest", dst)
	r2 := Compare("source", src, "dest", dst)
	if len(r1.Findings) != len(r2.Findings) {
		t.Fatalf("non-deterministic finding count: %d vs %d", len(r1.Findings), len(r2.Findings))
	}
	for i := range r1.Findings {
		if r1.Findings[i] != r2.Findings[i] {
			t.Fatalf("non-deterministic ordering at index %d: %+v vs %+v", i, r1.Findings[i], r2.Findings[i])
		}
	}
	if r1.Findings[0].ResourceID != "t1" {
		t.Fatalf("expected findings sorted by ResourceID, got order starting with %s", r1.Findings[0].ResourceID)
	}
}
