package compare

import (
	"encoding/json"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func inv(resources ...inventory.Resource) *inventory.Inventory {
	i := inventory.New("https://example.com/v1", "proj1")
	i.Resources = resources
	i.Sort()
	return i
}

// defaultSchemaDigest is a fixed, non-empty stand-in for a real
// appwrite.Table's computed schema digest. Real inventory.Collect always
// populates SchemaDigest for tables (unlike the opt-in RowSamples), so
// two "identical" test tables should share this same value unless a test
// is specifically exercising schema-change/schema-unverified behavior.
const defaultSchemaDigest = "test-schema-digest"

func table(id, dbID, name string, perms []string, meta map[string]any, rowCount int) inventory.Resource {
	return inventory.Resource{
		Type: inventory.ResourceTable, ID: id, ParentID: dbID, Name: name,
		Permissions: perms, Metadata: meta, RowCount: rowCount,
		SchemaDigest: defaultSchemaDigest,
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

// TestCompare_Idempotent is a regression guard for the README's repeated
// claim of determinism: running Compare three times over the exact same
// two inventories must produce byte-identical JSON output every time —
// same finding order, same content — not just an equal Overall()
// severity. sortFindings makes this true by construction, but nothing
// previously proved it against a non-trivial, multi-resource, multi-rule
// fixture exercising several finding types at once.
func TestCompare_Idempotent(t *testing.T) {
	src := inv(
		table("t1", "db1", "Widgets", []string{"read(\"any\")"}, map[string]any{"enabled": true, "row_security": false}, 5),
		table("t2", "db1", "Gadgets", nil, nil, 10),
	)
	dst := inv(
		table("t1", "db1", "Widgets", []string{"read(\"users\")"}, map[string]any{"enabled": false, "row_security": false}, 5),
	)

	var results [][]byte
	for i := 0; i < 3; i++ {
		res := Compare("source", src, "dest", dst)
		b, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("run %d: marshal: %v", i, err)
		}
		results = append(results, b)
	}
	for i := 1; i < len(results); i++ {
		if string(results[i]) != string(results[0]) {
			t.Fatalf("Compare is not idempotent: run 0 and run %d differ:\n--- run 0 ---\n%s\n--- run %d ---\n%s", i, results[0], i, results[i])
		}
	}
}

// TestCompare_CoverageMismatch_DetectedWhenResourcesFilterDiffers is a
// regression guard for the --resources filter: comparing an inventory
// collected with Resources=["tables"] against one collected
// unrestricted must surface that the two runs checked different
// categories, since a resource type present only on the unfiltered
// side would otherwise look like ordinary unexpected_resource drift
// rather than "this side was never asked to check that category."
func TestCompare_CoverageMismatch_DetectedWhenResourcesFilterDiffers(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, nil, 5))
	src.Collected = []string{"tables"}
	dst := inv(table("t1", "db1", "Widgets", nil, nil, 5))
	dst.Collected = inventory.ResourceCategories

	res := Compare("source", src, "dest", dst)
	if !res.CoverageMismatch() {
		t.Fatal("expected CoverageMismatch to be true when source/dest Collected differ")
	}
	if !equalStringSets(res.SourceCollected, []string{"tables"}) {
		t.Fatalf("expected SourceCollected to carry through, got %v", res.SourceCollected)
	}
}

// TestCompare_CoverageMismatch_FalseWhenSame proves the check doesn't
// false-positive for two runs that collected the same categories,
// regardless of slice order.
func TestCompare_CoverageMismatch_FalseWhenSame(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, nil, 5))
	src.Collected = []string{"tables", "storage"}
	dst := inv(table("t1", "db1", "Widgets", nil, nil, 5))
	dst.Collected = []string{"storage", "tables"}

	res := Compare("source", src, "dest", dst)
	if res.CoverageMismatch() {
		t.Fatal("expected CoverageMismatch to be false for the same categories in a different order")
	}
}

// TestCompare_AmbiguousLabels_DetectedWhenBothDefault is a regression
// guard for a real usability bug reproduced live: `amg snapshot` without
// --label always writes "target", so two manifests captured without
// --label produce messages like `exists in target but is missing in
// target` — unreadable. AmbiguousLabels must flag this so callers can
// print an advisory the way CoverageMismatch already does.
func TestCompare_AmbiguousLabels_DetectedWhenBothDefault(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, nil, 5))
	dst := inv()

	res := Compare("target", src, "target", dst)
	if !res.AmbiguousLabels() {
		t.Fatal("expected AmbiguousLabels to be true when both sides share a label")
	}
}

func TestCompare_AmbiguousLabels_FalseWhenDistinct(t *testing.T) {
	src := inv(table("t1", "db1", "Widgets", nil, nil, 5))
	dst := inv(table("t1", "db1", "Widgets", nil, nil, 5))

	res := Compare("source", src, "destination", dst)
	if res.AmbiguousLabels() {
		t.Fatal("expected AmbiguousLabels to be false for distinct labels")
	}
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
	if res.PolicyName != "default" {
		t.Fatalf("expected Compare (no explicit policy) to record policy_name %q, got %q", "default", res.PolicyName)
	}
}

// TestCompareWithPolicy_Strict_BlocksUnexpectedResource is a regression
// guard for the policy/severity separation: unexpected_resource's
// severity is a genuine policy choice (see Policy's doc comment), not a
// fixed comparison fact, so StrictPolicy must be able to promote it to
// BLOCK without touching any other rule's severity.
func TestCompareWithPolicy_Strict_BlocksUnexpectedResource(t *testing.T) {
	src := inv()
	dst := inv(table("t1", "db1", "Widgets", nil, nil, 5))

	res := CompareWithPolicy("source", src, "dest", dst, StrictPolicy())
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK under the strict policy, got %s (%+v)", res.Overall(), res.Findings)
	}
	f := findRule(res.Findings, RuleUnexpectedResource)
	if f == nil {
		t.Fatal("expected an unexpected_resource finding")
	}
	if f.Severity != SeverityBlock {
		t.Fatalf("expected unexpected_resource severity BLOCK under the strict policy, got %s", f.Severity)
	}
	if res.PolicyName != "strict" {
		t.Fatalf("expected policy_name %q, got %q", "strict", res.PolicyName)
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
	src := inventory.Resource{Type: inventory.ResourceTable, ID: "t1", ParentID: "db1", Name: "Widgets", CreatedAt: "2020-01-01T00:00:00Z", UpdatedAt: "2020-01-01T00:00:00Z", SchemaDigest: defaultSchemaDigest}
	dst := inventory.Resource{Type: inventory.ResourceTable, ID: "t1", ParentID: "db1", Name: "Widgets", CreatedAt: "2026-09-27T00:00:00Z", UpdatedAt: "2026-09-27T00:00:00Z", SchemaDigest: defaultSchemaDigest}

	res := Compare("source", inv(src), "dest", inv(dst))
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS: differing timestamps alone must not produce findings, got %s (%+v)", res.Overall(), res.Findings)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("expected zero findings, got %+v", res.Findings)
	}
}

// Table comparison originally checked only enabled/row_security
// metadata, not the actual column/index schema — so "email required
// varchar(255)" -> "email optional varchar(20)" could pass undetected.
// schema_changed catches this via a content digest over columns+indexes
// (see appwrite.schemaDigest), the same digest-not-enumerate pattern
// already proven for row sampling.
func TestCompare_SchemaChanged_Blocks(t *testing.T) {
	src := table("t1", "db1", "Widgets", nil, nil, 0)
	src.SchemaDigest = "digest-with-required-email-varchar255"
	dst := table("t1", "db1", "Widgets", nil, nil, 0)
	dst.SchemaDigest = "digest-with-optional-email-varchar20"

	res := Compare("source", inv(src), "dest", inv(dst))
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleSchemaChanged) == nil {
		t.Fatal("expected a schema_changed finding")
	}
}

func TestCompare_SchemaIdentical_Pass(t *testing.T) {
	src := table("t1", "db1", "Widgets", nil, nil, 0)
	dst := table("t1", "db1", "Widgets", nil, nil, 0)
	// Both use defaultSchemaDigest via the table() helper.

	res := Compare("source", inv(src), "dest", inv(dst))
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_SchemaDigestMissing_Warns(t *testing.T) {
	src := table("t1", "db1", "Widgets", nil, nil, 0)
	src.SchemaDigest = ""
	dst := table("t1", "db1", "Widgets", nil, nil, 0)

	res := Compare("source", inv(src), "dest", inv(dst))
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleSchemaUnverified) == nil {
		t.Fatal("expected a schema_unverified finding")
	}
	// Must never also silently claim a confirmed match.
	if findRule(res.Findings, RuleSchemaChanged) != nil {
		t.Fatal("must not report schema_changed when one side's digest is unknown")
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
